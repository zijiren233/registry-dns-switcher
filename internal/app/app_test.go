package app

import (
	"context"
	"testing"
	"time"

	"registry-dns-switcher/internal/config"
	"registry-dns-switcher/internal/metrics"
)

type fakeMetrics struct {
	queries []string
	results map[string][]metrics.Sample
}

func (f *fakeMetrics) Query(_ context.Context, query string) ([]metrics.Sample, error) {
	f.queries = append(f.queries, query)
	return f.results[query], nil
}

type fakeDNS struct {
	currentValue string
	records      map[string]string
	currentCalls int
	updates      []dnsUpdate
	deletes      []dnsDelete
}

type dnsUpdate struct {
	recordName string
	recordType string
	value      string
	ttl        int64
}

type dnsDelete struct {
	recordName string
	recordType string
}

func (f *fakeDNS) CurrentValue(_ context.Context, recordName, recordType string) (string, error) {
	f.currentCalls++
	if f.records != nil {
		return f.records[recordType], nil
	}

	if recordType != "A" {
		return "", nil
	}

	return f.currentValue, nil
}

func (f *fakeDNS) Upsert(_ context.Context, recordName, recordType, value string, ttl int64) error {
	if f.records != nil {
		f.records[recordType] = value
	}

	f.updates = append(f.updates, dnsUpdate{
		recordName: recordName,
		recordType: recordType,
		value:      value,
		ttl:        ttl,
	})
	f.currentValue = value

	return nil
}

func (f *fakeDNS) Delete(_ context.Context, recordName, recordType string) error {
	if f.records != nil {
		delete(f.records, recordType)
	}

	f.deletes = append(f.deletes, dnsDelete{
		recordName: recordName,
		recordType: recordType,
	})

	return nil
}

func TestReconcileSwitchesToHighestPriorityHealthyIP(t *testing.T) {
	cfg := testConfig()
	queryAPI := `sealos_registry_proxy_status{check_type="api",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	queryManifest := `sealos_registry_proxy_status{check_type="manifest",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	metricClient := &fakeMetrics{results: map[string][]metrics.Sample{
		queryAPI: {
			{Metric: map[string]string{"ip": "10.0.0.1"}, Value: 1},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1},
		},
		queryManifest: {
			{Metric: map[string]string{"ip": "10.0.0.1"}, Value: 1},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1},
		},
	}}
	dnsProvider := &fakeDNS{currentValue: "10.0.0.9"}

	switcher := NewWithDependencies(cfg, metricClient, dnsProvider)
	if err := switcher.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if len(dnsProvider.updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(dnsProvider.updates))
	}

	update := dnsProvider.updates[0]
	if update.recordName != "registry.example.com" ||
		update.recordType != "A" ||
		update.value != "10.0.0.2" ||
		update.ttl != 60 {
		t.Fatalf("unexpected update: %#v", update)
	}
}

func TestReconcileRequiresAPIAndManifestHealthy(t *testing.T) {
	cfg := testConfig()
	queryAPI := `sealos_registry_proxy_status{check_type="api",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	queryManifest := `sealos_registry_proxy_status{check_type="manifest",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	metricClient := &fakeMetrics{results: map[string][]metrics.Sample{
		queryAPI: {
			{Metric: map[string]string{"ip": "10.0.0.1"}, Value: 1},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1},
		},
		queryManifest: {
			{Metric: map[string]string{"ip": "10.0.0.1"}, Value: 1},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 0},
		},
	}}
	dnsProvider := &fakeDNS{currentValue: "10.0.0.9"}

	switcher := NewWithDependencies(cfg, metricClient, dnsProvider)
	if err := switcher.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if len(dnsProvider.updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(dnsProvider.updates))
	}

	if dnsProvider.updates[0].value != "10.0.0.1" {
		t.Fatalf("updated value = %q, want 10.0.0.1", dnsProvider.updates[0].value)
	}
}

func TestReconcileUsesLowestLatencyForPriorityTie(t *testing.T) {
	cfg := testConfig()
	cfg.SwitchPolicy.TieBreaker = "latency"
	cfg.VictoriaMetrics.LatencyMetricName = "sealos_registry_proxy_response_time_seconds"
	cfg.Targets = []config.TargetConfig{
		{IP: "10.0.0.1", Priority: 20},
		{IP: "10.0.0.2", Priority: 20},
	}

	queryAPI := `sealos_registry_proxy_status{check_type="api",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	queryManifest := `sealos_registry_proxy_status{check_type="manifest",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	queryLatency := `sealos_registry_proxy_response_time_seconds{endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	metricClient := &fakeMetrics{results: map[string][]metrics.Sample{
		queryAPI: {
			{Metric: map[string]string{"ip": "10.0.0.1"}, Value: 1},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1},
		},
		queryManifest: {
			{Metric: map[string]string{"ip": "10.0.0.1"}, Value: 1},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1},
		},
		queryLatency: {
			{Metric: map[string]string{"ip": "10.0.0.1"}, Value: 0.3},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 0.1},
		},
	}}
	dnsProvider := &fakeDNS{currentValue: "10.0.0.9"}

	switcher := NewWithDependencies(cfg, metricClient, dnsProvider)
	if err := switcher.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if len(dnsProvider.updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(dnsProvider.updates))
	}

	if dnsProvider.updates[0].value != "10.0.0.2" {
		t.Fatalf("updated value = %q, want 10.0.0.2", dnsProvider.updates[0].value)
	}
}

func TestReconcileUsesLatencyMatchers(t *testing.T) {
	cfg := testConfig()
	cfg.SwitchPolicy.TieBreaker = "latency"
	cfg.VictoriaMetrics.LatencyMetricName = "sealos_registry_proxy_response_time_seconds"
	cfg.VictoriaMetrics.LatencyMatchers = map[string]string{"check_type": "manifest"}
	cfg.Targets = []config.TargetConfig{
		{IP: "10.0.0.1", Priority: 20},
		{IP: "10.0.0.2", Priority: 20},
	}

	queryAPI := `sealos_registry_proxy_status{check_type="api",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	queryManifest := `sealos_registry_proxy_status{check_type="manifest",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	queryLatency := `sealos_registry_proxy_response_time_seconds{check_type="manifest",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	metricClient := &fakeMetrics{results: map[string][]metrics.Sample{
		queryAPI: {
			{Metric: map[string]string{"ip": "10.0.0.1"}, Value: 1},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1},
		},
		queryManifest: {
			{Metric: map[string]string{"ip": "10.0.0.1"}, Value: 1},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1},
		},
		queryLatency: {
			{Metric: map[string]string{"ip": "10.0.0.1"}, Value: 0.3},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 0.1},
		},
	}}
	dnsProvider := &fakeDNS{currentValue: "10.0.0.9"}

	switcher := NewWithDependencies(cfg, metricClient, dnsProvider)
	if err := switcher.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if len(dnsProvider.updates) != 1 {
		t.Fatalf("updates = %d, want 1", len(dnsProvider.updates))
	}

	if dnsProvider.updates[0].value != "10.0.0.2" {
		t.Fatalf("updated value = %q, want 10.0.0.2", dnsProvider.updates[0].value)
	}
}

func TestReconcileUsesConfiguredRegistryEndpointLabel(t *testing.T) {
	cfg := testConfig()
	cfg.VictoriaMetrics.RegistryEndpointLabel = "exported_endpoint"
	cfg.VictoriaMetrics.Matchers = map[string]string{"endpoint": "server"}

	queryAPI := `sealos_registry_proxy_status{check_type="api",endpoint="server",exported_endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	queryManifest := `sealos_registry_proxy_status{check_type="manifest",endpoint="server",exported_endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	metricClient := &fakeMetrics{results: map[string][]metrics.Sample{
		queryAPI:      {{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1}},
		queryManifest: {{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1}},
	}}
	dnsProvider := &fakeDNS{currentValue: "10.0.0.9"}

	switcher := NewWithDependencies(cfg, metricClient, dnsProvider)
	if err := switcher.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if len(dnsProvider.updates) != 1 || dnsProvider.updates[0].value != "10.0.0.2" {
		t.Fatalf("updates = %#v, want switch to 10.0.0.2", dnsProvider.updates)
	}

	if len(metricClient.queries) < 2 || metricClient.queries[0] != queryAPI ||
		metricClient.queries[1] != queryManifest {
		t.Fatalf("queries = %#v, want exported_endpoint queries", metricClient.queries)
	}
}

func TestReconcileSkipsDNSUpdateWhenRecordAlreadyMatches(t *testing.T) {
	cfg := testConfig()
	queryAPI := `sealos_registry_proxy_status{check_type="api",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	queryManifest := `sealos_registry_proxy_status{check_type="manifest",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	metricClient := &fakeMetrics{results: map[string][]metrics.Sample{
		queryAPI:      {{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1}},
		queryManifest: {{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1}},
	}}
	dnsProvider := &fakeDNS{currentValue: "10.0.0.2"}

	switcher := NewWithDependencies(cfg, metricClient, dnsProvider)
	if err := switcher.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if len(dnsProvider.updates) != 0 {
		t.Fatalf("updates = %d, want 0", len(dnsProvider.updates))
	}
}

func TestReconcileDryRunSkipsDNSReadAndUpdate(t *testing.T) {
	cfg := testConfig()
	cfg.Run.DryRun = true

	queryAPI := `sealos_registry_proxy_status{check_type="api",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	queryManifest := `sealos_registry_proxy_status{check_type="manifest",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	metricClient := &fakeMetrics{results: map[string][]metrics.Sample{
		queryAPI:      {{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1}},
		queryManifest: {{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1}},
	}}
	dnsProvider := &fakeDNS{currentValue: "10.0.0.9"}

	switcher := NewWithDependencies(cfg, metricClient, dnsProvider)
	if err := switcher.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if dnsProvider.currentCalls != 0 {
		t.Fatalf("currentCalls = %d, want 0", dnsProvider.currentCalls)
	}

	if len(dnsProvider.updates) != 0 {
		t.Fatalf("updates = %d, want 0", len(dnsProvider.updates))
	}
}

func TestReconcileWaitsForCurrentIPUnhealthyDurationBeforeSwitchingAway(t *testing.T) {
	cfg := testConfig()
	cfg.SwitchPolicy.UnhealthyFor = 2 * time.Minute

	queryAPI := `sealos_registry_proxy_status{check_type="api",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	queryManifest := `sealos_registry_proxy_status{check_type="manifest",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	metricClient := &fakeMetrics{results: map[string][]metrics.Sample{
		queryAPI: {
			{Metric: map[string]string{"ip": "10.0.0.1"}, Value: 0},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1},
		},
		queryManifest: {
			{Metric: map[string]string{"ip": "10.0.0.1"}, Value: 0},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1},
		},
	}}
	dnsProvider := &fakeDNS{currentValue: "10.0.0.1"}
	clock := &fakeClock{now: time.Unix(1000, 0)}

	switcher := NewWithDependencies(cfg, metricClient, dnsProvider)
	switcher.clock = clock

	if err := switcher.Reconcile(context.Background()); err != nil {
		t.Fatalf("first Reconcile returned error: %v", err)
	}

	if len(dnsProvider.updates) != 0 {
		t.Fatalf("updates before unhealthy duration = %d, want 0", len(dnsProvider.updates))
	}

	clock.now = clock.now.Add(2*time.Minute - time.Second)

	if err := switcher.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile returned error: %v", err)
	}

	if len(dnsProvider.updates) != 0 {
		t.Fatalf("updates before threshold = %d, want 0", len(dnsProvider.updates))
	}

	clock.now = clock.now.Add(time.Second)

	if err := switcher.Reconcile(context.Background()); err != nil {
		t.Fatalf("third Reconcile returned error: %v", err)
	}

	if len(dnsProvider.updates) != 1 || dnsProvider.updates[0].value != "10.0.0.2" {
		t.Fatalf("updates after threshold = %#v, want switch to 10.0.0.2", dnsProvider.updates)
	}
}

func TestReconcileWaitsForUnknownCurrentIPUnhealthyDurationBeforeSwitchingAway(t *testing.T) {
	cfg := testConfig()
	cfg.SwitchPolicy.UnhealthyFor = 2 * time.Minute

	queryAPI := `sealos_registry_proxy_status{check_type="api",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	queryManifest := `sealos_registry_proxy_status{check_type="manifest",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	metricClient := &fakeMetrics{results: map[string][]metrics.Sample{
		queryAPI: {
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1},
		},
		queryManifest: {
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1},
		},
	}}
	dnsProvider := &fakeDNS{currentValue: "10.0.0.99"}
	clock := &fakeClock{now: time.Unix(3000, 0)}

	switcher := NewWithDependencies(cfg, metricClient, dnsProvider)
	switcher.clock = clock

	if err := switcher.Reconcile(context.Background()); err != nil {
		t.Fatalf("first Reconcile returned error: %v", err)
	}

	if len(dnsProvider.updates) != 0 {
		t.Fatalf("updates before unhealthy duration = %d, want 0", len(dnsProvider.updates))
	}

	clock.now = clock.now.Add(2 * time.Minute)

	if err := switcher.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile returned error: %v", err)
	}

	if len(dnsProvider.updates) != 1 || dnsProvider.updates[0].value != "10.0.0.2" {
		t.Fatalf("updates after threshold = %#v, want switch to 10.0.0.2", dnsProvider.updates)
	}
}

func TestReconcileWaitsForHigherPriorityIPHealthyDurationBeforeSwitchingBack(t *testing.T) {
	cfg := testConfig()
	cfg.SwitchPolicy.HealthyFor = 5 * time.Minute

	queryAPI := `sealos_registry_proxy_status{check_type="api",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	queryManifest := `sealos_registry_proxy_status{check_type="manifest",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	metricClient := &fakeMetrics{results: map[string][]metrics.Sample{
		queryAPI: {
			{Metric: map[string]string{"ip": "10.0.0.1"}, Value: 1},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1},
		},
		queryManifest: {
			{Metric: map[string]string{"ip": "10.0.0.1"}, Value: 1},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 1},
		},
	}}
	dnsProvider := &fakeDNS{currentValue: "10.0.0.1"}
	clock := &fakeClock{now: time.Unix(2000, 0)}

	switcher := NewWithDependencies(cfg, metricClient, dnsProvider)
	switcher.clock = clock

	if err := switcher.Reconcile(context.Background()); err != nil {
		t.Fatalf("first Reconcile returned error: %v", err)
	}

	if len(dnsProvider.updates) != 0 {
		t.Fatalf("updates before healthy duration = %d, want 0", len(dnsProvider.updates))
	}

	clock.now = clock.now.Add(5*time.Minute - time.Second)

	if err := switcher.Reconcile(context.Background()); err != nil {
		t.Fatalf("second Reconcile returned error: %v", err)
	}

	if len(dnsProvider.updates) != 0 {
		t.Fatalf("updates before threshold = %d, want 0", len(dnsProvider.updates))
	}

	clock.now = clock.now.Add(time.Second)

	if err := switcher.Reconcile(context.Background()); err != nil {
		t.Fatalf("third Reconcile returned error: %v", err)
	}

	if len(dnsProvider.updates) != 1 || dnsProvider.updates[0].value != "10.0.0.2" {
		t.Fatalf("updates after threshold = %#v, want switch to 10.0.0.2", dnsProvider.updates)
	}
}

func TestReconcileDeletesStaleOppositeFamilyRecord(t *testing.T) {
	cfg := testConfig()
	cfg.Targets = []config.TargetConfig{
		{IP: "2001:db8::2", Priority: 20},
		{IP: "10.0.0.2", Priority: 10},
	}
	queryAPI := `sealos_registry_proxy_status{check_type="api",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	queryManifest := `sealos_registry_proxy_status{check_type="manifest",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	metricClient := &fakeMetrics{results: map[string][]metrics.Sample{
		queryAPI: {
			{Metric: map[string]string{"ip": "2001:db8::2"}, Value: 1},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 0},
		},
		queryManifest: {
			{Metric: map[string]string{"ip": "2001:db8::2"}, Value: 1},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 0},
		},
	}}
	dnsProvider := &fakeDNS{records: map[string]string{
		"A":    "10.0.0.2",
		"AAAA": "2001:db8::2",
	}}

	switcher := NewWithDependencies(cfg, metricClient, dnsProvider)
	if err := switcher.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if len(dnsProvider.updates) != 0 {
		t.Fatalf("updates = %d, want 0", len(dnsProvider.updates))
	}

	if len(dnsProvider.deletes) != 1 || dnsProvider.deletes[0].recordType != "A" {
		t.Fatalf("deletes = %#v, want stale A delete", dnsProvider.deletes)
	}

	if _, exists := dnsProvider.records["A"]; exists {
		t.Fatalf("A record still exists: %#v", dnsProvider.records)
	}
}

func TestReconcileKeepsCurrentDNSWhenNoTargetsAreHealthy(t *testing.T) {
	cfg := testConfig()

	queryAPI := `sealos_registry_proxy_status{check_type="api",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	queryManifest := `sealos_registry_proxy_status{check_type="manifest",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	metricClient := &fakeMetrics{results: map[string][]metrics.Sample{
		queryAPI: {
			{Metric: map[string]string{"ip": "10.0.0.1"}, Value: 0},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 0},
		},
		queryManifest: {
			{Metric: map[string]string{"ip": "10.0.0.1"}, Value: 0},
			{Metric: map[string]string{"ip": "10.0.0.2"}, Value: 0},
		},
	}}
	dnsProvider := &fakeDNS{currentValue: "10.0.0.1"}

	switcher := NewWithDependencies(cfg, metricClient, dnsProvider)
	if err := switcher.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile returned error: %v", err)
	}

	if len(dnsProvider.updates) != 0 {
		t.Fatalf("updates = %d, want 0", len(dnsProvider.updates))
	}
}

func testConfig() *config.Config {
	return &config.Config{
		VictoriaMetrics: config.VictoriaMetricsConfig{
			MetricName:            "sealos_registry_proxy_status",
			RegistryEndpointLabel: "endpoint",
		},
		Registry: config.RegistryConfig{
			Endpoint:   "https://registry.example.com:5443",
			Repository: "library/busybox",
			Reference:  "latest",
		},
		Targets: []config.TargetConfig{
			{IP: "10.0.0.1", Priority: 10},
			{IP: "10.0.0.2", Priority: 20},
		},
		DNS: config.DNSConfig{
			RecordName: "registry.example.com",
			TTL:        60,
		},
	}
}

type fakeClock struct {
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	return c.now
}
