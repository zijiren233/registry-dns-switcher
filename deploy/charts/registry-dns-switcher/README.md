# registry-dns-switcher

Helm chart for Registry DNS Switcher.

## Install

```bash
helm upgrade --install registry-dns-switcher ./deploy/charts/registry-dns-switcher \
  --namespace monitoring \
  --create-namespace
```

The default values run in dry-run mode.

Switch hysteresis is controlled by:

```yaml
switchPolicy:
  unhealthyFor: 2m
  healthyFor: 5m
```

`unhealthyFor` controls how long the current DNS IP must stay unhealthy before failover. `healthyFor` controls how long a higher-priority IP must stay healthy before switching back.

The default DNS provider is `fake`, which keeps records in memory and logs updates. Use it to test the switcher end-to-end without external DNS writes:

```bash
helm upgrade --install registry-dns-switcher ./deploy/charts/registry-dns-switcher \
  --set run.dryRun=false \
  --set dns.provider=fake
```

For Cloudflare:

```bash
helm upgrade --install registry-dns-switcher ./deploy/charts/registry-dns-switcher \
  --set run.dryRun=false \
  --set dns.provider=cloudflare \
  --set dns.recordName=registry-proxy.example.com \
  --set secret.cloudflare.apiToken="$CLOUDFLARE_API_TOKEN" \
  --set secret.cloudflare.zoneId="$CLOUDFLARE_ZONE_ID"
```

For AliDNS:

```bash
helm upgrade --install registry-dns-switcher ./deploy/charts/registry-dns-switcher \
  --set run.dryRun=false \
  --set dns.provider=alidns \
  --set dns.recordName=registry-proxy.example.com \
  --set dns.alidns.domainName=example.com \
  --set dns.alidns.rr=registry-proxy \
  --set secret.alidns.accessKeyId="$ALIYUN_ACCESS_KEY_ID" \
  --set secret.alidns.accessKeySecret="$ALIYUN_ACCESS_KEY_SECRET"
```
