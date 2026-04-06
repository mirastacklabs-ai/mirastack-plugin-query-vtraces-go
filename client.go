package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// VTracesClient is an HTTP client for the VictoriaTraces Jaeger-compatible API.
// Base path: /select/jaeger/api
// Endpoints:
//   - GET /traces?service=...             — Search traces
//   - GET /traces/{traceID}               — Get trace by ID
//   - GET /services                       — List services
//   - GET /services/{service}/operations  — List operations for a service
//   - GET /dependencies?endTs=...         — Service dependencies
type VTracesClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewVTracesClient creates a client for a VictoriaTraces instance.
func NewVTracesClient(baseURL string) *VTracesClient {
	return &VTracesClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
}

// SearchTraces searches for traces by service, operation, tags, duration, and time range.
func (c *VTracesClient) SearchTraces(ctx context.Context, opts *TraceSearchOpts) (json.RawMessage, error) {
	params := url.Values{}
	if opts.Service != "" {
		params.Set("service", opts.Service)
	}
	if opts.Operation != "" {
		params.Set("operation", opts.Operation)
	}
	if opts.Tags != "" {
		params.Set("tags", opts.Tags)
	}
	if opts.MinDuration != "" {
		params.Set("minDuration", opts.MinDuration)
	}
	if opts.MaxDuration != "" {
		params.Set("maxDuration", opts.MaxDuration)
	}
	if opts.Start != "" {
		params.Set("start", opts.Start)
	}
	if opts.End != "" {
		params.Set("end", opts.End)
	}
	if opts.Limit != "" {
		params.Set("limit", opts.Limit)
	}
	return c.getJSON(ctx, "/select/jaeger/api/traces", params)
}

// GetTrace retrieves a single trace by its trace ID.
func (c *VTracesClient) GetTrace(ctx context.Context, traceID string) (json.RawMessage, error) {
	path := fmt.Sprintf("/select/jaeger/api/traces/%s", url.PathEscape(traceID))
	return c.getJSON(ctx, path, nil)
}

// Services returns all known service names.
func (c *VTracesClient) Services(ctx context.Context) (json.RawMessage, error) {
	return c.getJSON(ctx, "/select/jaeger/api/services", nil)
}

// Operations returns operations for a given service.
func (c *VTracesClient) Operations(ctx context.Context, service string) (json.RawMessage, error) {
	path := fmt.Sprintf("/select/jaeger/api/services/%s/operations", url.PathEscape(service))
	return c.getJSON(ctx, path, nil)
}

// Dependencies returns service dependencies for a given time range.
func (c *VTracesClient) Dependencies(ctx context.Context, endTs, lookback string) (json.RawMessage, error) {
	params := url.Values{}
	if endTs != "" {
		params.Set("endTs", endTs)
	}
	if lookback != "" {
		params.Set("lookback", lookback)
	}
	return c.getJSON(ctx, "/select/jaeger/api/dependencies", params)
}

// TraceSearchOpts holds parameters for trace search.
type TraceSearchOpts struct {
	Service     string
	Operation   string
	Tags        string // JSON-encoded key-value pairs: {"k1":"v1","k2":"v2"}
	MinDuration string
	MaxDuration string
	Start       string // microseconds since epoch
	End         string // microseconds since epoch
	Limit       string
}

// getJSON performs a GET request and returns the raw JSON body.
func (c *VTracesClient) getJSON(ctx context.Context, path string, params url.Values) (json.RawMessage, error) {
	u := c.baseURL + path
	if len(params) > 0 {
		u += "?" + params.Encode()
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("VictoriaTraces API error (HTTP %d): %s", resp.StatusCode, truncate(string(body), 512))
	}

	return json.RawMessage(body), nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
