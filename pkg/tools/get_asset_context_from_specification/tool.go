package get_asset_context_from_specification

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	AssetId                string `json:"assetId" jsonschema:"Required. UUID of the asset to generate context for."`
	ContextSpecificationId string `json:"contextSpecificationId" jsonschema:"Required. UUID of the Context Specification to execute against the asset. Use list_context_specifications to discover available specs."`
	IncludeMetadata        bool   `json:"includeMetadata,omitempty" jsonschema:"Optional. When true, returns a JSON envelope with metadata (contextSpecificationName, assetType, generatedOn) alongside the YAML content. When false (default), returns the raw YAML content only. Most LLM pipelines should use false."`
}

type AssetType struct {
	PublicId string `json:"publicId" jsonschema:"The public ID of the asset type"`
	Name     string `json:"name,omitempty" jsonschema:"The display name of the asset type"`
}

type Output struct {
	Content                  string     `json:"content" jsonschema:"The generated YAML context. Contains the structured semantic context for the asset as defined by the Context Specification's mappingYaml."`
	AssetId                  string     `json:"assetId,omitempty" jsonschema:"The UUID of the asset context was generated for. Only present when includeMetadata is true."`
	ContextSpecificationId   string     `json:"contextSpecificationId,omitempty" jsonschema:"The UUID of the Context Specification used. Only present when includeMetadata is true."`
	ContextSpecificationName string     `json:"contextSpecificationName,omitempty" jsonschema:"The name of the Context Specification used. Only present when includeMetadata is true."`
	AssetType                *AssetType `json:"assetType,omitempty" jsonschema:"The asset type of the target asset. Only present when includeMetadata is true."`
	GeneratedOn              string     `json:"generatedOn,omitempty" jsonschema:"ISO-8601 UTC timestamp when the context was generated. Only present when includeMetadata is true."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_asset_context_from_specification",
		Title:       "Get Asset Context From Specification",
		Description: "Context generation execution tool. Executes a Context Specification against a specific asset and returns the generated context as structured YAML. This is the final step in the context workflow: use list_context_specifications to discover specs, optionally use get_context_specification to inspect the mapping, then call this tool to generate the context.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("assetId", input.AssetId); err != nil {
			return Output{}, err
		}
		if err := validation.UUID("contextSpecificationId", input.ContextSpecificationId); err != nil {
			return Output{}, err
		}

		result, err := clients.GenerateContext(ctx, collibraClient, input.AssetId, input.ContextSpecificationId, input.IncludeMetadata)
		if err != nil {
			return Output{}, err
		}

		output := Output{Content: result.Content}

		if result.Metadata != nil {
			m := result.Metadata
			output.AssetId = m.AssetId
			output.ContextSpecificationId = m.ContextSpecificationId
			output.ContextSpecificationName = m.ContextSpecificationName
			output.AssetType = &AssetType{
				PublicId: m.AssetType.PublicId,
				Name:     m.AssetType.Name,
			}
			output.GeneratedOn = m.GeneratedOn
		}

		return output, nil
	}
}
