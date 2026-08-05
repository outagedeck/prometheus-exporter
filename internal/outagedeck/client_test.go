package outagedeck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchProvider(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/providers/github" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Fatal("missing API key")
		}
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "outagedeck-prometheus-exporter/0.1.0") {
			t.Fatalf("unexpected user agent: %s", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"slug":"github","name":"GitHub","currentStatus":{"code":"operational"},"source":{"checkedAt":"2026-08-05T00:00:00Z"},"counts":{"activeIncidents":0},"services":[{"slug":"github-api","name":"GitHub API","status":"operational"}]}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "test-key", time.Second, "0.1.0")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := client.FetchProvider(context.Background(), "github")
	if err != nil {
		t.Fatal(err)
	}
	if provider.Name != "GitHub" || len(provider.Services) != 1 {
		t.Fatalf("unexpected provider: %#v", provider)
	}
}

func TestFetchProviderError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limit exceeded"}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", time.Second, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.FetchProvider(context.Background(), "github")
	if err == nil || !strings.Contains(err.Error(), "rate limit exceeded") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewClientRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	if _, err := NewClient("file:///tmp/api", "", time.Second, "test"); err == nil {
		t.Fatal("expected invalid URL error")
	}
	if _, err := NewClient(DefaultAPIBaseURL, "", 0, "test"); err == nil {
		t.Fatal("expected invalid timeout error")
	}
}
