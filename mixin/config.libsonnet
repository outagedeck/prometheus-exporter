{
  _config+:: {
    providerDisruptionFor: '2m',
    providerUnknownFor: '10m',
    serviceDisruptionFor: '2m',
    refreshFailedFor: '10m',
    sourceStaleFor: '5m',
    sourceStaleSeconds: 900,
    warningSeverity: 'warning',
    criticalSeverity: 'critical',
  },
}
