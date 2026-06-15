package get_context_specification

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	ContextSpecificationId string `json:"contextSpecificationId" jsonschema:"Required. The UUID of the Context Specification to retrieve."`
}

type AssetType struct {
	PublicId string `json:"publicId" jsonschema:"The public ID of the asset type (e.g. 'Table')"`
	Name     string `json:"name,omitempty" jsonschema:"The display name of the asset type"`
}

type Output struct {
	ID             string    `json:"id" jsonschema:"The UUID of the Context Specification"`
	Name           string    `json:"name" jsonschema:"The display name of the Context Specification"`
	Description    string    `json:"description,omitempty" jsonschema:"Optional description of the Context Specification"`
	AssetType      AssetType `json:"assetType" jsonschema:"The asset type this Context Specification applies to"`
	MappingYaml    string    `json:"mappingYaml" jsonschema:"The raw YAML mapping configuration that defines how context is generated. Inspect this to understand which fields and metrics will be collected before executing get_context."`
	CreatedBy      string    `json:"createdBy" jsonschema:"UUID of the user who created this Context Specification"`
	CreatedOn      string    `json:"createdOn" jsonschema:"ISO-8601 timestamp when this Context Specification was created"`
	LastModifiedBy string    `json:"lastModifiedBy" jsonschema:"UUID of the user who last modified this Context Specification"`
	LastModifiedOn string    `json:"lastModifiedOn" jsonschema:"ISO-8601 timestamp of the last modification"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_context_specification",
		Title:       "Get Context Specification",
		Description: "Inspection tool. Returns the full Context Specification including the mappingYaml configuration, so you can understand which fields and metrics will be populated before executing get_context. Use this after list_context_specifications to examine a specific spec before running it against an asset.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("contextSpecificationId", input.ContextSpecificationId); err != nil {
			return Output{}, err
		}

		spec, err := clients.GetContextSpecification(ctx, collibraClient, input.ContextSpecificationId)
		if err != nil {
			return Output{}, err
		}

		return Output{
			ID:          spec.ID,
			Name:        spec.Name,
			Description: spec.Description,
			AssetType: AssetType{
				PublicId: spec.AssetType.PublicId,
				Name:     spec.AssetType.Name,
			},
			MappingYaml:    spec.MappingYaml,
			CreatedBy:      spec.CreatedBy,
			CreatedOn:      spec.CreatedOn,
			LastModifiedBy: spec.LastModifiedBy,
			LastModifiedOn: spec.LastModifiedOn,
		}, nil
	}
}
