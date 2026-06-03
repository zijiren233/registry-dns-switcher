package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Run             RunConfig             `yaml:"run"`
	SwitchPolicy    SwitchPolicyConfig    `yaml:"switchPolicy"`
	VictoriaMetrics VictoriaMetricsConfig `yaml:"victoriaMetrics"`
	Registry        RegistryConfig        `yaml:"registry"`
	Targets         []TargetConfig        `yaml:"targets"`
	DNS             DNSConfig             `yaml:"dns"`
}

type RunConfig struct {
	Once     bool          `yaml:"once"`
	DryRun   bool          `yaml:"dryRun"`
	Interval time.Duration `yaml:"interval"`
}

type SwitchPolicyConfig struct {
	UnhealthyFor time.Duration `yaml:"unhealthyFor"`
	HealthyFor   time.Duration `yaml:"healthyFor"`
	TieBreaker   string        `yaml:"tieBreaker"`
}

type VictoriaMetricsConfig struct {
	URL               string            `yaml:"url"`
	QueryPath         string            `yaml:"queryPath"`
	Timeout           time.Duration     `yaml:"timeout"`
	BearerToken       string            `yaml:"bearerToken"`
	BasicAuth         BasicAuthConfig   `yaml:"basicAuth"`
	MetricName        string            `yaml:"metricName"`
	LatencyMetricName string            `yaml:"latencyMetricName"`
	LatencyMatchers   map[string]string `yaml:"latencyMatchers"`
	Matchers          map[string]string `yaml:"matchers"`
}

type BasicAuthConfig struct {
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

type RegistryConfig struct {
	Endpoint   string `yaml:"endpoint"`
	Info       string `yaml:"info"`
	Repository string `yaml:"repository"`
	Reference  string `yaml:"reference"`
}

type TargetConfig struct {
	IP       string `yaml:"ip"`
	Priority int    `yaml:"priority"`
	Enabled  *bool  `yaml:"enabled"`
}

type DNSConfig struct {
	Provider   string           `yaml:"provider"`
	RecordName string           `yaml:"recordName"`
	TTL        int64            `yaml:"ttl"`
	AliDNS     AliDNSConfig     `yaml:"alidns"`
	Cloudflare CloudflareConfig `yaml:"cloudflare"`
	Fake       FakeDNSConfig    `yaml:"fake"`
}

type AliDNSConfig struct {
	RegionID        string `yaml:"regionId"`
	AccessKeyID     string `yaml:"accessKeyId"`
	AccessKeySecret string `yaml:"accessKeySecret"`
	DomainName      string `yaml:"domainName"`
	RR              string `yaml:"rr"`
}

type CloudflareConfig struct {
	APIToken string `yaml:"apiToken"`
	ZoneID   string `yaml:"zoneId"`
	Proxied  *bool  `yaml:"proxied"`
}

type FakeDNSConfig struct {
	Records map[string]string `yaml:"records"`
}

type LoadOption func(*Config)

func WithDryRun(dryRun bool) LoadOption {
	return func(cfg *Config) {
		cfg.Run.DryRun = dryRun
	}
}

func WithOnce(once bool) LoadOption {
	return func(cfg *Config) {
		cfg.Run.Once = once
	}
}

func LoadFile(path string, opts ...LoadOption) (*Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return Load(content, opts...)
}

func Load(content []byte, opts ...LoadOption) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(content, &cfg); err != nil {
		return nil, err
	}

	expandEnv(&cfg)
	applyDefaults(&cfg)
	for _, opt := range opts {
		opt(&cfg)
	}
	if err := validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func expandEnv(value any) {
	expandEnvValue(reflect.ValueOf(value))
}

func expandEnvValue(value reflect.Value) {
	if !value.IsValid() {
		return
	}

	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return
		}
		expandEnvValue(value.Elem())
		return
	}

	switch value.Kind() {
	case reflect.Struct:
		for i := range value.NumField() {
			expandEnvValue(value.Field(i))
		}
	case reflect.Slice:
		for i := range value.Len() {
			expandEnvValue(value.Index(i))
		}
	case reflect.Map:
		if value.Type().Key().Kind() != reflect.String || value.Type().Elem().Kind() != reflect.String {
			return
		}
		for _, key := range value.MapKeys() {
			value.SetMapIndex(key, reflect.ValueOf(os.ExpandEnv(value.MapIndex(key).String())))
		}
	case reflect.String:
		if value.CanSet() {
			value.SetString(os.ExpandEnv(value.String()))
		}
	}
}

func applyDefaults(cfg *Config) {
	if cfg.Run.Interval == 0 {
		cfg.Run.Interval = time.Minute
	}
	if cfg.VictoriaMetrics.QueryPath == "" {
		cfg.VictoriaMetrics.QueryPath = "/api/v1/query"
	}
	if cfg.VictoriaMetrics.Timeout == 0 {
		cfg.VictoriaMetrics.Timeout = 15 * time.Second
	}
	if cfg.VictoriaMetrics.MetricName == "" {
		cfg.VictoriaMetrics.MetricName = "sealos_registry_proxy_status"
	}
	if cfg.VictoriaMetrics.LatencyMetricName == "" {
		cfg.VictoriaMetrics.LatencyMetricName = "sealos_registry_proxy_response_time_seconds"
	}
	if cfg.SwitchPolicy.TieBreaker == "" {
		cfg.SwitchPolicy.TieBreaker = "order"
	}
	if cfg.DNS.TTL == 0 {
		cfg.DNS.TTL = 60
	}
}

func validate(cfg *Config) error {
	if cfg.VictoriaMetrics.URL == "" {
		return errors.New("victoriaMetrics.url is required")
	}
	if cfg.Registry.Endpoint == "" {
		return errors.New("registry.endpoint is required")
	}
	switch cfg.SwitchPolicy.TieBreaker {
	case "order":
	case "latency":
	default:
		return fmt.Errorf("unsupported switchPolicy.tieBreaker %q", cfg.SwitchPolicy.TieBreaker)
	}
	if len(cfg.Targets) == 0 {
		return errors.New("targets is required")
	}
	for _, target := range cfg.Targets {
		if net.ParseIP(target.IP) == nil {
			return fmt.Errorf("invalid target ip %q", target.IP)
		}
	}
	if cfg.DNS.Provider == "" {
		return errors.New("dns.provider is required")
	}
	if cfg.DNS.RecordName == "" {
		return errors.New("dns.recordName is required")
	}
	switch cfg.DNS.Provider {
	case "fake":
		return nil
	case "alidns":
		if cfg.Run.DryRun {
			return nil
		}
		if cfg.DNS.AliDNS.RegionID == "" ||
			cfg.DNS.AliDNS.AccessKeyID == "" ||
			cfg.DNS.AliDNS.AccessKeySecret == "" ||
			cfg.DNS.AliDNS.DomainName == "" {
			return errors.New("alidns regionId, accessKeyId, accessKeySecret, and domainName are required")
		}
	case "cloudflare":
		if cfg.Run.DryRun {
			return nil
		}
		if cfg.DNS.Cloudflare.APIToken == "" || cfg.DNS.Cloudflare.ZoneID == "" {
			return errors.New("cloudflare apiToken and zoneId are required")
		}
	default:
		return fmt.Errorf("unsupported dns.provider %q", cfg.DNS.Provider)
	}
	return nil
}
