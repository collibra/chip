package list_controls

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	DomainID string `json:"domainId,omitempty" jsonschema:"Optional. Filter to controls in a specific domain (UUID)."`
	Offset   int    `json:"offset,omitempty" jsonschema:"Optional. Page offset; default 0."`
	Limit    int    `json:"limit,omitempty" jsonschema:"Optional. Page size; the server caps this. Default: 50."`
}

type Output struct {
	Result map[string]any `json:"result" jsonschema:"Raw response from GET /rest/controlManagement/v1/controls. Contains a paged 'results' array of ManagedControl summaries plus paging metadata."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "list_controls",
		Description: "List Control Tower controls, optionally filtered by domain. Returns the management API page with 'results' carrying each control's id, name, severity, controlType, enabled flag, and saved query. Use this for browsing the existing control inventory.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		query := map[string]string{}
		if input.DomainID != "" {
			query["domainId"] = input.DomainID
		}
		if input.Offset > 0 {
			query["offset"] = strconv.Itoa(input.Offset)
		}
		limit := input.Limit
		if limit <= 0 {
			limit = 50
		}
		query["limit"] = strconv.Itoa(limit)

		raw, err := clients.ListControls(ctx, collibraClient, query)
		if err != nil {
			return Output{}, err
		}
		var result map[string]any
		if err := json.Unmarshal(raw, &result); err != nil {
			return Output{}, err
		}
		return Output{Result: result}, nil
	}
}
