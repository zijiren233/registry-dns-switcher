package metrics

import "testing"

func TestRegistryStatusQuery(t *testing.T) {
	query := RegistryStatusQuery(
		"sealos_registry_proxy_status",
		map[string]string{
			"endpoint":   "https://registry.example.com:5443",
			"repository": "library/busybox",
			"reference":  "latest",
		},
		"manifest",
	)

	want := `sealos_registry_proxy_status{check_type="manifest",endpoint="https://registry.example.com:5443",reference="latest",repository="library/busybox"}`
	if query != want {
		t.Fatalf("query = %q, want %q", query, want)
	}
}

func TestRegistryLatencyQuery(t *testing.T) {
	query := RegistryLatencyQuery(
		"sealos_registry_proxy_response_time_seconds",
		map[string]string{
			"endpoint": "https://registry.example.com:5443",
		},
	)

	want := `sealos_registry_proxy_response_time_seconds{endpoint="https://registry.example.com:5443"}`
	if query != want {
		t.Fatalf("query = %q, want %q", query, want)
	}
}
