package find_workflow_definitions

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Name               string `json:"name,omitempty" jsonschema:"Optional. Partial name match of the workflow definition to search for."`
	DefinitionIDPhrase string `json:"definitionIdPhrase,omitempty" jsonschema:"Optional. Partial match on the workflow definition ID."`
	Description        string `json:"description,omitempty" jsonschema:"Optional. Partial match on the workflow definition description."`
	Enabled            *bool  `json:"enabled,omitempty" jsonschema:"Optional. Filter to only enabled (or only disabled) workflow definitions. Workflows must be enabled in order to be started."`
	Global             *bool  `json:"global,omitempty" jsonschema:"Optional. Filter to only global (or only non-global) workflow definitions."`
	Limit              int    `json:"limit,omitempty" jsonschema:"Optional. Maximum number of results to return. The maximum allowed limit is 1000. Default: 100."`
	Offset             int    `json:"offset,omitempty" jsonschema:"Optional. Index of first result (pagination offset). Default: 0."`
}

type Output struct {
	Total       int64                `json:"total" jsonschema:"The total number of workflow definitions matching the search criteria"`
	Offset      int64                `json:"offset" jsonschema:"The offset for the results"`
	Limit       int64                `json:"limit" jsonschema:"The maximum number of results returned"`
	Definitions []WorkflowDefinition `json:"definitions" jsonschema:"The list of matching workflow definitions"`
}

type WorkflowDefinition struct {
	ID                        string `json:"id" jsonschema:"The UUID of the workflow definition. Use this as workflowDefinitionId when starting a workflow."`
	Name                      string `json:"name,omitempty" jsonschema:"The display name of the workflow definition"`
	Description               string `json:"description,omitempty" jsonschema:"The description of the workflow definition"`
	ProcessID                 string `json:"processId,omitempty" jsonschema:"The process ID from the BPMN definition (stable across redeploys)"`
	Enabled                   bool   `json:"enabled" jsonschema:"Whether the workflow is enabled. Only enabled workflows can be started."`
	FormRequired              bool   `json:"formRequired" jsonschema:"Whether starting this workflow requires form input. If true, fetch the start form data to learn which properties to provide."`
	BusinessItemDiscriminator string `json:"businessItemDiscriminator,omitempty" jsonschema:"The resource type the workflow operates on: ASSET, DOMAIN, COMMUNITY, USER, or GLOBAL"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "find_workflow_definitions",
		Description: "Find workflow definitions in Collibra by name (partial match), ID phrase, description, or enabled/global flags. Returns matching definitions with the metadata needed to start a workflow (id, whether it is enabled, whether it requires form input, and which business item type it targets).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.Limit == 0 {
			input.Limit = 100
		}

		response, err := clients.FindWorkflowDefinitions(ctx, collibraClient, clients.WorkflowDefinitionsQueryParams{
			Name:               input.Name,
			DefinitionIDPhrase: input.DefinitionIDPhrase,
			Description:        input.Description,
			Enabled:            input.Enabled,
			Global:             input.Global,
			Limit:              input.Limit,
			Offset:             input.Offset,
		})
		if err != nil {
			return Output{}, err
		}

		definitions := make([]WorkflowDefinition, len(response.Results))
		for i, wd := range response.Results {
			definitions[i] = WorkflowDefinition{
				ID:                        wd.ID,
				Name:                      wd.Name,
				Description:               wd.Description,
				ProcessID:                 wd.ProcessID,
				Enabled:                   wd.Enabled,
				FormRequired:              wd.FormRequired,
				BusinessItemDiscriminator: wd.BusinessItemDiscriminator,
			}
		}

		return Output{
			Total:       response.Total,
			Offset:      response.Offset,
			Limit:       response.Limit,
			Definitions: definitions,
		}, nil
	}
}
