package dns

import (
	"context"
	"testing"

	"registry-dns-switcher/internal/config"
)

func TestFakeProviderStoresRecordsInMemory(t *testing.T) {
	provider := NewFakeProvider(config.FakeDNSConfig{
		Records: map[string]string{
			"registry.example.com/A": "10.0.0.1",
		},
	})

	current, err := provider.CurrentValue(context.Background(), "registry.example.com", "A")
	if err != nil {
		t.Fatalf("CurrentValue returned error: %v", err)
	}
	if current != "10.0.0.1" {
		t.Fatalf("current = %q, want 10.0.0.1", current)
	}

	if err := provider.Upsert(context.Background(), "registry.example.com", "A", "10.0.0.2", 60); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}

	current, err = provider.CurrentValue(context.Background(), "registry.example.com", "A")
	if err != nil {
		t.Fatalf("CurrentValue returned error: %v", err)
	}
	if current != "10.0.0.2" {
		t.Fatalf("current = %q, want 10.0.0.2", current)
	}

	if err := provider.Delete(context.Background(), "registry.example.com", "A"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}

	current, err = provider.CurrentValue(context.Background(), "registry.example.com", "A")
	if err != nil {
		t.Fatalf("CurrentValue returned error: %v", err)
	}
	if current != "" {
		t.Fatalf("current = %q, want empty", current)
	}
}

func TestNewProviderSupportsFake(t *testing.T) {
	provider, err := NewProvider(config.DNSConfig{
		Provider: "fake",
		Fake: config.FakeDNSConfig{
			Records: map[string]string{"registry.example.com/A": "10.0.0.1"},
		},
	})
	if err != nil {
		t.Fatalf("NewProvider returned error: %v", err)
	}

	current, err := provider.CurrentValue(context.Background(), "registry.example.com", "A")
	if err != nil {
		t.Fatalf("CurrentValue returned error: %v", err)
	}
	if current != "10.0.0.1" {
		t.Fatalf("current = %q, want 10.0.0.1", current)
	}
}
