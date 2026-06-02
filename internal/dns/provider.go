package dns

import (
	"context"
	"fmt"

	"registry-dns-switcher/internal/config"
)

type Provider interface {
	CurrentValue(ctx context.Context, recordName, recordType string) (string, error)
	Upsert(ctx context.Context, recordName, recordType, value string, ttl int64) error
	Delete(ctx context.Context, recordName, recordType string) error
}

func NewProvider(cfg config.DNSConfig) (Provider, error) {
	switch cfg.Provider {
	case "fake":
		return NewFakeProvider(cfg.Fake), nil
	case "alidns":
		return NewAliDNSProvider(cfg.AliDNS)
	case "cloudflare":
		return NewCloudflareProvider(cfg.Cloudflare)
	default:
		return nil, fmt.Errorf("unsupported dns provider %q", cfg.Provider)
	}
}
