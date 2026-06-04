package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadFileExpandsEnvironmentVariables(t *testing.T) {
	t.Setenv("CF_TOKEN_FOR_TEST", "token-value")

	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
victoriaMetrics:
  url: http://vm.example.com
registry:
  endpoint: https://registry.example.com:5443
targets:
  - ip: 10.0.0.1
    priority: 10
dns:
  provider: cloudflare
  recordName: registry.example.com
  cloudflare:
    apiToken: ${CF_TOKEN_FOR_TEST}
    zoneId: zone-id
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}

	if cfg.DNS.Cloudflare.APIToken != "token-value" {
		t.Fatalf("apiToken = %q, want token-value", cfg.DNS.Cloudflare.APIToken)
	}
}

func TestLoadFilePreservesEnvironmentValuesWithYAMLSyntax(t *testing.T) {
	t.Setenv("VM_PASSWORD_FOR_TEST", "abc #def")
	t.Setenv("CF_TOKEN_FOR_TEST", "*token-value")

	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
victoriaMetrics:
  url: http://vm.example.com
  basicAuth:
    password: ${VM_PASSWORD_FOR_TEST}
registry:
  endpoint: https://registry.example.com:5443
targets:
  - ip: 10.0.0.1
    priority: 10
dns:
  provider: cloudflare
  recordName: registry.example.com
  cloudflare:
    apiToken: ${CF_TOKEN_FOR_TEST}
    zoneId: zone-id
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}

	if cfg.VictoriaMetrics.BasicAuth.Password != "abc #def" {
		t.Fatalf("password = %q, want abc #def", cfg.VictoriaMetrics.BasicAuth.Password)
	}
	if cfg.DNS.Cloudflare.APIToken != "*token-value" {
		t.Fatalf("apiToken = %q, want *token-value", cfg.DNS.Cloudflare.APIToken)
	}
}

func TestLoadFileDryRunAllowsMissingDNSCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
run:
  dryRun: true
victoriaMetrics:
  url: http://vm.example.com
registry:
  endpoint: https://registry.example.com:5443
targets:
  - ip: 10.0.0.1
    priority: 10
dns:
  provider: cloudflare
  recordName: registry.example.com
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	if _, err := LoadFile(path); err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}
}

func TestLoadFileDryRunOptionAllowsMissingDNSCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
victoriaMetrics:
  url: http://vm.example.com
registry:
  endpoint: https://registry.example.com:5443
targets:
  - ip: 10.0.0.1
    priority: 10
dns:
  provider: cloudflare
  recordName: registry.example.com
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadFile(path, WithDryRun(true))
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}
	if !cfg.Run.DryRun {
		t.Fatal("dryRun = false, want true")
	}
}

func TestLoadFileRequiresDNSCredentialsOutsideDryRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
victoriaMetrics:
  url: http://vm.example.com
registry:
  endpoint: https://registry.example.com:5443
targets:
  - ip: 10.0.0.1
    priority: 10
dns:
  provider: cloudflare
  recordName: registry.example.com
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	if _, err := LoadFile(path); err == nil {
		t.Fatal("expected error for missing DNS credentials outside dry-run")
	}
}

func TestLoadFileAllowsFakeProviderWithoutCredentials(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
victoriaMetrics:
  url: http://vm.example.com
registry:
  endpoint: https://registry.example.com:5443
targets:
  - ip: 10.0.0.1
    priority: 10
dns:
  provider: fake
  recordName: registry.example.com
  fake:
    records:
      registry.example.com/A: 10.0.0.9
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}

	if cfg.DNS.Fake.Records["registry.example.com/A"] != "10.0.0.9" {
		t.Fatalf("fake record not loaded: %#v", cfg.DNS.Fake.Records)
	}
}

func TestLoadFileParsesDurationFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
run:
  dryRun: true
  interval: 30s
switchPolicy:
  unhealthyFor: 2m
  healthyFor: 5m
victoriaMetrics:
  url: http://vm.example.com
  timeout: 10s
registry:
  endpoint: https://registry.example.com:5443
targets:
  - ip: 10.0.0.1
    priority: 10
dns:
  provider: cloudflare
  recordName: registry.example.com
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}

	if cfg.Run.Interval != 30*time.Second {
		t.Fatalf("interval = %v, want 30s", cfg.Run.Interval)
	}
	if cfg.VictoriaMetrics.Timeout != 10*time.Second {
		t.Fatalf("timeout = %v, want 10s", cfg.VictoriaMetrics.Timeout)
	}
	if cfg.SwitchPolicy.UnhealthyFor != 2*time.Minute {
		t.Fatalf("unhealthyFor = %v, want 2m", cfg.SwitchPolicy.UnhealthyFor)
	}
	if cfg.SwitchPolicy.HealthyFor != 5*time.Minute {
		t.Fatalf("healthyFor = %v, want 5m", cfg.SwitchPolicy.HealthyFor)
	}
	if cfg.SwitchPolicy.TieBreaker != "order" {
		t.Fatalf("tieBreaker = %q, want order", cfg.SwitchPolicy.TieBreaker)
	}
	if cfg.VictoriaMetrics.LatencyMetricName != "sealos_registry_proxy_response_time_seconds" {
		t.Fatalf("latencyMetricName = %q, want default", cfg.VictoriaMetrics.LatencyMetricName)
	}
	if cfg.VictoriaMetrics.RegistryEndpointLabel != "endpoint" {
		t.Fatalf("registryEndpointLabel = %q, want endpoint", cfg.VictoriaMetrics.RegistryEndpointLabel)
	}
}

func TestLoadFileAllowsLatencyTieBreakerWithDefaultLatencyMetric(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
run:
  dryRun: true
switchPolicy:
  tieBreaker: latency
victoriaMetrics:
  url: http://vm.example.com
registry:
  endpoint: https://registry.example.com:5443
targets:
  - ip: 10.0.0.1
    priority: 10
dns:
  provider: cloudflare
  recordName: registry.example.com
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}

	if cfg.SwitchPolicy.TieBreaker != "latency" {
		t.Fatalf("tieBreaker = %q, want latency", cfg.SwitchPolicy.TieBreaker)
	}
	if cfg.VictoriaMetrics.LatencyMetricName != "sealos_registry_proxy_response_time_seconds" {
		t.Fatalf("latencyMetricName = %q, want default", cfg.VictoriaMetrics.LatencyMetricName)
	}
}

func TestLoadFileParsesRegistryEndpointLabel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := []byte(`
run:
  dryRun: true
victoriaMetrics:
  url: http://vm.example.com
  registryEndpointLabel: exported_endpoint
  matchers:
    endpoint: server
registry:
  endpoint: https://registry.example.com:5443
targets:
  - ip: 10.0.0.1
    priority: 10
dns:
  provider: cloudflare
  recordName: registry.example.com
`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := LoadFile(path)
	if err != nil {
		t.Fatalf("LoadFile returned error: %v", err)
	}

	if cfg.VictoriaMetrics.RegistryEndpointLabel != "exported_endpoint" {
		t.Fatalf("registryEndpointLabel = %q, want exported_endpoint", cfg.VictoriaMetrics.RegistryEndpointLabel)
	}
	if cfg.VictoriaMetrics.Matchers["endpoint"] != "server" {
		t.Fatalf("matchers.endpoint = %q, want server", cfg.VictoriaMetrics.Matchers["endpoint"])
	}
}
