package jobs_find

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Name          string   `json:"name,omitempty" jsonschema:"filter by job name"`
	NameMatchMode string   `json:"nameMatchMode,omitempty" jsonschema:"name match mode: EXACT or ANYWHERE"`
	Result        []string `json:"result,omitempty" jsonschema:"filter by result: NOT_SET, SUCCESS, COMPLETED_WITH_ERROR, FAILURE, ABORTED"`
	State         []string `json:"state,omitempty" jsonschema:"filter by state: WAITING, RUNNING, COMPLETED, FAILED, DELETED"`
	Type          []string `json:"type,omitempty" jsonschema:"filter by job type"`
	User          string   `json:"user,omitempty" jsonschema:"filter by user who created the job"`
	SortField     string   `json:"sortField,omitempty" jsonschema:"field to sort by"`
	SortOrder     string   `json:"sortOrder,omitempty" jsonschema:"sort order: ASC or DESC"`
	Cursor        string   `json:"cursor,omitempty" jsonschema:"pagination cursor from previous response"`
	PageSize      int      `json:"pageSize,omitempty" jsonschema:"number of results per page"`
}

type Output struct {
	Jobs       []clients.Job `json:"jobs,omitempty" jsonschema:"list of matching jobs"`
	Count      int             `json:"count" jsonschema:"number of jobs returned"`
	NextCursor string          `json:"nextCursor,omitempty" jsonschema:"cursor for the next page of results"`
	Error      string          `json:"error,omitempty" jsonschema:"error message if retrieval failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "jobs_find",
		Description: "Finds jobs matching the given filter criteria in the Collibra Jobs API.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		resp, err := clients.FindJobs(ctx, collibraClient, input.Name, input.NameMatchMode, input.Result, input.State, input.Type, input.User, input.SortField, input.SortOrder, input.Cursor, input.PageSize)
		if err != nil {
			return Output{Error: fmt.Sprintf("failed to find jobs: %s", err.Error())}, nil
		}
		return Output{Jobs: resp.Results, Count: len(resp.Results), NextCursor: resp.NextCursor}, nil
	}
}
