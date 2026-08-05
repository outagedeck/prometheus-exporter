package exporter

import (
	"context"
	"errors"
	"testing"

	"github.com/outagedeck/prometheus-exporter/internal/outagedeck"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type fakeClient struct {
	providers map[string]outagedeck.Provider
	errors    map[string]error
}

func (c fakeClient) FetchProvider(_ context.Context, slug string) (outagedeck.Provider, error) {
	if err := c.errors[slug]; err != nil {
		return outagedeck.Provider{}, err
	}
	return c.providers[slug], nil
}

func TestStatusCode(t *testing.T) {
	t.Parallel()

	tests := map[string]float64{
		"unknown": 0, "operational": 1, "maintenance": 2,
		"degraded_performance": 3, "partial_outage": 4, "major_outage": 5,
	}
	for status, expected := range tests {
		if actual := StatusCode(status); actual != expected {
			t.Errorf("StatusCode(%q) = %v; want %v", status, actual, expected)
		}
	}
}

func TestCollectorRefreshAndCollect(t *testing.T) {
	t.Parallel()

	client := fakeClient{providers: map[string]outagedeck.Provider{
		"github": {
			Slug: "github", Name: "GitHub",
			CurrentStatus: outagedeck.CurrentStatus{Code: "degraded"},
			Source:        outagedeck.Source{CheckedAt: "2026-08-05T00:00:00Z"},
			Counts:        outagedeck.Counts{ActiveIncidents: 1},
			Services: []outagedeck.Service{
				{Slug: "github-api", Name: "GitHub API", Status: "partial_outage"},
			},
		},
	}}
	collector := NewCollector(client, []string{"github"})
	if failures := collector.Refresh(context.Background()); len(failures) != 0 {
		t.Fatalf("unexpected failures: %v", failures)
	}
	if !collector.HasSuccessfulSnapshot() {
		t.Fatal("expected successful snapshot")
	}

	registry := prometheus.NewPedanticRegistry()
	registry.MustRegister(collector)
	metricCount, err := testutil.GatherAndCount(
		registry,
		"outagedeck_provider_status_code",
		"outagedeck_provider_active_incidents",
		"outagedeck_service_status_code",
		"outagedeck_scrape_success",
	)
	if err != nil {
		t.Fatal(err)
	}
	if metricCount != 4 {
		t.Fatalf("gathered %d metric families; want 4", metricCount)
	}
	if value := testutil.ToFloat64(prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: "test"}, func() float64 {
		return StatusCode(client.providers["github"].CurrentStatus.Code)
	})); value != 3 {
		t.Fatalf("provider status code = %v; want 3", value)
	}
}

func TestCollectorRetainsLastGoodSnapshot(t *testing.T) {
	t.Parallel()

	client := &fakeClient{providers: map[string]outagedeck.Provider{
		"github": {Slug: "github", Name: "GitHub", CurrentStatus: outagedeck.CurrentStatus{Code: "operational"}},
	}, errors: map[string]error{}}
	collector := NewCollector(client, []string{"github"})
	collector.Refresh(context.Background())
	client.errors["github"] = errors.New("temporary failure")
	if failures := collector.Refresh(context.Background()); failures["github"] == nil {
		t.Fatal("expected refresh failure")
	}
	if !collector.HasSuccessfulSnapshot() {
		t.Fatal("last good snapshot should be retained")
	}
}
