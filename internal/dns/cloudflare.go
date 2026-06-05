package dns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"registry-dns-switcher/internal/config"
)

const cloudflareBaseURL = "https://api.cloudflare.com/client/v4"

type CloudflareProvider struct {
	apiToken string
	zoneID   string
	proxied  bool
	client   *http.Client
}

type cloudflareListResponse struct {
	Success bool `json:"success"`
	Result  []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Type    string `json:"type"`
		Content string `json:"content"`
	} `json:"result"`
	Errors []cloudflareError `json:"errors"`
}

type cloudflareWriteResponse struct {
	Success bool              `json:"success"`
	Errors  []cloudflareError `json:"errors"`
}

type cloudflareError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func NewCloudflareProvider(cfg config.CloudflareConfig) (*CloudflareProvider, error) {
	proxied := false
	if cfg.Proxied != nil {
		proxied = *cfg.Proxied
	}

	return &CloudflareProvider{
		apiToken: cfg.APIToken,
		zoneID:   cfg.ZoneID,
		proxied:  proxied,
		client:   &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (p *CloudflareProvider) CurrentValue(
	ctx context.Context,
	recordName, recordType string,
) (string, error) {
	record, err := p.findRecord(ctx, recordName, recordType)
	if err != nil {
		return "", err
	}

	if record.ID == "" {
		return "", nil
	}

	return record.Content, nil
}

func (p *CloudflareProvider) Upsert(
	ctx context.Context,
	recordName, recordType, value string,
	ttl int64,
) error {
	record, err := p.findRecord(ctx, recordName, recordType)
	if err != nil {
		return err
	}

	body := map[string]any{
		"type":    recordType,
		"name":    recordName,
		"content": value,
		"ttl":     ttl,
		"proxied": p.proxied,
	}
	method := http.MethodPost

	endpoint := fmt.Sprintf("%s/zones/%s/dns_records", cloudflareBaseURL, p.zoneID)
	if record.ID != "" {
		method = http.MethodPut
		endpoint = fmt.Sprintf("%s/zones/%s/dns_records/%s", cloudflareBaseURL, p.zoneID, record.ID)
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(payload))
	if err != nil {
		return err
	}

	p.setHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var parsed cloudflareWriteResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !parsed.Success {
		return fmt.Errorf(
			"cloudflare upsert failed: status=%d errors=%v",
			resp.StatusCode,
			parsed.Errors,
		)
	}

	return nil
}

func (p *CloudflareProvider) Delete(ctx context.Context, recordName, recordType string) error {
	record, err := p.findRecord(ctx, recordName, recordType)
	if err != nil {
		return err
	}

	if record.ID == "" {
		return nil
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodDelete,
		fmt.Sprintf("%s/zones/%s/dns_records/%s", cloudflareBaseURL, p.zoneID, record.ID),
		nil,
	)
	if err != nil {
		return err
	}

	p.setHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var parsed cloudflareWriteResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !parsed.Success {
		return fmt.Errorf(
			"cloudflare delete failed: status=%d errors=%v",
			resp.StatusCode,
			parsed.Errors,
		)
	}

	return nil
}

type cloudflareRecord struct {
	ID      string
	Content string
}

func (p *CloudflareProvider) findRecord(
	ctx context.Context,
	recordName, recordType string,
) (cloudflareRecord, error) {
	endpoint, err := url.Parse(fmt.Sprintf("%s/zones/%s/dns_records", cloudflareBaseURL, p.zoneID))
	if err != nil {
		return cloudflareRecord{}, err
	}

	values := endpoint.Query()
	values.Set("type", recordType)
	values.Set("name", recordName)
	endpoint.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return cloudflareRecord{}, err
	}

	p.setHeaders(req)

	resp, err := p.client.Do(req)
	if err != nil {
		return cloudflareRecord{}, err
	}
	defer resp.Body.Close()

	var parsed cloudflareListResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return cloudflareRecord{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 || !parsed.Success {
		return cloudflareRecord{}, fmt.Errorf(
			"cloudflare list failed: status=%d errors=%v",
			resp.StatusCode,
			parsed.Errors,
		)
	}

	if len(parsed.Result) == 0 {
		return cloudflareRecord{}, nil
	}

	return cloudflareRecord{
		ID:      parsed.Result[0].ID,
		Content: parsed.Result[0].Content,
	}, nil
}

func (p *CloudflareProvider) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+p.apiToken)
	req.Header.Set("Content-Type", "application/json")
}
