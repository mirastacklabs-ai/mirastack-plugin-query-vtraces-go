package main

import (
	"context"
	"fmt"

	"github.com/mirastacklabs-ai/mirastack-sdk-go"
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
		Name:         "query_vtraces",
		Version:      "0.1.0",
		Description:  "Search and retrieve distributed traces from VictoriaTraces. Supports trace search by service/operation/tags, trace retrieval by ID, service listing, operation listing, and service dependency analysis via the Jaeger-compatible API.",
		Permissions:  []mirastack.Permission{mirastack.PermissionRead},
		DevOpsStages: []mirastack.DevOpsStage{mirastack.StageObserve},
		Intents: []mirastack.IntentPattern{
			{Pattern: "search traces", Description: "Search distributed traces", Priority: 10},
			{Pattern: "get trace", Description: "Retrieve trace by ID", Priority: 9},
			{Pattern: "trace dependencies", Description: "Show service dependencies from traces", Priority: 7},
			{Pattern: "find slow traces", Description: "Find traces with high latency", Priority: 8},
		},
		ConfigParams: []mirastack.ConfigParam{
			{Key: "traces_url", Type: "string", Required: true, Description: "VictoriaTraces base URL (e.g. http://victoriatraces:9411)"},
		},
	}
}

func (p *QueryVTracesPlugin) Schema() *mirastack.PluginSchema {
	return &mirastack.PluginSchema{
		InputParams: []mirastack.ParamSchema{
			{Name: "action", Type: "string", Required: true, Description: "One of: search, trace_by_id, services, operations, dependencies"},
			{Name: "service", Type: "string", Required: false, Description: "Service name to filter traces by"},
			{Name: "operation", Type: "string", Required: false, Description: "Operation/endpoint name to filter"},
			{Name: "trace_id", Type: "string", Required: false, Description: "Specific trace ID to retrieve (required for trace_by_id)"},
			{Name: "tags", Type: "string", Required: false, Description: "Key-value tags as 'k1=v1,k2=v2' to filter traces"},
			{Name: "min_duration", Type: "string", Required: false, Description: "Minimum trace duration (e.g., 100ms, 1s)"},
			{Name: "max_duration", Type: "string", Required: false, Description: "Maximum trace duration (e.g., 5s, 10s)"},
			{Name: "start", Type: "string", Required: false, Description: "Start time (RFC3339 or relative like -1h)"},
			{Name: "end", Type: "string", Required: false, Description: "End time (RFC3339 or 'now')"},
			{Name: "limit", Type: "string", Required: false, Description: "Maximum number of traces to return (default: 50)"},
		},
		OutputParams: []mirastack.ParamSchema{
			{Name: "result", Type: "json", Required: true, Description: "Query result in Jaeger API response format"},
		},
	}
}

func (p *QueryVTracesPlugin) Execute(ctx context.Context, req *mirastack.ExecuteRequest) (*mirastack.ExecuteResponse, error) {
	if p.logger == nil {
		p.logger, _ = zap.NewProduction()
	}

	action := req.Params["action"]
	if action == "" {
		return &mirastack.ExecuteResponse{
			Output: map[string]string{"error": "action parameter is required"},
			Logs:   []string{"missing required parameter: action"},
		}, nil
	}

	result, err := p.dispatch(ctx, action, req.Params, req.TimeRange)
	if err != nil {
		return &mirastack.ExecuteResponse{
			Output: map[string]string{"error": err.Error()},
			Logs:   []string{fmt.Sprintf("action %s failed: %v", action, err)},
		}, nil
	}

	return &mirastack.ExecuteResponse{
		Output: map[string]string{"result": result},
		Logs:   []string{fmt.Sprintf("action %s completed", action)},
	}, nil
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
