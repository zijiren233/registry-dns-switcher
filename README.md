# Registry DNS Switcher

Registry DNS Switcher reads registry proxy health metrics from VictoriaMetrics and updates a DNS record to the selected healthy IP.

It is designed for metrics emitted by `sealos-state-metrics` registryproxy collector:

```text
sealos_registry_proxy_status{check_type="api",ip="10.0.0.12"} 1
sealos_registry_proxy_status{check_type="manifest",ip="10.0.0.12"} 1
```

An IP is healthy only when both `api` and `manifest` samples are `1`. Among healthy IPs, the largest `priority` wins. Targets with the same priority are selected by `switchPolicy.tieBreaker`.

`switchPolicy.unhealthyFor` delays failover after the current DNS IP becomes unhealthy. `switchPolicy.healthyFor` delays switching to a selected healthy target while the current DNS IP is still healthy. When no configured target is healthy, the tool keeps the current DNS record unchanged and logs a warning.

## Configuration

```yaml
run:
  interval: 30s
  dryRun: false

switchPolicy:
  unhealthyFor: 2m
  healthyFor: 5m
  tieBreaker: order

victoriaMetrics:
  url: http://victoria-metrics.monitoring.svc:8428
  queryPath: /api/v1/query
  timeout: 10s
  bearerToken: ${VICTORIA_METRICS_BEARER_TOKEN}
  basicAuth:
    username: ${VICTORIA_METRICS_BASIC_AUTH_USERNAME}
    password: ${VICTORIA_METRICS_BASIC_AUTH_PASSWORD}
  metricName: sealos_registry_proxy_status
  latencyMetricName: sealos_registry_proxy_response_time_seconds
  latencyMatchers: {}

registry:
  endpoint: https://registry-proxy.example.com:5443
  repository: ""
  reference: ""

targets:
  - ip: 10.0.0.12
    priority: 100
  - ip: 10.0.0.13
    priority: 90

dns:
  provider: fake
  recordName: registry-proxy.example.com
  ttl: 60
  fake:
    records:
      registry-proxy.example.com/A: 10.0.0.99
```

The `fake` provider keeps records in memory and logs every update. It is useful for testing the full VictoriaMetrics query and priority-selection flow without touching external DNS.

`switchPolicy.tieBreaker` controls how targets with the same `priority` are selected:

```yaml
switchPolicy:
  tieBreaker: order
```

`order` keeps the first healthy target from `targets`. To select the lower-latency IP for equal priority, enable `latency`:

```yaml
switchPolicy:
  tieBreaker: latency
```

`victoriaMetrics.latencyMetricName` defaults to `sealos_registry_proxy_response_time_seconds`. The latency metric from `sealos-state-metric` has `endpoint`, `ip`, and `check_type` labels. By default, latency tie-breaking uses the lowest latency sample per IP from all returned `check_type` values. Use `victoriaMetrics.latencyMatchers` to narrow the latency query:

```yaml
victoriaMetrics:
  latencyMetricName: sealos_registry_proxy_response_time_seconds
  latencyMatchers:
    check_type: manifest
```

VictoriaMetrics authentication can use either a bearer token or basic auth. Values support environment variable expansion:

```yaml
victoriaMetrics:
  bearerToken: ${VICTORIA_METRICS_BEARER_TOKEN}
```

```yaml
victoriaMetrics:
  basicAuth:
    username: ${VICTORIA_METRICS_BASIC_AUTH_USERNAME}
    password: ${VICTORIA_METRICS_BASIC_AUTH_PASSWORD}
```

For Cloudflare:

```yaml
dns:
  provider: cloudflare
  recordName: registry-proxy.example.com
  ttl: 60
  cloudflare:
    apiToken: your-token
    zoneId: your-zone-id
    proxied: false
```

For AliDNS:

```yaml
dns:
  provider: alidns
  recordName: registry-proxy.example.com
  ttl: 60
  alidns:
    regionId: cn-hangzhou
    accessKeyId: your-access-key-id
    accessKeySecret: your-access-key-secret
    domainName: example.com
    rr: registry-proxy
```

`dns.recordName` is the FQDN to maintain. For AliDNS, `domainName` is the base zone and `rr` is optional; when empty, the tool derives it from `recordName`.

## Run

```bash
go run . --config config.example.yaml --once --dry-run
```

Long-running mode:

```bash
go run . --config config.yaml
```

Flags override config:

```text
--once      run one reconciliation and exit
--dry-run   select target IP without changing DNS
```

## VictoriaMetrics Query

The tool queries:

```promql
sealos_registry_proxy_status{endpoint="<endpoint>",check_type="api"}
sealos_registry_proxy_status{endpoint="<endpoint>",check_type="manifest"}
```

`registry.info`, `registry.repository`, and `registry.reference` are optional. When set, they are added as label matchers. Additional static label matchers can be set under `victoriaMetrics.matchers`.

## TDD Coverage

Tests cover the behaviors that decide correctness:

- Both `api` and `manifest` must be healthy for an IP to be eligible.
- Highest `priority` wins among healthy enabled targets.
- Same-priority targets can use the configured tie-breaker.
- Failover waits for `switchPolicy.unhealthyFor`.
- Switchback waits for `switchPolicy.healthyFor`.
- All targets unhealthy keeps DNS unchanged.
- DNS update is skipped when the record already points to the selected IP.
- PromQL matcher construction is deterministic.

Run tests:

```bash
go test ./...
```
