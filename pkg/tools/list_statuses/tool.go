// Package list_statuses exposes the cached DGC status catalog to MCP
// clients. The full catalog is fetched once per chip-binary lifetime
// (see clients/catalog_cache.go) and sliced server-side per request.
package list_statuses

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Limit  int `json:"limit,omitempty" jsonschema:"Optional. Maximum number of results to return. The maximum allowed limit is 1000. Default: 100."`
	Offset int `json:"offset,omitempty" jsonschema:"Optional. Index of first result (pagination offset). Default: 0."`
}

type Output struct {
	Total    int64    `json:"total" jsonschema:"The total number of statuses in the catalog"`
	Offset   int64    `json:"offset" jsonschema:"The offset for the results"`
	Limit    int64    `json:"limit" jsonschema:"The maximum number of results returned"`
	Statuses []Status `json:"statuses" jsonschema:"The list of statuses"`
}

type Status struct {
	ID          string `json:"id" jsonschema:"The unique identifier of the status"`
	Name        string `json:"name" jsonschema:"The display name of the status"`
	Description string `json:"description,omitempty" jsonschema:"The description of the status"`
	PublicID    string `json:"publicId,omitempty" jsonschema:"The public id of the status"`
	System      bool   `json:"system" jsonschema:"Whether this is a system status"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "list_statuses",
		Description: "List statuses available in Collibra. The full catalog is cached server-side for one hour, so repeated calls do not re-hit DGC.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.Limit <= 0 {
			input.Limit = 100
		}
		all, err := clients.ListAllStatuses(ctx, collibraClient)
		if err != nil {
			return Output{}, err
		}
		page := paginate(all, input.Offset, input.Limit)
		out := Output{
			Total:    int64(len(all)),
			Offset:   int64(input.Offset),
			Limit:    int64(input.Limit),
			Statuses: make([]Status, len(page)),
		}
		for i, s := range page {
			out.Statuses[i] = Status{
				ID:          s.ID,
				Name:        s.Name,
				Description: s.Description,
				PublicID:    s.PublicID,
				System:      s.System,
			}
		}
		return out, nil
	}
}

func paginate(all []clients.StatusDetails, offset, limit int) []clients.StatusDetails {
	if offset >= len(all) {
		return nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end]
}