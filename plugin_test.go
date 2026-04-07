package main

import (
	"encoding/json"
	"testing"
)

func TestInfo_HasPerActionIntents(t *testing.T) {
	p := &QueryVTracesPlugin{}
	info := p.Info()

	if info.Version != "0.2.0" {
		t.Errorf("expected version 0.2.0, got %s", info.Version)
	}

	for _, action := range info.Actions {
		if len(action.Intents) == 0 {
			t.Errorf("action %q has no per-action intents", action.ID)
		}
	}
}

func TestInfo_HasPromptTemplates(t *testing.T) {
	p := &QueryVTracesPlugin{}
	info := p.Info()

	if len(info.PromptTemplates) == 0 {
		t.Fatal("expected at least one PromptTemplate")
	}
	if info.PromptTemplates[0].Name != "query_vtraces_guide" {
		t.Errorf("expected template name query_vtraces_guide, got %s", info.PromptTemplates[0].Name)
	}
}

func TestInfo_PluginIntentsExpanded(t *testing.T) {
	p := &QueryVTracesPlugin{}
	info := p.Info()

	if len(info.Intents) < 5 {
		t.Errorf("expected >=5 plugin-level intents, got %d", len(info.Intents))
	}
}

func TestInfo_DependenciesHasPlanStage(t *testing.T) {
	p := &QueryVTracesPlugin{}
	info := p.Info()

	for _, action := range info.Actions {
		if action.ID == "dependencies" {
			hasPlan := false
			for _, stage := range action.Stages {
				if stage == 0 { // StagePlan
					hasPlan = true
				}
			}
			if !hasPlan {
				t.Error("dependencies action should include StagePlan")
			}
		}
	}
}

func TestEnrichTracesOutput_BasicFields(t *testing.T) {
	out := enrichTracesOutput("search", `{"data":[]}`)

	if out["action"] != "search" {
		t.Errorf("expected action=search, got %v", out["action"])
	}
	if out["result_count"] != 0 {
		t.Errorf("expected result_count=0, got %v", out["result_count"])
	}
}

func TestEnrichTracesOutput_ExtractsServices(t *testing.T) {
	raw := `{"data":[{"traceID":"abc","processes":{"p1":{"serviceName":"frontend"},"p2":{"serviceName":"backend"}}}]}`
	out := enrichTracesOutput("search", raw)

	if out["result_count"] != 1 {
		t.Errorf("expected result_count=1, got %v", out["result_count"])
	}
	services, ok := out["services_found"].([]string)
	if !ok {
		t.Fatal("expected services_found to be []string")
	}
	if len(services) != 2 {
		t.Errorf("expected 2 services, got %d", len(services))
	}
}

func TestEnrichTracesOutput_Truncation(t *testing.T) {
	long := make([]byte, 33000)
	for i := range long {
		long[i] = 'x'
	}
	out := enrichTracesOutput("search", string(long))

	if out["truncated"] != true {
		t.Error("expected truncated=true for oversized result")
	}
}

func TestEnrichTracesOutput_JSONMarshalable(t *testing.T) {
	out := enrichTracesOutput("services", `{"data":["svc-a","svc-b"]}`)

	_, err := json.Marshal(out)
	if err != nil {
		t.Errorf("enriched output not JSON-serializable: %v", err)
	}
}

func TestInfo_ActionDescriptionsEnriched(t *testing.T) {
	p := &QueryVTracesPlugin{}
	info := p.Info()

	for _, action := range info.Actions {
		if len(action.Description) < 50 {
			t.Errorf("action %q description too short (%d chars)", action.ID, len(action.Description))
		}
	}
}
