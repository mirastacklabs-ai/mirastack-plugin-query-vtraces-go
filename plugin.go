package main

import (
	"context"
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
		Name:         "query_vtraces",
		Version:      "0.1.0",
		Description:  "Search and retrieve distributed traces from VictoriaTraces. Supports trace search by service/operation/tags, trace retrieval by ID, service listing, operation listing, and service dependency analysis via the Jaeger-compatible API.",
		Permissions:  []mirastack.Permission{mirastack.PermissionRead},
		DevOpsStages: []mirastack.DevOpsStage{mirastack.StageObserve},
		Actions: []mirastack.Action{
			{
				ID:          "search",
				Description: "Search distributed traces by service, operation, tags, and duration",
				Permission:  mirastack.PermissionRead,
				Stages:      []mirastack.DevOpsStage{mirastack.StageObserve},
				InputParams: []mirastack.ParamSchema{
					{Name: "service", Type: "string", Required: false, Description: "Service name to filter traces by"},
					{Name: "operation", Type: "string", Required: false, Description: "Operation/endpoint name to filter"},
					{Name: "tags", Type: "string", Required: false, Description: "Key-value tags as 'k1=v1,k2=v2' to filter traces"},
					{Name: "min_duration", Type: "string", Required: false, Description: "Minimum trace duration (e.g., 100ms, 1s)"},
					{Name: "max_duration", Type: "string", Required: false, Description: "Maximum trace duration (e.g., 5s, 10s)"},
					{Name: "limit", Type: "string", Required: false, Description: "Maximum number of traces to return (default: 50)"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Traces in Jaeger API response format"},
				},
			},
			{
				ID:          "trace_by_id",
				Description: "Retrieve a specific trace by its trace ID",
				Permission:  mirastack.PermissionRead,
				Stages:      []mirastack.DevOpsStage{mirastack.StageObserve},
				InputParams: []mirastack.ParamSchema{
					{Name: "trace_id", Type: "string", Required: true, Description: "Specific trace ID to retrieve"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Full trace data with all spans"},
				},
			},
			{
				ID:          "services",
				Description: "List all services reporting traces",
				Permission:  mirastack.PermissionRead,
				Stages:      []mirastack.DevOpsStage{mirastack.StageObserve},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Array of service names"},
				},
			},
			{
				ID:          "operations",
				Description: "List operations for a service",
				Permission:  mirastack.PermissionRead,
				Stages:      []mirastack.DevOpsStage{mirastack.StageObserve},
				InputParams: []mirastack.ParamSchema{
					{Name: "service", Type: "string", Required: true, Description: "Service name to list operations for"},
				},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Array of operation names"},
				},
			},
			{
				ID:          "dependencies",
				Description: "Analyze service dependencies from trace data",
				Permission:  mirastack.PermissionRead,
				Stages:      []mirastack.DevOpsStage{mirastack.StageObserve},
				OutputParams: []mirastack.ParamSchema{
					{Name: "result", Type: "json", Required: true, Description: "Service dependency graph"},
				},
			},
		},
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

	resp, _ := mirastack.RespondMap(map[string]any{"result": result})
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
