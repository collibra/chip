package tools_test

import (
	"context"
	"log"
	"net/http"
	"slices"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/skills"
	"github.com/collibra/chip/pkg/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const debugToolName = "get_debug_mcp_init_request"

func TestRegisterAll_DebugToolHiddenByDefault(t *testing.T) {
	names := listToolNames(t, &chip.ServerToolConfig{})
	if slices.Contains(names, debugToolName) {
		t.Fatalf("expected %q to be absent when EnableDebugTools=false; got tools=%v", debugToolName, names)
	}
}

func TestRegisterAll_DebugToolVisibleWhenEnabled(t *testing.T) {
	names := listToolNames(t, &chip.ServerToolConfig{EnableDebugTools: true})
	if !slices.Contains(names, debugToolName) {
		t.Fatalf("expected %q to be present when EnableDebugTools=true; got tools=%v", debugToolName, names)
	}
}

// dataQualityToolNames are the tools gated behind the data-quality
// capability flag, in registration order.
var dataQualityToolNames = []string{
	"create_data_quality_job",
	"create_data_quality_rule",
	"get_data_quality_rule",
	"get_data_quality_rule_results",
	"validate_data_quality_rule",
	"list_data_quality_rule_templates",
	"get_data_quality_rule_template",
	"deploy_data_quality_rule_template",
	"generate_data_quality_rule_sql",
	"find_data_quality_rules",
	"dq_cancel_job_run",
	"dq_delete_job_run",
	"dq_delete_job",
	"dq_update_job",
	"dq_get_job",
	"dq_get_job_run",
	"dq_search_jobs",
	"dq_search_job_runs",
}

func TestRegisterAll_DataQualityToolsHiddenByDefault(t *testing.T) {
	names := listToolNames(t, &chip.ServerToolConfig{})
	for _, name := range dataQualityToolNames {
		if slices.Contains(names, name) {
			t.Errorf("expected %q to be absent when DataQuality=false", name)
		}
	}
}

func TestRegisterAll_DataQualityToolsVisibleWhenEnabled(t *testing.T) {
	names := listToolNames(t, &chip.ServerToolConfig{DataQuality: true})
	for _, name := range dataQualityToolNames {
		if !slices.Contains(names, name) {
			t.Errorf("expected %q to be present when DataQuality=true", name)
		}
	}
}

// TestRegisterAll_DataQualityFlagOnlyMovesDataQualityTools checks the wrapper
// did not swallow a neighbouring registration: the non-DQ surface must be
// identical in both states, asserted by name membership in both directions.
func TestRegisterAll_DataQualityFlagOnlyMovesDataQualityTools(t *testing.T) {
	off := listToolNames(t, &chip.ServerToolConfig{})
	on := listToolNames(t, &chip.ServerToolConfig{DataQuality: true})

	for _, name := range off {
		if !slices.Contains(on, name) {
			t.Errorf("tool %q is registered with DataQuality=false but not with DataQuality=true", name)
		}
	}
	for _, name := range on {
		if slices.Contains(dataQualityToolNames, name) {
			continue
		}
		if !slices.Contains(off, name) {
			t.Errorf("non-DQ tool %q is only registered with DataQuality=true", name)
		}
	}
	if len(on) != len(off)+len(dataQualityToolNames) {
		t.Errorf("DataQuality=true registered %d tools, DataQuality=false %d; expected a difference of exactly %d",
			len(on), len(off), len(dataQualityToolNames))
	}
}

// search_catalog_columns sits among the DQ registrations but is a Knowledge
// Graph search over catalog Column assets; it is not gated.
func TestRegisterAll_SearchCatalogColumnsIsNotGated(t *testing.T) {
	for _, cfg := range []*chip.ServerToolConfig{{}, {DataQuality: true}} {
		names := listToolNames(t, cfg)
		if !slices.Contains(names, "search_catalog_columns") {
			t.Errorf("expected search_catalog_columns to be registered with DataQuality=%v", cfg.DataQuality)
		}
	}
}

// The allow-list filters within the enabled capabilities; it cannot reach a
// tool inside a closed gate, because such a tool is never offered to
// toolRegister (and therefore never reaches IsToolEnabled).
func TestRegisterAll_EnabledToolsCannotReopenDataQualityGate(t *testing.T) {
	names := listToolNames(t, &chip.ServerToolConfig{
		EnabledTools: []string{"create_data_quality_rule", "dq_delete_job"},
	})
	for _, name := range dataQualityToolNames {
		if slices.Contains(names, name) {
			t.Errorf("expected %q to stay absent when named in enabled-tools with DataQuality=false", name)
		}
	}
}

// data-quality is a capability, not an experimental feature name: enabling it
// as one must have no effect on registration.
func TestRegisterAll_DataQualityAsExperimentalFeatureRegistersNothing(t *testing.T) {
	names := listToolNames(t, &chip.ServerToolConfig{
		Experimental: []string{chip.DataQualityCapabilityName},
	})
	for _, name := range dataQualityToolNames {
		if slices.Contains(names, name) {
			t.Errorf("expected %q to be absent when %q is passed as an experimental feature",
				name, chip.DataQualityCapabilityName)
		}
	}
}

func TestRegisterAll_AllToolsHaveProperAnnotations(t *testing.T) {
	// Every gate on, so a feature-flagged tool can't skip the annotation check.
	cfg := &chip.ServerToolConfig{
		EnableDebugTools: true,
		DataQuality:      true,
		Experimental:     []string{tools.ContextSpecificationsFeature, skills.FeatureName},
	}
	for _, tool := range listTools(t, cfg) {
		if tool.Title == "" {
			t.Errorf("tool %q has no title", tool.Name)
		}
		if tool.Annotations == nil {
			t.Errorf("tool %q has no annotations", tool.Name)
			continue
		}
		if tool.Annotations.DestructiveHint == nil {
			t.Errorf("tool %q does not set DestructiveHint explicitly", tool.Name)
		}
		if tool.Annotations.OpenWorldHint == nil {
			t.Errorf("tool %q does not set OpenWorldHint explicitly", tool.Name)
		}
		if tool.Annotations.ReadOnlyHint && tool.Annotations.DestructiveHint != nil && *tool.Annotations.DestructiveHint {
			t.Errorf("tool %q is read-only but marked destructive", tool.Name)
		}
		if tool.Annotations.ReadOnlyHint && !tool.Annotations.IdempotentHint {
			t.Errorf("tool %q is read-only but not marked idempotent", tool.Name)
		}
	}
}

func listToolNames(t *testing.T, cfg *chip.ServerToolConfig) []string {
	t.Helper()
	var names []string
	for _, tool := range listTools(t, cfg) {
		names = append(names, tool.Name)
	}
	return names
}

func listTools(t *testing.T, cfg *chip.ServerToolConfig) []*mcp.Tool {
	t.Helper()
	server := chip.NewServer()
	if err := tools.RegisterAll(server, &http.Client{}, cfg); err != nil {
		t.Fatalf("RegisterAll failed: %v", err)
	}

	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := server.Connect(t.Context(), t1, nil); err != nil {
		log.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "v0.0.1"}, nil)
	session, err := client.Connect(t.Context(), t2, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = session.Close() }()

	var result []*mcp.Tool
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatalf("listing tools: %v", err)
		}
		result = append(result, tool)
	}
	return result
}
