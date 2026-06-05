package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL     string
	queryPath   string
	httpClient  *http.Client
	bearerToken string
	username    string
	password    string
}

type Options struct {
	BaseURL     string
	QueryPath   string
	Timeout     time.Duration
	BearerToken string
	Username    string
	Password    string
}

type Sample struct {
	Metric map[string]string
	Value  float64
}

type queryResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Value  []any             `json:"value"`
		} `json:"result"`
	} `json:"data"`
	ErrorType string `json:"errorType"`
	Error     string `json:"error"`
}

func NewClient(options Options) *Client {
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}

	queryPath := options.QueryPath
	if queryPath == "" {
		queryPath = "/api/v1/query"
	}

	return &Client{
		baseURL:     strings.TrimRight(options.BaseURL, "/"),
		queryPath:   queryPath,
		httpClient:  &http.Client{Timeout: timeout},
		bearerToken: options.BearerToken,
		username:    options.Username,
		password:    options.Password,
	}
}

func (c *Client) Query(ctx context.Context, query string) ([]Sample, error) {
	endpoint, err := url.JoinPath(c.baseURL, c.queryPath)
	if err != nil {
		return nil, err
	}

	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}

	values := reqURL.Query()
	values.Set("query", query)
	reqURL.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return nil, err
	}

	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}

	if c.username != "" || c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("victoriametrics query failed with status %d", resp.StatusCode)
	}

	var parsed queryResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}

	if parsed.Status != "success" {
		return nil, fmt.Errorf(
			"victoriametrics query failed: %s %s",
			parsed.ErrorType,
			parsed.Error,
		)
	}

	samples := make([]Sample, 0, len(parsed.Data.Result))
	for _, result := range parsed.Data.Result {
		if len(result.Value) < 2 {
			continue
		}

		valueString, ok := result.Value[1].(string)
		if !ok {
			continue
		}

		value, err := strconv.ParseFloat(valueString, 64)
		if err != nil {
			continue
		}

		samples = append(samples, Sample{
			Metric: result.Metric,
			Value:  value,
		})
	}

	return samples, nil
}

func RegistryStatusQuery(metricName string, matchers map[string]string, checkType string) string {
	allMatchers := make(map[string]string, len(matchers)+1)
	maps.Copy(allMatchers, matchers)

	allMatchers["check_type"] = checkType

	return metricName + "{" + formatMatchers(allMatchers) + "}"
}

func RegistryLatencyQuery(metricName string, matchers map[string]string) string {
	return metricName + "{" + formatMatchers(matchers) + "}"
}

func formatMatchers(matchers map[string]string) string {
	keys := make([]string, 0, len(matchers))
	for key := range matchers {
		keys = append(keys, key)
	}

	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", key, matchers[key]))
	}

	return strings.Join(parts, ",")
}
