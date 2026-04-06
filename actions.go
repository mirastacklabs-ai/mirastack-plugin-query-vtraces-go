package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	mirastack "github.com/mirastacklabs-ai/mirastack-agents-sdk-go"
	"github.com/mirastacklabs-ai/mirastack-agents-sdk-go/datetimeutils"
)

// Action handlers for the query_vtraces plugin.
// Each action maps to a VictoriaTraces Jaeger-compatible API endpoint.

func (p *QueryVTracesPlugin) actionSearch(ctx context.Context, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	service := params["service"]
	if service == "" {
		return "", fmt.Errorf("service parameter is required for search action")
	}

	opts := &TraceSearchOpts{
		Service:     service,
		Operation:   params["operation"],
		MinDuration: params["min_duration"],
		MaxDuration: params["max_duration"],
		Limit:       params["limit"],
	}
	if opts.Limit == "" {
		opts.Limit = "50"
	}

	// Convert tags from "k1=v1,k2=v2" format to Jaeger JSON tag format
	if tagsRaw := params["tags"]; tagsRaw != "" {
		tagMap := make(map[string]string)
		for _, pair := range strings.Split(tagsRaw, ",") {
			parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
			if len(parts) == 2 {
				tagMap[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
			}
		}
		if len(tagMap) > 0 {
			tagsJSON, _ := json.Marshal(tagMap)
			opts.Tags = string(tagsJSON)
		}
	}

	// Convert time parameters to microseconds since epoch for Jaeger API.
	// Prefer engine-parsed TimeRange; fall back to raw params.
	if tr != nil && tr.StartEpochMs > 0 {
		opts.Start = datetimeutils.FormatEpochMicros(tr.StartEpochMs)
		opts.End = datetimeutils.FormatEpochMicros(tr.EndEpochMs)
	} else {
		now := time.Now()
		opts.Start = strconv.FormatInt(now.Add(-time.Hour).UnixMicro(), 10)
		opts.End = strconv.FormatInt(now.UnixMicro(), 10)

		if s := params["start"]; s != "" {
			if t, err := time.Parse(time.RFC3339, s); err == nil {
				opts.Start = strconv.FormatInt(t.UnixMicro(), 10)
			}
		}
		if e := params["end"]; e != "" && !strings.EqualFold(e, "now") {
			if t, err := time.Parse(time.RFC3339, e); err == nil {
				opts.End = strconv.FormatInt(t.UnixMicro(), 10)
			}
		}
	}

	result, err := p.client.SearchTraces(ctx, opts)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func (p *QueryVTracesPlugin) actionTraceByID(ctx context.Context, params map[string]string) (string, error) {
	traceID := params["trace_id"]
	if traceID == "" {
		return "", fmt.Errorf("trace_id parameter is required for trace_by_id action")
	}
	result, err := p.client.GetTrace(ctx, traceID)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func (p *QueryVTracesPlugin) actionServices(ctx context.Context) (string, error) {
	result, err := p.client.Services(ctx)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func (p *QueryVTracesPlugin) actionOperations(ctx context.Context, params map[string]string) (string, error) {
	service := params["service"]
	if service == "" {
		return "", fmt.Errorf("service parameter is required for operations action")
	}
	result, err := p.client.Operations(ctx, service)
	if err != nil {
		return "", err
	}
	return string(result), nil
}

func (p *QueryVTracesPlugin) actionDependencies(ctx context.Context, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	var endTs, lookback string

	if tr != nil && tr.StartEpochMs > 0 {
		// Engine provided parsed time range — use SDK formatters
		endTs = datetimeutils.FormatEpochMillis(tr.EndEpochMs)
		lookback = datetimeutils.FormatLookbackMillis(tr.StartEpochMs, tr.EndEpochMs)
	} else {
		// Fall back to raw params
		now := time.Now()
		endTs = strconv.FormatInt(now.UnixMilli(), 10)
		lookback = "3600000" // 1 hour default

		if end := params["end"]; end != "" && !strings.EqualFold(end, "now") {
			if t, err := time.Parse(time.RFC3339, end); err == nil {
				endTs = strconv.FormatInt(t.UnixMilli(), 10)
			}
		}

		if start := params["start"]; start != "" {
			if t, err := time.Parse(time.RFC3339, start); err == nil {
				endTime, _ := strconv.ParseInt(endTs, 10, 64)
				lb := endTime - t.UnixMilli()
				if lb > 0 {
					lookback = strconv.FormatInt(lb, 10)
				}
			}
		}
	}

	result, err := p.client.Dependencies(ctx, endTs, lookback)
	if err != nil {
		return "", err
	}
	return string(result), nil
}
