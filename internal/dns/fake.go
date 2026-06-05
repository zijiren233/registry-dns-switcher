package dns

import (
	"context"
	"log/slog"
	"maps"
	"sync"

	"registry-dns-switcher/internal/config"
)

type FakeProvider struct {
	mu      sync.RWMutex
	records map[string]string
}

func NewFakeProvider(cfg config.FakeDNSConfig) *FakeProvider {
	records := make(map[string]string, len(cfg.Records))
	maps.Copy(records, cfg.Records)

	return &FakeProvider{records: records}
}

func (p *FakeProvider) CurrentValue(
	_ context.Context,
	recordName, recordType string,
) (string, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.records[fakeRecordKey(recordName, recordType)], nil
}

func (p *FakeProvider) Upsert(
	_ context.Context,
	recordName, recordType, value string,
	ttl int64,
) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := fakeRecordKey(recordName, recordType)
	current := p.records[key]
	p.records[key] = value

	slog.Info("fake dns upsert",
		"record", recordName,
		"type", recordType,
		"previous", current,
		"value", value,
		"ttl", ttl,
	)

	return nil
}

func (p *FakeProvider) Delete(_ context.Context, recordName, recordType string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	key := fakeRecordKey(recordName, recordType)
	current := p.records[key]
	delete(p.records, key)

	slog.Info("fake dns delete",
		"record", recordName,
		"type", recordType,
		"previous", current,
	)

	return nil
}

func fakeRecordKey(recordName, recordType string) string {
	return recordName + "/" + recordType
}
