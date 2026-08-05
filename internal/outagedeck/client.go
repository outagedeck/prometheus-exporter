package outagedeck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const DefaultAPIBaseURL = "https://outagedeck.com/api/v1"

type CurrentStatus struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Headline string `json:"headline"`
	Summary  string `json:"summary"`
}

type Source struct {
	CheckedAt   string `json:"checkedAt"`
	OfficialURL string `json:"officialUrl"`
}

type Counts struct {
	ActiveIncidents int `json:"activeIncidents"`
}

type Service struct {
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type Provider struct {
	Slug          string        `json:"slug"`
	Name          string        `json:"name"`
	CurrentStatus CurrentStatus `json:"currentStatus"`
	Source        Source        `json:"source"`
	Counts        Counts        `json:"counts"`
	Services      []Service     `json:"services"`
}

type providerEnvelope struct {
	Data Provider `json:"data"`
}

type errorEnvelope struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
	Message string `json:"message"`
}

type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	userAgent  string
}

func NewClient(baseURL, apiKey string, timeout time.Duration, version string) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = DefaultAPIBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, fmt.Errorf("invalid OutageDeck API base URL: %q", baseURL)
	}
	if timeout <= 0 {
		return nil, errors.New("HTTP timeout must be greater than zero")
	}
	if strings.TrimSpace(version) == "" {
		version = "dev"
	}
	return &Client{
		baseURL:    baseURL,
		apiKey:     strings.TrimSpace(apiKey),
		httpClient: &http.Client{Timeout: timeout},
		userAgent:  "outagedeck-prometheus-exporter/" + version + " (+https://github.com/outagedeck/prometheus-exporter)",
	}, nil
}

func (c *Client) FetchProvider(ctx context.Context, slug string) (Provider, error) {
	endpoint := c.baseURL + "/providers/" + url.PathEscape(slug)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Provider{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}

	response, err := c.httpClient.Do(req)
	if err != nil {
		return Provider{}, fmt.Errorf("request provider %s: %w", slug, err)
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		var payload errorEnvelope
		_ = json.Unmarshal(body, &payload)
		message := payload.Error.Message
		if message == "" {
			message = payload.Message
		}
		if message == "" {
			message = response.Status
		}
		return Provider{}, fmt.Errorf("OutageDeck API returned %s: %s", response.Status, message)
	}

	var payload providerEnvelope
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return Provider{}, fmt.Errorf("decode OutageDeck response: %w", err)
	}
	if payload.Data.Slug == "" || payload.Data.Name == "" || payload.Data.CurrentStatus.Code == "" {
		return Provider{}, errors.New("OutageDeck API returned an incomplete provider response")
	}
	return payload.Data, nil
}
