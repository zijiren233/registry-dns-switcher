package dns

import "testing"

func TestAliDNSRecordRRDerivesApex(t *testing.T) {
	provider := &AliDNSProvider{domainName: "example.com"}

	for _, recordName := range []string{"example.com", "example.com."} {
		if got := provider.recordRR(recordName); got != "@" {
			t.Fatalf("recordRR(%q) = %q, want @", recordName, got)
		}
	}
}

func TestAliDNSRecordRRDerivesSubdomain(t *testing.T) {
	provider := &AliDNSProvider{domainName: "example.com."}

	if got := provider.recordRR("registry.example.com."); got != "registry" {
		t.Fatalf("recordRR() = %q, want registry", got)
	}
}

func TestAliDNSRecordRRUsesExplicitRR(t *testing.T) {
	provider := &AliDNSProvider{domainName: "example.com", rr: "registry"}

	if got := provider.recordRR("example.com."); got != "registry" {
		t.Fatalf("recordRR() = %q, want registry", got)
	}
}
