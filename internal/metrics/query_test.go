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
