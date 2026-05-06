// Package find_status looks up a single status by publicId or exact
// name from the cached DGC catalog. Returns a resolve_domain-style
// envelope (match | candidates | notFound).
package find_status

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	PublicID string `json:"publicId,omitempty" jsonschema:"Optional. Status publicId. Exact match. At least one of publicId or name must be provided."`
	Name     string `json:"name,omitempty" jsonschema:"Optional. Status display name. Exact match (case-sensitive). At least one of publicId or name must be provided."`
}

type Output struct {
	Match      *Status  `json:"match,omitempty" jsonschema:"Set when exactly one status matches"`
	Candidates []Status `json:"candidates,omitempty" jsonschema:"Set when multiple statuses match the criteria; the caller must pick one"`
	NotFound   bool     `json:"notFound,omitempty" jsonschema:"True when no status matches"`
	Reason     string   `json:"reason,omitempty" jsonschema:"Explanation when match is empty (notFound or multi-match)"`
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
		Name:        "find_status",
		Description: "Find a single status by publicId or exact name. Returns match / candidates / notFound. Reads from the same one-hour cache as list_statuses.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.PublicID == "" && input.Name == "" {
			return Output{}, errors.New("at least one of publicId or name must be provided")
		}
		all, err := clients.ListAllStatuses(ctx, collibraClient)
		if err != nil {
			return Output{}, err
		}
		var matches []Status
		for _, s := range all {
			if input.PublicID != "" && s.PublicID != input.PublicID {
				continue
			}
			if input.Name != "" && s.Name != input.Name {
				continue
			}
			matches = append(matches, Status{
				ID:          s.ID,
				Name:        s.Name,
				Description: s.Description,
				PublicID:    s.PublicID,
				System:      s.System,
			})
		}
		switch len(matches) {
		case 0:
			return Output{NotFound: true, Reason: notFoundReason(input)}, nil
		case 1:
			m := matches[0]
			return Output{Match: &m}, nil
		default:
			return Output{Candidates: matches, Reason: "multiple matches; pick one by id and re-call with the publicId"}, nil
		}
	}
}

func notFoundReason(in Input) string {
	switch {
	case in.PublicID != "" && in.Name != "":
		return fmt.Sprintf("no status matches publicId=%q and name=%q", in.PublicID, in.Name)
	case in.PublicID != "":
		return fmt.Sprintf("no status matches publicId=%q", in.PublicID)
	default:
		return fmt.Sprintf("no status matches name=%q", in.Name)
	}
}