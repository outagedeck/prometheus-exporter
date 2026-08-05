# OutageDeck Prometheus Exporter

Export live cloud and SaaS provider status from [OutageDeck](https://outagedeck.com/?utm_source=prometheus&utm_medium=integration&utm_campaign=prometheus_exporter) as Prometheus metrics.

The exporter polls OutageDeck on a conservative five-minute interval, caches the latest provider snapshots, and serves them locally at `/metrics`. It works without an API key and supports optional keyed access for higher quotas.

## Quick start

```bash
docker run --rm -p 9787:9787 \
  ghcr.io/outagedeck/prometheus-exporter:v0.1.0 \
  --providers github,aws,openai
```

Then open <http://localhost:9787/metrics> or add the target to Prometheus:

```yaml
scrape_configs:
  - job_name: outagedeck
    static_configs:
      - targets: ["outagedeck-exporter:9787"]
```

Prometheus can scrape the cached metrics frequently without increasing upstream API traffic. The exporter refreshes all configured providers separately on `--refresh-interval`.

## Install

Prebuilt Linux, macOS, and Windows binaries for AMD64 and ARM64 are attached to each [GitHub release](https://github.com/outagedeck/prometheus-exporter/releases).

With Go 1.24 or newer:

```bash
go install github.com/outagedeck/prometheus-exporter/cmd/outagedeck-prometheus-exporter@latest
outagedeck-prometheus-exporter --providers github,aws,openai
```

Build locally:

```bash
go build ./cmd/outagedeck-prometheus-exporter
```

## Configuration

| Flag | Environment variable | Default | Purpose |
| --- | --- | --- | --- |
| `--providers` | `OUTAGEDECK_PROVIDERS` | `github,aws,openai` | Comma-separated provider slugs, up to 20 |
| `--refresh-interval` | `OUTAGEDECK_REFRESH_INTERVAL` | `5m` | Upstream refresh interval |
| `--request-timeout` | `OUTAGEDECK_REQUEST_TIMEOUT` | `10s` | Timeout for each provider request |
| `--api-key` | `OUTAGEDECK_API_KEY` | empty | Optional API key |
| `--web.listen-address` | `OUTAGEDECK_LISTEN_ADDRESS` | `:9787` | Exporter listen address |
| `--web.telemetry-path` | `OUTAGEDECK_METRICS_PATH` | `/metrics` | Metrics path |
| `--api-base-url` | `OUTAGEDECK_API_BASE_URL` | production API | Alternate API base for testing |

Browse the [live provider catalog](https://outagedeck.com/providers?utm_source=prometheus&utm_medium=integration&utm_campaign=prometheus_exporter) for valid slugs. The anonymous API permits 120 requests per hour; the default three providers at a five-minute refresh use 36 requests per hour. For larger or faster deployments, [create an API key](https://outagedeck.com/account?utm_source=prometheus&utm_medium=integration&utm_campaign=prometheus_exporter).

## Metrics

| Metric | Meaning |
| --- | --- |
| `outagedeck_provider_status_code` | Provider status: `0` unknown, `1` operational, `2` maintenance, `3` degraded, `4` partial outage, `5` major outage |
| `outagedeck_provider_status_info` | Current human-readable provider status as a label |
| `outagedeck_provider_active_incidents` | Active incident count |
| `outagedeck_provider_source_timestamp_seconds` | Timestamp of OutageDeck's latest official-source check |
| `outagedeck_provider_source_age_seconds` | Age of that source observation |
| `outagedeck_service_status_code` | Status code for each tracked provider service |
| `outagedeck_service_status_info` | Current human-readable service status as a label |
| `outagedeck_scrape_success` | Whether the latest OutageDeck API refresh succeeded |
| `outagedeck_scrape_duration_seconds` | Latest upstream request duration |
| `outagedeck_scrape_timestamp_seconds` | Latest upstream refresh-attempt timestamp |
| `outagedeck_exporter_build_info` | Exporter version and build metadata |

The last good provider snapshot remains available during a transient API failure. Use `outagedeck_scrape_success` alongside the status and source-age metrics to distinguish upstream refresh failures from provider incidents.

## Alerting rules

Ready-to-use rules live in [`examples/alerts.yml`](examples/alerts.yml). The core provider alert is deliberately simple:

```promql
outagedeck_provider_status_code >= 3
```

This alerts for degraded performance and outages while excluding planned maintenance. The example rules separately alert on unknown state, stale official-source data, and exporter refresh failures.

## Endpoints

- `/metrics` — Prometheus metrics
- `/healthz` — process liveness
- `/readyz` — readiness after at least one provider snapshot succeeds

## Security and privacy

The exporter makes read-only HTTPS requests to the public OutageDeck API. API keys are optional, are sent only in the `X-API-Key` header, and are never included in metric labels or logs. See [`SECURITY.md`](SECURITY.md) for vulnerability reporting.

## License

MIT
