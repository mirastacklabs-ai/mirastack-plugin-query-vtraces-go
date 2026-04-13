package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mirastacklabs-ai/mirastack-agents-sdk-go"
	"go.uber.org/zap"
)

// QueryVTracesPlugin queries VictoriaTraces using the Jaeger-compatible API.
// Absorbs both get_traces (Phase 1: compact summaries) and query_full_traces
// (Phase 2: full span waterfall) into a single plugin with action-based dispatch.
// The "v" prefix denotes Victoria-specific.
type QueryVTracesPlugin struct {
	client *VTracesClient
	engine *mirastack.EngineContext
	logger *zap.Logger
}

// SetEngineContext injects the engine callback context (pull model config).
func (p *QueryVTracesPlugin) SetEngineContext(ec *mirastack.EngineContext) {
	p.engine = ec
}

func (p *QueryVTracesPlugin) Info() *mirastack.PluginInfo {
	return &mirastack.PluginInfo{
		Name:    "query_vtraces",
		Version: "0.2.0",
		Description: "Search and retrieve distributed traces from VictoriaTraces via the Jaeger-compatible API. " +
			"Use this plugin to find traces by service, operation, tags, or duration; retrieve full span waterfalls; " +
			"discover instrumented services and operations; and analyze service dependency graphs. " +
			"Start with services to discover what is instrumented, then search for specific traces.",
		Permissions:  []mirastack.Permission{mirastack.PermissionRead},
		DevOpsStages: []mirastack.DevOpsStage{mirastack.StageObserve},
		Actions: []mirastack.Action{
			{
				ID: "search",
				Description: "Search distributed traces by service, operation, tags, and duration. " +
					"Use this for finding slow requests, error traces, or traces matching specific criteria. " +
					"Returns trace summaries with span counts and durations.",
				Permission: mirastack.PermissionRead,
				Stages:     []mirastack.DevOpsStage{mirastack.StageObserve},
				Intents: []mirastack.IntentPattern{
					{Pattern: "search traces", Description: "Search distributed traces", Priority: 10},
					{Pattern: "find slow traces", Description: "Find traces with high latency", Priority: 9},
					{Pattern: "find error traces", Description: "Find traces containing errors", Priority: 9},
					{Pattern: "traces for service", Description: "Search traces for a specific service", Priority: 8},
				},
				InputParams: []mirastack.ParamSchema{
					{Name: "service", Type: "string", Required: false, Description: "Service name to filter traces by"},
					{Name: "operation", Type: "string", Required: false, Description: "Operation/endpoint name to filter (e.g., 'GET /api/orders')"},
					{Name: "tags", Type: "string", Required: false, Description: "Key-value tags as 'k1=v1,k2=v2' to filter traces (e.g., 'http.status_code=500')"},
					{Name: "min_duration", Type: "string", Required: false, Description: "Minimum trace duration (e.g., 100ms, 1s)"},
					{Name: "max_duration", Type: "string", Required: false, Description: "Maximum trace duration (e.g., 5s, 10s)"},
					{Name: "limit", Type: "string", Required: false, Description: "Maximum number of traces to return (default: 50)"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Traces in Jaeger API response format"},
				},
			},
			{
				ID: "trace_by_id",
				Description: "Retrieve a specific trace by its trace ID including the full span waterfall. " +
					"Use this after identifying a trace of interest from search results " +
					"to see all spans, timing, parent-child relationships, and tag details.",
				Permission: mirastack.PermissionRead,
				Stages:     []mirastack.DevOpsStage{mirastack.StageObserve},
				Intents: []mirastack.IntentPattern{
					{Pattern: "get trace", Description: "Retrieve a trace by ID", Priority: 10},
					{Pattern: "trace details", Description: "Show full span details for a trace", Priority: 9},
					{Pattern: "span waterfall", Description: "Show the span waterfall for a trace", Priority: 8},
				},
				InputParams: []mirastack.ParamSchema{
					{Name: "trace_id", Type: "string", Required: true, Description: "Specific trace ID to retrieve (hex string, e.g., 'abc123def456')"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Full trace data with all spans in Jaeger format"},
				},
			},
			{
				ID: "services",
				Description: "List all services reporting distributed traces. " +
					"Use this for discovery — to find which services are instrumented with tracing. " +
					"This is typically the first call before searching traces.",
				Permission: mirastack.PermissionRead,
				Stages:     []mirastack.DevOpsStage{mirastack.StageObserve},
				Intents: []mirastack.IntentPattern{
					{Pattern: "traced services", Description: "List services with tracing enabled", Priority: 9},
					{Pattern: "which services have traces", Description: "Discover instrumented services", Priority: 8},
					{Pattern: "tracing coverage", Description: "Check which services report traces", Priority: 7},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Array of service names"},
				},
			},
			{
				ID: "operations",
				Description: "List all operations (endpoints, methods) for a specific service. " +
					"Use this to discover what API endpoints or internal operations a service exposes. " +
					"Useful before filtering trace search to a specific operation.",
				Permission: mirastack.PermissionRead,
				Stages:     []mirastack.DevOpsStage{mirastack.StageObserve},
				Intents: []mirastack.IntentPattern{
					{Pattern: "service operations", Description: "List operations for a traced service", Priority: 9},
					{Pattern: "endpoints for service", Description: "Find API endpoints of a service", Priority: 8},
					{Pattern: "what operations does", Description: "Discover operations a service handles", Priority: 7},
				},
				InputParams: []mirastack.ParamSchema{
					{Name: "service", Type: "string", Required: true, Description: "Service name to list operations for"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Array of operation names"},
				},
			},
			{
				ID: "dependencies",
				Description: "Analyze service dependency graph derived from trace data. " +
					"Shows which services call which other services and the call volume. " +
					"Use this for understanding service topology and identifying critical paths.",
				Permission: mirastack.PermissionRead,
				Stages:     []mirastack.DevOpsStage{mirastack.StageObserve, mirastack.StagePlan},
				Intents: []mirastack.IntentPattern{
					{Pattern: "trace dependencies", Description: "Show service dependencies from trace data", Priority: 9},
					{Pattern: "service call graph", Description: "Visualize service-to-service call relationships", Priority: 8},
					{Pattern: "which services call", Description: "Find upstream/downstream service dependencies", Priority: 7},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Service dependency graph with call counts"},
				},
			},
		},
		Intents: []mirastack.IntentPattern{
			{Pattern: "search traces", Description: "Search distributed traces", Priority: 10},
			{Pattern: "get trace", Description: "Retrieve trace by ID", Priority: 9},
			{Pattern: "find slow traces", Description: "Find traces with high latency", Priority: 8},
			{Pattern: "trace dependencies", Description: "Show service dependencies from traces", Priority: 7},
			{Pattern: "distributed tracing", Description: "Work with distributed trace data", Priority: 7},
			{Pattern: "span details", Description: "View trace span details", Priority: 6},
			{Pattern: "trace latency", Description: "Analyze request latency via traces", Priority: 6},
		},
		PromptTemplates: []mirastack.PromptTemplate{
			{
				Name:        "query_vtraces_guide",
				Description: "Best practices for using VictoriaTraces distributed tracing tools",
				Content: `You have access to VictoriaTraces distributed tracing tools. Follow these guidelines:

1. DISCOVERY FIRST: Use services action to find instrumented services. Then operations to find endpoints.
2. SEARCH STRATEGY: Start broad (service only), then narrow with operation, tags, and duration filters.
3. LATENCY ANALYSIS: Use min_duration filter to find slow traces (e.g., min_duration=1s).
4. ERROR INVESTIGATION: Filter by tags like http.status_code=500 or error=true.
5. TRACE DEEP DIVE: After search, use trace_by_id to get the full span waterfall for a specific trace.
6. DEPENDENCIES: Use dependencies action to understand service topology before investigating issues.
7. TAG FILTERING: Tags use key=value format. Common tags: http.method, http.status_code, error, db.type.
8. LIMIT results initially: start with limit=10, increase to 50+ for broader analysis.
9. INTERPRETATION:
   - Short traces with errors = fast failure (connection refused, auth failure)
   - Long traces with many spans = cascading slowness
   - Missing spans = instrumentation gaps
   - Fan-out patterns = potential N+1 query issues`,
			},
		},
		ConfigParams: []mirastack.ConfigParam{
			{Key: "traces_url", Type: "string", Required: true, Description: "VictoriaTraces base URL (e.g. http://victoriatraces:10428)"},
		},
	}
}

func (p *QueryVTracesPlugin) Schema() *mirastack.PluginSchema {
	info := p.Info()
	return &mirastack.PluginSchema{
		Actions: info.Actions,
	}
}

func (p *QueryVTracesPlugin) Execute(ctx context.Context, req *mirastack.ExecuteRequest) (*mirastack.ExecuteResponse, error) {
	if p.logger == nil {
		p.logger, _ = zap.NewProduction()
	}

	action := req.ActionID
	if action == "" {
		action = req.Params["action"]
	}
	if action == "" {
		resp, _ := mirastack.RespondError("action parameter is required")
		resp.Logs = []string{"missing required parameter: action"}
		return resp, nil
	}

	result, err := p.dispatch(ctx, action, req.Params, req.TimeRange)
	if err != nil {
		resp, _ := mirastack.RespondError(err.Error())
		resp.Logs = []string{fmt.Sprintf("action %s failed: %v", action, err)}
		return resp, nil
	}

	resp, _ := mirastack.RespondMap(enrichTracesOutput(action, result))
	resp.Logs = []string{fmt.Sprintf("action %s completed", action)}
	return resp, nil
}

func (p *QueryVTracesPlugin) dispatch(ctx context.Context, action string, params map[string]string, tr *mirastack.TimeRange) (string, error) {
	if p.client == nil {
		return "", fmt.Errorf("plugin not configured: traces_url not set")
	}

	switch action {
	case "search":
		return p.actionSearch(ctx, params, tr)
	case "trace_by_id":
		return p.actionTraceByID(ctx, params)
	case "services":
		return p.actionServices(ctx)
	case "operations":
		return p.actionOperations(ctx, params)
	case "dependencies":
		return p.actionDependencies(ctx, params, tr)
	default:
		return "", fmt.Errorf("unknown action: %s", action)
	}
}

func (p *QueryVTracesPlugin) HealthCheck(ctx context.Context) error {
	// Pull config from engine (cached 15s in SDK)
	if p.engine != nil {
		config, err := p.engine.GetConfig(ctx)
		if err == nil {
			p.applyConfig(config)
		}
	}
	if p.client == nil {
		return fmt.Errorf("not configured")
	}
	_, err := p.client.Services(ctx)
	return err
}

func (p *QueryVTracesPlugin) ConfigUpdated(_ context.Context, config map[string]string) error {
	p.applyConfig(config)
	return nil
}

func (p *QueryVTracesPlugin) applyConfig(config map[string]string) {
	if url, ok := config["traces_url"]; ok && url != "" {
		p.client = NewVTracesClient(url)
		if p.logger != nil {
			p.logger.Info("VictoriaTraces client updated", zap.String("url", url))
		}
	}
}

// enrichTracesOutput wraps raw trace query results with metadata for LLM consumption.
func enrichTracesOutput(action, raw string) map[string]any {
	out := map[string]any{
		"action": action,
		"result": raw,
	}

	const maxLen = 32000
	if len(raw) > maxLen {
		out["result"] = raw[:maxLen]
		out["truncated"] = true
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		// Jaeger API wraps results in {"data": [...]}
		if data, ok := parsed["data"]; ok {
			switch d := data.(type) {
			case []any:
				out["result_count"] = len(d)
				// For search results, extract unique services.
				if action == "search" {
					services := map[string]bool{}
					for _, t := range d {
						if trace, ok := t.(map[string]any); ok {
							if procs, ok := trace["processes"].(map[string]any); ok {
								for _, proc := range procs {
									if pm, ok := proc.(map[string]any); ok {
										if sn, ok := pm["serviceName"].(string); ok {
											services[sn] = true
										}
									}
								}
							}
						}
					}
					if len(services) > 0 {
						names := make([]string, 0, len(services))
						for s := range services {
							names = append(names, s)
						}
						out["services_found"] = names
					}
				}
			}
		}
	}

	return out
}
