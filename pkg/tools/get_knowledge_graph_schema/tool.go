// Package get_knowledge_graph_schema implements the get_knowledge_graph_schema
// MCP tool: it returns the vocabulary available for search_knowledge_graph
// queries — every attribute and relation name, whether each is a scalar
// Attribute or a Relation, its data type, and a human description — merged
// from two preview Knowledge Graph REST endpoints (GET
// /rest/knowledgeGraph/v1/hackaton/descriptions and GET
// /rest/knowledgeGraph/v1/hackation/attributes — "hackation" is the real,
// live upstream path, not a typo on this side).
package get_knowledge_graph_schema

import (
	"context"
	"fmt"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a get_knowledge_graph_schema call.
type OutputStatus string

const (
	// StatusSuccess means the schema was fetched.
	StatusSuccess OutputStatus = "success"
	// StatusError means a downstream call failed (e.g. the knowledge graph endpoint is unavailable).
	StatusError OutputStatus = "error"
)

// Input is the tool's typed input. It takes no parameters.
type Input struct{}

// TypeRef is an asset type referenced as the source or target end of a
// Relation, with its description.
type TypeRef struct {
	PublicID    string `json:"publicId" jsonschema:"Asset type publicId, usable after @type: in a search_knowledge_graph query, e.g. @type:Table."`
	Description string `json:"description,omitempty" jsonschema:"Human-readable description, when known."`
}

// SchemaAttribute is one attribute or relation name usable in a
// search_knowledge_graph query.
type SchemaAttribute struct {
	Name          string    `json:"name" jsonschema:"The attribute or relation name as used in a search_knowledge_graph query token, e.g. the Attribute in 'Attribute:value' or the Rel in 'Rel.Attribute:value'."`
	Kind          string    `json:"kind" jsonschema:"'Attribute' for a filterable property, or 'Relation' for a traversable relationship type used in Rel.Attribute chaining."`
	AttributeType string    `json:"attributeType,omitempty" jsonschema:"The value's data type (e.g. String, Float, Boolean, LocalDateTime), when Kind is 'Attribute'."`
	Description   string    `json:"description,omitempty" jsonschema:"Human-readable description, when known."`
	SourceTypes   []TypeRef `json:"sourceTypes,omitempty" jsonschema:"When Kind is 'Relation': the asset type(s) observed at the source end of this relation, each with its description. Empty when no metadata is available for this relation — it may still work in a query, but its valid endpoints are unconfirmed."`
	TargetTypes   []TypeRef `json:"targetTypes,omitempty" jsonschema:"When Kind is 'Relation': the asset type(s) observed at the target end of this relation, each with its description. Empty when no metadata is available for this relation — it may still work in a query, but its valid endpoints are unconfirmed."`
}

// Output is the typed response.
type Output struct {
	Status     OutputStatus      `json:"status" jsonschema:"'success' when the schema was fetched; 'error' for downstream failures."`
	Message    string            `json:"message" jsonschema:"Human-readable summary."`
	Attributes []SchemaAttribute `json:"attributes,omitempty" jsonschema:"Every attribute and relation name usable in a search_knowledge_graph query."`
	Count      int               `json:"count" jsonschema:"Number of attributes/relations returned."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "get_knowledge_graph_schema",
		Title: "Get Knowledge Graph Query Schema",
		Description: "Return the vocabulary available for search_knowledge_graph queries: every attribute and relation name, " +
			"whether it is a scalar Attribute (used as attribute:value / Attribute:value) or a Relation (used for Rel.Attribute " +
			"multi-hop traversal), its data type, and a human description. " +
			"Always call this before composing any search_knowledge_graph query, even one that looks simple or uses a name you're " +
			"confident about — to confirm exact attribute/relation names and types rather than guessing them, especially the " +
			"uppercase/lowercase first letter, which search_knowledge_graph uses " +
			"to distinguish array attributes from scalar properties. Names are case-sensitive: use the exact casing returned here " +
			"verbatim in search_knowledge_graph, since a wrong-case name matches nothing and fails silently with zero results. " +
			"Takes no parameters. Read-only; no permissions required beyond standard catalog access.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, _ Input) (Output, error) {
		attributes, err := clients.GetKnowledgeGraphSchemaAttributes(ctx, collibraClient)
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not get the knowledge graph schema: %v", err)}, nil
		}
		descriptions, err := clients.GetKnowledgeGraphDescriptions(ctx, collibraClient)
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not get knowledge graph descriptions: %v", err)}, nil
		}

		results := make([]SchemaAttribute, 0, len(attributes))
		for _, a := range attributes {
			results = append(results, SchemaAttribute{
				Name:          a.Name,
				Kind:          a.Type,
				AttributeType: a.AttributeType,
				Description:   descriptions[a.Name],
				SourceTypes:   typeRefs(a.SourceTypes, descriptions),
				TargetTypes:   typeRefs(a.TargetTypes, descriptions),
			})
		}

		return Output{
			Status:     StatusSuccess,
			Message:    fmt.Sprintf("Found %d attribute(s)/relation(s).", len(results)),
			Attributes: results,
			Count:      len(results),
		}, nil
	}
}

func typeRefs(publicIDs []string, descriptions map[string]string) []TypeRef {
	if len(publicIDs) == 0 {
		return nil
	}
	refs := make([]TypeRef, 0, len(publicIDs))
	for _, id := range publicIDs {
		refs = append(refs, TypeRef{PublicID: id, Description: descriptions[id]})
	}
	return refs
}
