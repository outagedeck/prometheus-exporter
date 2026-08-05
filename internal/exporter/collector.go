package exporter

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/outagedeck/prometheus-exporter/internal/outagedeck"
	"github.com/prometheus/client_golang/prometheus"
)

type ProviderClient interface {
	FetchProvider(context.Context, string) (outagedeck.Provider, error)
}

type providerState struct {
	provider    *outagedeck.Provider
	success     float64
	duration    float64
	lastAttempt time.Time
	err         error
}

type Collector struct {
	client    ProviderClient
	providers []string

	mu     sync.RWMutex
	states map[string]providerState

	providerStatus          *prometheus.Desc
	providerStatusInfo      *prometheus.Desc
	providerActiveIncidents *prometheus.Desc
	providerSourceTimestamp *prometheus.Desc
	providerSourceAge       *prometheus.Desc
	serviceStatus           *prometheus.Desc
	serviceStatusInfo       *prometheus.Desc
	scrapeSuccess           *prometheus.Desc
	scrapeDuration          *prometheus.Desc
	lastScrapeTimestamp     *prometheus.Desc
}

func NewCollector(client ProviderClient, providers []string) *Collector {
	return &Collector{
		client:    client,
		providers: append([]string(nil), providers...),
		states:    make(map[string]providerState, len(providers)),
		providerStatus: prometheus.NewDesc(
			"outagedeck_provider_status_code",
			"Normalized provider status: 0 unknown, 1 operational, 2 maintenance, 3 degraded, 4 partial outage, 5 major outage.",
			[]string{"provider", "name"}, nil,
		),
		providerStatusInfo: prometheus.NewDesc(
			"outagedeck_provider_status_info",
			"Provider status metadata; the current status series has value 1.",
			[]string{"provider", "name", "status"}, nil,
		),
		providerActiveIncidents: prometheus.NewDesc(
			"outagedeck_provider_active_incidents",
			"Number of active incidents reported for the provider.",
			[]string{"provider", "name"}, nil,
		),
		providerSourceTimestamp: prometheus.NewDesc(
			"outagedeck_provider_source_timestamp_seconds",
			"Unix timestamp when OutageDeck last checked the provider's official source.",
			[]string{"provider", "name"}, nil,
		),
		providerSourceAge: prometheus.NewDesc(
			"outagedeck_provider_source_age_seconds",
			"Age in seconds of the provider's official-source observation.",
			[]string{"provider", "name"}, nil,
		),
		serviceStatus: prometheus.NewDesc(
			"outagedeck_service_status_code",
			"Normalized service status: 0 unknown, 1 operational, 2 maintenance, 3 degraded, 4 partial outage, 5 major outage.",
			[]string{"provider", "service", "name"}, nil,
		),
		serviceStatusInfo: prometheus.NewDesc(
			"outagedeck_service_status_info",
			"Service status metadata; the current status series has value 1.",
			[]string{"provider", "service", "name", "status"}, nil,
		),
		scrapeSuccess: prometheus.NewDesc(
			"outagedeck_scrape_success",
			"Whether the latest OutageDeck API refresh succeeded for the provider.",
			[]string{"provider"}, nil,
		),
		scrapeDuration: prometheus.NewDesc(
			"outagedeck_scrape_duration_seconds",
			"Duration of the latest OutageDeck API refresh for the provider.",
			[]string{"provider"}, nil,
		),
		lastScrapeTimestamp: prometheus.NewDesc(
			"outagedeck_scrape_timestamp_seconds",
			"Unix timestamp of the latest OutageDeck API refresh attempt for the provider.",
			[]string{"provider"}, nil,
		),
	}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.providerStatus
	ch <- c.providerStatusInfo
	ch <- c.providerActiveIncidents
	ch <- c.providerSourceTimestamp
	ch <- c.providerSourceAge
	ch <- c.serviceStatus
	ch <- c.serviceStatusInfo
	ch <- c.scrapeSuccess
	ch <- c.scrapeDuration
	ch <- c.lastScrapeTimestamp
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	states := make(map[string]providerState, len(c.states))
	for slug, state := range c.states {
		states[slug] = state
	}
	c.mu.RUnlock()

	now := time.Now()
	for _, slug := range c.providers {
		state := states[slug]
		ch <- prometheus.MustNewConstMetric(c.scrapeSuccess, prometheus.GaugeValue, state.success, slug)
		ch <- prometheus.MustNewConstMetric(c.scrapeDuration, prometheus.GaugeValue, state.duration, slug)
		if !state.lastAttempt.IsZero() {
			ch <- prometheus.MustNewConstMetric(c.lastScrapeTimestamp, prometheus.GaugeValue, float64(state.lastAttempt.Unix()), slug)
		}
		if state.provider == nil {
			continue
		}

		provider := state.provider
		status := normalizeStatus(provider.CurrentStatus.Code)
		ch <- prometheus.MustNewConstMetric(c.providerStatus, prometheus.GaugeValue, StatusCode(status), provider.Slug, provider.Name)
		ch <- prometheus.MustNewConstMetric(c.providerStatusInfo, prometheus.GaugeValue, 1, provider.Slug, provider.Name, status)
		ch <- prometheus.MustNewConstMetric(c.providerActiveIncidents, prometheus.GaugeValue, float64(provider.Counts.ActiveIncidents), provider.Slug, provider.Name)

		if checkedAt, err := time.Parse(time.RFC3339Nano, provider.Source.CheckedAt); err == nil {
			age := now.Sub(checkedAt).Seconds()
			if age < 0 {
				age = 0
			}
			ch <- prometheus.MustNewConstMetric(c.providerSourceTimestamp, prometheus.GaugeValue, float64(checkedAt.Unix()), provider.Slug, provider.Name)
			ch <- prometheus.MustNewConstMetric(c.providerSourceAge, prometheus.GaugeValue, age, provider.Slug, provider.Name)
		}

		for _, service := range provider.Services {
			serviceStatus := normalizeStatus(service.Status)
			ch <- prometheus.MustNewConstMetric(c.serviceStatus, prometheus.GaugeValue, StatusCode(serviceStatus), provider.Slug, service.Slug, service.Name)
			ch <- prometheus.MustNewConstMetric(c.serviceStatusInfo, prometheus.GaugeValue, 1, provider.Slug, service.Slug, service.Name, serviceStatus)
		}
	}
}

func (c *Collector) Refresh(ctx context.Context) map[string]error {
	type result struct {
		slug     string
		provider outagedeck.Provider
		duration float64
		attempt  time.Time
		err      error
	}

	results := make(chan result, len(c.providers))
	var group sync.WaitGroup
	for _, slug := range c.providers {
		group.Add(1)
		go func() {
			defer group.Done()
			started := time.Now()
			provider, err := c.client.FetchProvider(ctx, slug)
			results <- result{
				slug: slug, provider: provider, duration: time.Since(started).Seconds(),
				attempt: time.Now(), err: err,
			}
		}()
	}
	group.Wait()
	close(results)

	errorsByProvider := make(map[string]error)
	c.mu.Lock()
	defer c.mu.Unlock()
	for result := range results {
		state := c.states[result.slug]
		state.duration = result.duration
		state.lastAttempt = result.attempt
		state.err = result.err
		if result.err != nil {
			state.success = 0
			errorsByProvider[result.slug] = result.err
		} else {
			state.success = 1
			provider := result.provider
			state.provider = &provider
		}
		c.states[result.slug] = state
	}
	return errorsByProvider
}

func (c *Collector) HasSuccessfulSnapshot() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, state := range c.states {
		if state.provider != nil {
			return true
		}
	}
	return false
}

func normalizeStatus(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "operational":
		return "operational"
	case "maintenance", "under_maintenance", "scheduled_maintenance":
		return "maintenance"
	case "degraded", "degraded_performance":
		return "degraded"
	case "partial_outage", "outage":
		return "partial_outage"
	case "major_outage":
		return "major_outage"
	default:
		return "unknown"
	}
}

func StatusCode(status string) float64 {
	switch normalizeStatus(status) {
	case "operational":
		return 1
	case "maintenance":
		return 2
	case "degraded":
		return 3
	case "partial_outage":
		return 4
	case "major_outage":
		return 5
	default:
		return 0
	}
}
