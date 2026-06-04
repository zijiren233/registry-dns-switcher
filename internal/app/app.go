package app

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"time"

	"registry-dns-switcher/internal/config"
	"registry-dns-switcher/internal/dns"
	"registry-dns-switcher/internal/metrics"
	"registry-dns-switcher/internal/switcher"
)

type MetricsClient interface {
	Query(ctx context.Context, query string) ([]metrics.Sample, error)
}

type DNSProvider interface {
	CurrentValue(ctx context.Context, recordName, recordType string) (string, error)
	Upsert(ctx context.Context, recordName, recordType, value string, ttl int64) error
	Delete(ctx context.Context, recordName, recordType string) error
}

type Clock interface {
	Now() time.Time
}

type Switcher struct {
	cfg               *config.Config
	metrics           MetricsClient
	dns               DNSProvider
	clock             Clock
	healthSince       map[string]time.Time
	unhealthySince    map[string]time.Time
	lastObservedState map[string]bool
}

func New(cfg *config.Config) (*Switcher, error) {
	metricsClient := metrics.NewClient(metrics.Options{
		BaseURL:     cfg.VictoriaMetrics.URL,
		QueryPath:   cfg.VictoriaMetrics.QueryPath,
		Timeout:     cfg.VictoriaMetrics.Timeout,
		BearerToken: cfg.VictoriaMetrics.BearerToken,
		Username:    cfg.VictoriaMetrics.BasicAuth.Username,
		Password:    cfg.VictoriaMetrics.BasicAuth.Password,
	})

	if cfg.Run.DryRun {
		return NewWithDependencies(cfg, metricsClient, noopDNSProvider{}), nil
	}

	dnsProvider, err := dns.NewProvider(cfg.DNS)
	if err != nil {
		return nil, err
	}

	return NewWithDependencies(cfg, metricsClient, dnsProvider), nil
}

func NewWithDependencies(
	cfg *config.Config,
	metricsClient MetricsClient,
	dnsProvider DNSProvider,
) *Switcher {
	return &Switcher{
		cfg:               cfg,
		metrics:           metricsClient,
		dns:               dnsProvider,
		clock:             systemClock{},
		healthSince:       make(map[string]time.Time),
		unhealthySince:    make(map[string]time.Time),
		lastObservedState: make(map[string]bool),
	}
}

func (s *Switcher) Reconcile(ctx context.Context) error {
	apiQuery := metrics.RegistryStatusQuery(
		s.cfg.VictoriaMetrics.MetricName,
		s.registryMatchers(),
		"api",
	)
	manifestQuery := metrics.RegistryStatusQuery(
		s.cfg.VictoriaMetrics.MetricName,
		s.registryMatchers(),
		"manifest",
	)

	apiSamples, err := s.metrics.Query(ctx, apiQuery)
	if err != nil {
		return fmt.Errorf("query api health: %w", err)
	}
	manifestSamples, err := s.metrics.Query(ctx, manifestQuery)
	if err != nil {
		return fmt.Errorf("query manifest health: %w", err)
	}

	healthy := switcher.HealthyIPs(
		toHealthSamples(apiSamples),
		toHealthSamples(manifestSamples),
	)
	latencies, err := s.queryLatencies(ctx)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	s.observeHealth(toTargets(s.cfg.Targets), healthy, now)

	if s.cfg.Run.DryRun {
		target, err := switcher.SelectTargetWithPolicy(toTargets(s.cfg.Targets), healthy, s.selectionPolicy(latencies))
		if err != nil {
			slog.Warn("no healthy target found", "error", err)
			return nil
		}
		slog.Info("selected healthy registry ip",
			"record", s.cfg.DNS.RecordName,
			"type", dnsRecordType(target.IP),
			"target", target.IP,
			"priority", target.Priority,
			"dryRun", true,
		)
		return nil
	}

	currentValues, err := s.currentDNSValues(ctx)
	if err != nil {
		return fmt.Errorf("read current dns record: %w", err)
	}
	target, current, ok := s.selectPolicyTarget(toTargets(s.cfg.Targets), healthy, latencies, currentValues, now)
	if !ok {
		return nil
	}

	recordType := dnsRecordType(target.IP)
	oppositeRecordType := oppositeDNSRecordType(recordType)
	if currentValues[recordType] == target.IP && currentValues[oppositeRecordType] == "" {
		slog.Info("dns record already points to selected ip",
			"record", s.cfg.DNS.RecordName,
			"type", recordType,
			"ip", target.IP,
		)
		return nil
	}

	if currentValues[recordType] != target.IP {
		slog.Info("selected healthy registry ip",
			"record", s.cfg.DNS.RecordName,
			"type", recordType,
			"current", current,
			"target", target.IP,
			"priority", target.Priority,
			"dryRun", false,
		)

		if err := s.dns.Upsert(ctx, s.cfg.DNS.RecordName, recordType, target.IP, s.cfg.DNS.TTL); err != nil {
			return fmt.Errorf("upsert dns record: %w", err)
		}
	}

	if currentValues[oppositeRecordType] != "" {
		slog.Info("removing stale dns record",
			"record", s.cfg.DNS.RecordName,
			"type", oppositeRecordType,
			"value", currentValues[oppositeRecordType],
			"selectedType", recordType,
			"target", target.IP,
		)
		if err := s.dns.Delete(ctx, s.cfg.DNS.RecordName, oppositeRecordType); err != nil {
			return fmt.Errorf("delete stale dns record: %w", err)
		}
	}
	return nil
}

func (s *Switcher) currentDNSValues(ctx context.Context) (map[string]string, error) {
	values := make(map[string]string, 2)
	for _, recordType := range []string{"A", "AAAA"} {
		current, err := s.dns.CurrentValue(ctx, s.cfg.DNS.RecordName, recordType)
		if err != nil {
			return nil, err
		}
		values[recordType] = current
	}
	return values, nil
}

func (s *Switcher) queryLatencies(ctx context.Context) (map[string]float64, error) {
	if s.cfg.SwitchPolicy.TieBreaker != "latency" {
		return nil, nil
	}

	query := metrics.RegistryLatencyQuery(
		s.cfg.VictoriaMetrics.LatencyMetricName,
		s.latencyMatchers(),
	)
	samples, err := s.metrics.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query latency: %w", err)
	}
	return toLatencyMap(samples), nil
}

func (s *Switcher) observeHealth(targets []switcher.Target, healthy map[string]struct{}, now time.Time) {
	for _, target := range targets {
		if !target.Enabled {
			continue
		}
		_, isHealthy := healthy[target.IP]
		if previous, exists := s.lastObservedState[target.IP]; exists && previous == isHealthy {
			continue
		}
		s.lastObservedState[target.IP] = isHealthy
		if isHealthy {
			s.healthSince[target.IP] = now
			delete(s.unhealthySince, target.IP)
			continue
		}
		s.unhealthySince[target.IP] = now
		delete(s.healthSince, target.IP)
	}
}

func (s *Switcher) selectPolicyTarget(
	targets []switcher.Target,
	healthy map[string]struct{},
	latencies map[string]float64,
	currentValues map[string]string,
	now time.Time,
) (switcher.Target, string, bool) {
	target, err := switcher.SelectTargetWithPolicy(targets, healthy, s.selectionPolicy(latencies))
	if err != nil {
		slog.Warn("no healthy target found", "error", err, "current", selectedCurrentValue(currentValues))
		return switcher.Target{}, "", false
	}

	current := currentForTarget(currentValues, target)
	if current == "" || current == target.IP {
		return target, current, true
	}

	if _, currentHealthy := healthy[current]; currentHealthy {
		if s.cfg.SwitchPolicy.HealthyFor > 0 {
			since, ok := s.healthSince[target.IP]
			if !ok || now.Sub(since) < s.cfg.SwitchPolicy.HealthyFor {
				slog.Info("selected target is healthy but waiting for healthy duration",
					"current", current,
					"target", target.IP,
					"healthyFor", s.cfg.SwitchPolicy.HealthyFor,
				)
				return switcher.Target{}, current, false
			}
		}
		return target, current, true
	}

	if s.cfg.SwitchPolicy.UnhealthyFor > 0 {
		since, ok := s.unhealthySince[current]
		if !ok {
			s.unhealthySince[current] = now
			since = now
		}
		if now.Sub(since) < s.cfg.SwitchPolicy.UnhealthyFor {
			slog.Info("current target is unhealthy but waiting for unhealthy duration",
				"current", current,
				"target", target.IP,
				"unhealthyFor", s.cfg.SwitchPolicy.UnhealthyFor,
			)
			return switcher.Target{}, current, false
		}
	}
	return target, current, true
}

func (s *Switcher) selectionPolicy(latencies map[string]float64) switcher.SelectionPolicy {
	return switcher.SelectionPolicy{
		TieBreaker: s.cfg.SwitchPolicy.TieBreaker,
		Latencies:  latencies,
	}
}

func (s *Switcher) registryMatchers() map[string]string {
	matchers := make(map[string]string, len(s.cfg.VictoriaMetrics.Matchers)+4)
	for key, value := range s.cfg.VictoriaMetrics.Matchers {
		matchers[key] = value
	}
	registryEndpointLabel := s.cfg.VictoriaMetrics.RegistryEndpointLabel
	if registryEndpointLabel == "" {
		registryEndpointLabel = "endpoint"
	}
	matchers[registryEndpointLabel] = s.cfg.Registry.Endpoint
	if s.cfg.Registry.Info != "" {
		matchers["info"] = s.cfg.Registry.Info
	}
	if s.cfg.Registry.Repository != "" {
		matchers["repository"] = s.cfg.Registry.Repository
	}
	if s.cfg.Registry.Reference != "" {
		matchers["reference"] = s.cfg.Registry.Reference
	}
	return matchers
}

func (s *Switcher) latencyMatchers() map[string]string {
	matchers := s.registryMatchers()
	for key, value := range s.cfg.VictoriaMetrics.LatencyMatchers {
		matchers[key] = value
	}
	return matchers
}

func toHealthSamples(samples []metrics.Sample) []switcher.HealthSample {
	result := make([]switcher.HealthSample, 0, len(samples))
	for _, sample := range samples {
		ip := sample.Metric["ip"]
		if ip == "" {
			continue
		}
		result = append(result, switcher.HealthSample{
			IP:    ip,
			Value: sample.Value,
		})
	}
	return result
}

func toLatencyMap(samples []metrics.Sample) map[string]float64 {
	result := make(map[string]float64, len(samples))
	for _, sample := range samples {
		ip := sample.Metric["ip"]
		if ip == "" {
			continue
		}
		current, exists := result[ip]
		if !exists || sample.Value < current {
			result[ip] = sample.Value
		}
	}
	return result
}

func toTargets(configs []config.TargetConfig) []switcher.Target {
	targets := make([]switcher.Target, 0, len(configs))
	for _, cfg := range configs {
		enabled := true
		if cfg.Enabled != nil {
			enabled = *cfg.Enabled
		}
		targets = append(targets, switcher.Target{
			IP:       cfg.IP,
			Priority: cfg.Priority,
			Enabled:  enabled,
		})
	}
	return targets
}

func dnsRecordType(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed != nil && parsed.To4() == nil {
		return "AAAA"
	}
	return "A"
}

func oppositeDNSRecordType(recordType string) string {
	if recordType == "A" {
		return "AAAA"
	}
	return "A"
}

func selectedCurrentValue(values map[string]string) string {
	if values["A"] != "" {
		return values["A"]
	}
	return values["AAAA"]
}

func currentForTarget(values map[string]string, target switcher.Target) string {
	recordType := dnsRecordType(target.IP)
	if values[recordType] != "" {
		return values[recordType]
	}
	return selectedCurrentValue(values)
}

type noopDNSProvider struct{}

func (noopDNSProvider) CurrentValue(context.Context, string, string) (string, error) {
	return "", nil
}

func (noopDNSProvider) Upsert(context.Context, string, string, string, int64) error {
	return nil
}

func (noopDNSProvider) Delete(context.Context, string, string) error {
	return nil
}

type systemClock struct{}

func (systemClock) Now() time.Time {
	return time.Now()
}
