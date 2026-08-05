# OutageDeck monitoring mixin

This directory packages the OutageDeck Prometheus alerts and Grafana dashboard as a [monitoring mixin](https://monitoring.mixins.dev/).

The mixin exposes:

- six Prometheus alerts for provider disruption, major outages, unknown state, service disruption, refresh failure, and stale source data;
- the nine-panel OutageDeck provider-status Grafana dashboard; and
- configurable alert durations, source-age threshold, and severity labels.

Generate the alerts as JSON:

```bash
jsonnet -e '(import "mixin/mixin.libsonnet").prometheusAlerts'
```

Generate the Grafana dashboard:

```bash
mkdir -p generated/dashboards
jsonnet -m generated/dashboards -e '(import "mixin/mixin.libsonnet").grafanaDashboards'
```

Override configuration by adding to the imported object:

```jsonnet
(import 'mixin.libsonnet') + {
  _config+:: {
    sourceStaleSeconds: 1800,
    warningSeverity: 'page',
  },
}
```
