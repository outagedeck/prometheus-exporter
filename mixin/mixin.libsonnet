local dashboard = std.parseJson(importstr '../examples/grafana-dashboard.json');

(import 'config.libsonnet') + {
  prometheusAlerts: {
    groups: [
      {
        name: 'outagedeck',
        rules: [
          {
            alert: 'OutageDeckProviderDisruption',
            expr: 'outagedeck_provider_status_code >= 3 and outagedeck_provider_status_code < 5',
            'for': $._config.providerDisruptionFor,
            labels: {
              severity: $._config.warningSeverity,
            },
            annotations: {
              summary: '{{ $labels.name }} is reporting a service disruption',
              description: 'OutageDeck provider status code is {{ $value }} for {{ $labels.provider }}.',
            },
          },
          {
            alert: 'OutageDeckProviderMajorOutage',
            expr: 'outagedeck_provider_status_code == 5',
            'for': $._config.providerDisruptionFor,
            labels: {
              severity: $._config.criticalSeverity,
            },
            annotations: {
              summary: '{{ $labels.name }} is reporting a major outage',
              description: 'OutageDeck reports a major outage for {{ $labels.provider }}.',
            },
          },
          {
            alert: 'OutageDeckProviderStatusUnknown',
            expr: 'outagedeck_provider_status_code == 0',
            'for': $._config.providerUnknownFor,
            labels: {
              severity: $._config.warningSeverity,
            },
            annotations: {
              summary: '{{ $labels.name }} status is unknown',
              description: 'OutageDeck cannot currently normalize the provider status.',
            },
          },
          {
            alert: 'OutageDeckServiceDisruption',
            expr: 'outagedeck_service_status_code >= 3',
            'for': $._config.serviceDisruptionFor,
            labels: {
              severity: $._config.warningSeverity,
            },
            annotations: {
              summary: '{{ $labels.name }} is reporting a service disruption',
              description: 'OutageDeck service status code is {{ $value }} for {{ $labels.service }}.',
            },
          },
          {
            alert: 'OutageDeckRefreshFailed',
            expr: 'outagedeck_scrape_success == 0',
            'for': $._config.refreshFailedFor,
            labels: {
              severity: $._config.warningSeverity,
            },
            annotations: {
              summary: 'OutageDeck refresh failed for {{ $labels.provider }}',
              description: 'The exporter has failed to refresh this provider for the configured interval.',
            },
          },
          {
            alert: 'OutageDeckSourceDataStale',
            expr: 'outagedeck_provider_source_age_seconds > ' + std.toString($._config.sourceStaleSeconds),
            'for': $._config.sourceStaleFor,
            labels: {
              severity: $._config.warningSeverity,
            },
            annotations: {
              summary: 'OutageDeck source data is stale for {{ $labels.name }}',
              description: 'The latest official-source observation is older than the configured threshold.',
            },
          },
        ],
      },
    ],
  },

  prometheusRules: {
    groups: [],
  },

  grafanaDashboards: {
    'outagedeck-provider-status.json': dashboard,
  },
}
