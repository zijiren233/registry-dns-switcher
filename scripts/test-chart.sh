#!/usr/bin/env bash
set -euo pipefail

chart="${1:-deploy/charts/registry-dns-switcher}"
rendered="$(mktemp)"
trap 'rm -f "${rendered}"' EXIT

helm lint "${chart}"
helm template registry-dns-switcher "${chart}" >"${rendered}"

grep -q 'kind: Deployment' "${rendered}"
grep -q 'kind: ConfigMap' "${rendered}"
grep -q 'kind: Secret' "${rendered}"
grep -q 'dryRun: true' "${rendered}"
grep -q 'unhealthyFor: "2m"' "${rendered}"
grep -q 'healthyFor: "5m"' "${rendered}"
grep -q 'registryEndpointLabel: "endpoint"' "${rendered}"
grep -q 'provider: "fake"' "${rendered}"
grep -q 'registry-proxy.example.com/A: 10.0.0.99' "${rendered}"
grep -q 'name: registry-dns-switcher-credentials' "${rendered}"
grep -q 'apiToken: ${CLOUDFLARE_API_TOKEN}' "${rendered}"
grep -q 'bearerToken: ${VICTORIA_METRICS_BEARER_TOKEN}' "${rendered}"
grep -q 'password: ${VICTORIA_METRICS_BASIC_AUTH_PASSWORD}' "${rendered}"

rendered_with_secrets="$(mktemp)"
trap 'rm -f "${rendered}" "${rendered_with_secrets}"' EXIT

helm template registry-dns-switcher "${chart}" \
  --set secret.victoriaMetrics.bearerToken=vm-token \
  --set secret.victoriaMetrics.basicAuth.username=vm-user \
  --set secret.victoriaMetrics.basicAuth.password=vm-password \
  >"${rendered_with_secrets}"

grep -q 'VICTORIA_METRICS_BEARER_TOKEN: "vm-token"' "${rendered_with_secrets}"
grep -q 'VICTORIA_METRICS_BASIC_AUTH_USERNAME: "vm-user"' "${rendered_with_secrets}"
grep -q 'VICTORIA_METRICS_BASIC_AUTH_PASSWORD: "vm-password"' "${rendered_with_secrets}"
awk '
  /^kind: ConfigMap$/ { in_configmap=1; next }
  /^---$/ { in_configmap=0 }
  in_configmap && /vm-token|vm-user|vm-password/ { found=1 }
  END { exit found ? 1 : 0 }
' "${rendered_with_secrets}"
