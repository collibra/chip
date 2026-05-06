package dry_run_control_query

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Query       map[string]any `json:"query" jsonschema:"Required. A ControlQuery object as defined in the Collibra Control Tower management API (version: '1.0', assetSelector, optional pathTraversalRequirement). The full schema lives in the create-control plugin's references/oas/control-tower-management-api.yaml."`
	SampleLimit int            `json:"sampleLimit,omitempty" jsonschema:"Optional. Number of failing-asset samples to return alongside the count. Default: 5."`
}

type Output struct {
	Result json.RawMessage `json:"result" jsonschema:"Raw response from POST /rest/controlExecution/v1/controlQueries/dryRun. Includes failingAssetsCount and (per the request) sampleFailedAssets. The shape is preserved verbatim so downstream consumers don't lose fields."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "dry_run_control_query",
		Description: "Execute a ControlQuery dry-run against the Control Tower execution service. Returns the failing-asset count plus a small sample of failing assets without persisting anything. Used by the create-control iterative loop.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		limit := input.SampleLimit
		if limit <= 0 {
			limit = 5
		}
		queryBytes, err := json.Marshal(input.Query)
		if err != nil {
			return Output{}, err
		}
		raw, err := clients.DryRunControlQuery(ctx, collibraClient, queryBytes, limit)
		if err != nil {
			return Output{}, err
		}
		return Output{Result: json.RawMessage(raw)}, nil
	}
}
