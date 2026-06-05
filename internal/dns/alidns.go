package dns

import (
	"context"
	"strings"

	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
	"github.com/aliyun/alibaba-cloud-sdk-go/services/alidns"
	"registry-dns-switcher/internal/config"
)

type AliDNSProvider struct {
	client     *alidns.Client
	domainName string
	rr         string
}

func NewAliDNSProvider(cfg config.AliDNSConfig) (*AliDNSProvider, error) {
	client, err := alidns.NewClientWithAccessKey(
		cfg.RegionID,
		cfg.AccessKeyID,
		cfg.AccessKeySecret,
	)
	if err != nil {
		return nil, err
	}

	return &AliDNSProvider{
		client:     client,
		domainName: cfg.DomainName,
		rr:         cfg.RR,
	}, nil
}

func (p *AliDNSProvider) CurrentValue(
	_ context.Context,
	recordName, recordType string,
) (string, error) {
	record, err := p.findRecord(recordName, recordType)
	if err != nil {
		return "", err
	}

	return record.Value, nil
}

func (p *AliDNSProvider) Upsert(
	_ context.Context,
	recordName, recordType, value string,
	ttl int64,
) error {
	record, err := p.findRecord(recordName, recordType)
	if err != nil {
		return err
	}

	if record.RecordID == "" {
		req := alidns.CreateAddDomainRecordRequest()
		req.Scheme = "https"
		req.DomainName = p.domainName
		req.RR = p.recordRR(recordName)
		req.Type = recordType
		req.Value = value
		req.TTL = requests.NewInteger64(ttl)
		_, err := p.client.AddDomainRecord(req)

		return err
	}

	req := alidns.CreateUpdateDomainRecordRequest()
	req.Scheme = "https"
	req.RecordId = record.RecordID
	req.RR = p.recordRR(recordName)
	req.Type = recordType
	req.Value = value
	req.TTL = requests.NewInteger64(ttl)
	_, err = p.client.UpdateDomainRecord(req)

	return err
}

func (p *AliDNSProvider) Delete(_ context.Context, recordName, recordType string) error {
	record, err := p.findRecord(recordName, recordType)
	if err != nil {
		return err
	}

	if record.RecordID == "" {
		return nil
	}

	req := alidns.CreateDeleteDomainRecordRequest()
	req.Scheme = "https"
	req.RecordId = record.RecordID
	_, err = p.client.DeleteDomainRecord(req)

	return err
}

type aliRecord struct {
	RecordID string
	Value    string
}

func (p *AliDNSProvider) findRecord(recordName, recordType string) (aliRecord, error) {
	req := alidns.CreateDescribeDomainRecordsRequest()
	req.Scheme = "https"
	req.DomainName = p.domainName
	req.RRKeyWord = p.recordRR(recordName)
	req.Type = recordType

	resp, err := p.client.DescribeDomainRecords(req)
	if err != nil {
		return aliRecord{}, err
	}

	rr := p.recordRR(recordName)
	for _, record := range resp.DomainRecords.Record {
		if record.RR == rr && record.Type == recordType {
			return aliRecord{
				RecordID: record.RecordId,
				Value:    record.Value,
			}, nil
		}
	}

	return aliRecord{}, nil
}

func (p *AliDNSProvider) recordRR(recordName string) string {
	if p.rr != "" {
		return p.rr
	}

	domainName := strings.TrimSuffix(p.domainName, ".")

	rr := strings.TrimSuffix(recordName, ".")
	if rr == domainName {
		return "@"
	}

	rr = strings.TrimSuffix(rr, "."+domainName)
	if rr == "" {
		return "@"
	}

	return rr
}
