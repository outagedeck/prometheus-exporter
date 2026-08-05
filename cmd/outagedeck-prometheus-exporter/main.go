package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/outagedeck/prometheus-exporter/internal/exporter"
	"github.com/outagedeck/prometheus-exporter/internal/outagedeck"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

type config struct {
	listenAddress   string
	metricsPath     string
	providers       []string
	refreshInterval time.Duration
	requestTimeout  time.Duration
	apiBaseURL      string
	apiKey          string
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("outagedeck-prometheus-exporter", flag.ContinueOnError)
	flags.SetOutput(stderr)
	listenAddress := flags.String("web.listen-address", envOr("OUTAGEDECK_LISTEN_ADDRESS", ":9787"), "address on which to expose metrics")
	metricsPath := flags.String("web.telemetry-path", envOr("OUTAGEDECK_METRICS_PATH", "/metrics"), "path under which to expose metrics")
	providersValue := flags.String("providers", envOr("OUTAGEDECK_PROVIDERS", "github,aws,openai"), "comma-separated OutageDeck provider slugs")
	refreshInterval := flags.Duration("refresh-interval", envDuration("OUTAGEDECK_REFRESH_INTERVAL", 5*time.Minute), "interval between OutageDeck API refreshes")
	requestTimeout := flags.Duration("request-timeout", envDuration("OUTAGEDECK_REQUEST_TIMEOUT", 10*time.Second), "timeout for each OutageDeck API request")
	apiBaseURL := flags.String("api-base-url", envOr("OUTAGEDECK_API_BASE_URL", outagedeck.DefaultAPIBaseURL), "OutageDeck API base URL")
	apiKey := flags.String("api-key", os.Getenv("OUTAGEDECK_API_KEY"), "optional OutageDeck API key")
	showVersion := flags.Bool("version", false, "print version and exit")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *showVersion {
		fmt.Fprintf(stdout, "outagedeck-prometheus-exporter %s (commit %s, built %s)\n", version, commit, date)
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments: %s", strings.Join(flags.Args(), " "))
	}

	providers, err := normalizeProviders(*providersValue)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(*metricsPath, "/") || *metricsPath == "/" || *metricsPath == "/healthz" || *metricsPath == "/readyz" {
		return errors.New("--web.telemetry-path must be a distinct absolute path")
	}
	if *refreshInterval < 30*time.Second {
		return errors.New("--refresh-interval must be at least 30 seconds")
	}
	if *requestTimeout <= 0 {
		return errors.New("--request-timeout must be greater than zero")
	}

	cfg := config{
		listenAddress: *listenAddress, metricsPath: *metricsPath, providers: providers,
		refreshInterval: *refreshInterval, requestTimeout: *requestTimeout,
		apiBaseURL: *apiBaseURL, apiKey: strings.TrimSpace(*apiKey),
	}
	return serve(cfg, stderr)
}

func serve(cfg config, logOutput io.Writer) error {
	logger := slog.New(slog.NewJSONHandler(logOutput, nil))
	client, err := outagedeck.NewClient(cfg.apiBaseURL, cfg.apiKey, cfg.requestTimeout, version)
	if err != nil {
		return err
	}
	collector := exporter.NewCollector(client, cfg.providers)

	refresh := func(ctx context.Context) {
		failures := collector.Refresh(ctx)
		for provider, refreshErr := range failures {
			logger.Error("provider refresh failed", "provider", provider, "error", refreshErr)
		}
		logger.Info("provider refresh completed", "providers", len(cfg.providers), "successful", len(cfg.providers)-len(failures))
	}
	refresh(context.Background())

	registry := prometheus.NewRegistry()
	registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}), collector)
	buildInfo := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "outagedeck_exporter_build_info",
		Help: "Build information for the OutageDeck Prometheus exporter.",
	}, []string{"version", "commit", "date"})
	buildInfo.WithLabelValues(version, commit, date).Set(1)
	registry.MustRegister(buildInfo)

	mux := http.NewServeMux()
	mux.Handle(cfg.metricsPath, promhttp.HandlerFor(registry, promhttp.HandlerOpts{EnableOpenMetrics: true}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, "{\"status\":\"ok\"}\n")
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if !collector.HasSuccessfulSnapshot() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, "{\"status\":\"not_ready\"}\n")
			return
		}
		_, _ = io.WriteString(w, "{\"status\":\"ready\"}\n")
	})
	mux.HandleFunc("/", landingHandler(cfg.metricsPath))

	server := &http.Server{
		Addr: cfg.listenAddress, Handler: mux,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 2 * time.Minute,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		ticker := time.NewTicker(cfg.refreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				refresh(ctx)
			}
		}
	}()

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("exporter listening", "address", cfg.listenAddress, "metrics_path", cfg.metricsPath, "providers", cfg.providers)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case serverErr := <-serverErrors:
		if !errors.Is(serverErr, http.ErrServerClosed) {
			return serverErr
		}
		return nil
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(shutdownContext)
	}
}

func normalizeProviders(value string) ([]string, error) {
	seen := make(map[string]bool)
	providers := make([]string, 0)
	for _, raw := range strings.Split(value, ",") {
		slug := strings.ToLower(strings.TrimSpace(raw))
		if slug == "" || seen[slug] {
			continue
		}
		for index, char := range slug {
			valid := char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-'
			if !valid || char == '-' && (index == 0 || index == len(slug)-1) {
				return nil, fmt.Errorf("invalid provider slug: %s", slug)
			}
		}
		seen[slug] = true
		providers = append(providers, slug)
	}
	if len(providers) == 0 {
		return nil, errors.New("provide at least one provider slug")
	}
	if len(providers) > 20 {
		return nil, errors.New("at most 20 providers can be monitored by one exporter")
	}
	return providers, nil
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func envDuration(name string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func landingHandler(metricsPath string) http.HandlerFunc {
	page := template.Must(template.New("landing").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><title>OutageDeck Prometheus Exporter</title></head>
<body><main><h1>OutageDeck Prometheus Exporter</h1><p><a href="{{.MetricsPath}}">Metrics</a></p>
<p>Cloud and SaaS status data from <a href="https://outagedeck.com/?utm_source=prometheus&amp;utm_medium=integration&amp;utm_campaign=prometheus_exporter">OutageDeck</a>.</p></main></body></html>`))
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = page.Execute(w, map[string]string{"MetricsPath": metricsPath})
	}
}
