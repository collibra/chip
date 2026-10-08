// Package search_knowledge_graph implements the search_knowledge_graph MCP
// tool: it searches catalog assets of any type using the Knowledge Graph
// service's query syntax (full-text terms, type/attribute filters, numeric
// comparisons, and multi-hop relationship traversal), via a preview REST
// endpoint (POST /rest/knowledgeGraph/v1/hackaton/search) that has not yet
// been promoted to a stable, versioned contract.
package search_knowledge_graph

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a search_knowledge_graph call.
type OutputStatus string

const (
	// StatusSuccess means the search ran.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any call.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the search failed (e.g. the knowledge graph endpoint is unavailable).
	StatusError OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	Query string `json:"query" jsonschema:"Required. A query string in the knowledge graph query syntax: a space-separated list of tokens, ANDed together by default. Token types: a bare word is a full-text term matched against the asset's name/displayName (e.g. revenue); a \"quoted phrase\" is a full-text phrase match; @type:TypeName restricts to one asset type (e.g. @type:Report); Attribute:value matches an array attribute's elements exactly, where an uppercase first letter on the attribute name means it is an array attribute (e.g. SecurityClassification:Restricted), and a lowercase first letter means a scalar property such as name or displayName; wildcards *value*, value*, *value work on both; Attribute:>=N / <=N / >N / <N is a numeric comparison on an array attribute; Rel.Attribute:value traverses an undirected relationship to a neighbor asset and filters on its attribute, and Rel1.Rel2.Attribute:value chains multiple hops; AND, OR and NOT combine tokens and parenthesized groups (AND is the default between tokens). Attribute, relation and type names are case-sensitive and must match exactly, including the leading uppercase/lowercase letter that signals array vs scalar — get the exact spelling and casing from get_knowledge_graph_schema rather than guessing; a wrong-case name matches nothing and returns zero results silently, with no error. Example: '@type:Table DataQualityScore:>=90 SchemaContainsTable.DataSourceType:Snowflake'."`
}

// AssetResult is one matching asset.
type AssetResult struct {
	ID         string         `json:"id" jsonschema:"Internal node id of the matching asset."`
	Labels     []string       `json:"labels" jsonschema:"Asset type labels on the node, e.g. ['Asset','Table']."`
	Score      float64        `json:"score" jsonschema:"Relevance score; higher ranks first."`
	Properties map[string]any `json:"properties" jsonschema:"The asset's attribute values, keyed by attribute name. For a Rel.Attribute filter, the matched neighbor's value is also included, under the key 'Rel.Attribute'."`
}

// Output is the typed response.
type Output struct {
	Status  OutputStatus  `json:"status" jsonschema:"'success' when the search ran; 'validation_error' for a missing query; 'error' for downstream failures (incl. the knowledge graph endpoint being unavailable)."`
	Message string        `json:"message" jsonschema:"Human-readable summary."`
	Results []AssetResult `json:"results,omitempty" jsonschema:"Matching assets, highest score first."`
	Count   int           `json:"count" jsonschema:"Number of assets returned."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "search_knowledge_graph",
		Title: "Search Knowledge Graph Assets by Query Syntax",
		Description: "Search catalog assets of ANY type using the knowledge graph's query syntax: full-text terms, @type: filters, " +
			"scalar/array attribute filters (including numeric comparisons), and Rel.Attribute multi-hop relationship traversal " +
			"(e.g. finding Reports sourced from a Salesforce schema). " +
			"Use this instead of search_asset_keyword when you need more than a plain substring match — a specific asset type, " +
			"an attribute value, a quality-score threshold, or a lineage/relationship condition. " +
			"Use search_catalog_columns instead when searching specifically for Column assets by domain, data steward, or a named " +
			"relation to a Business Term/Business Rule/Data Element/Data Attribute — that tool calls the stable, generally available " +
			"Knowledge Graph GraphQL API, whereas this tool calls a preview REST endpoint that has not yet been promoted to a stable " +
			"contract, which is why it ships behind the knowledge-graph-search experimental feature. " +
			"Example questions this answers: 'Tables in Snowflake with a data quality score over 90', 'Reports marked Restricted', " +
			"'columns sourced from Salesforce feeding reports used by Finance'. " +
			"Returns matching assets with their id, type labels, relevance score and attribute values. " +
			"Always call get_knowledge_graph_schema first, every time, to confirm exact attribute/relation names and casing before " +
			"composing the query string — do not guess names, and do not skip this because you called it earlier in the conversation. " +
			"Before calling this tool, explain the reasoning behind the query string token by token — which word(s) of the request each " +
			"token came from, and which schema entry justifies it — rather than presenting the finished query with no explanation. " +
			"Read-only; no permissions required beyond standard catalog access.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		query := strings.TrimSpace(input.Query)
		if query == "" {
			return Output{Status: StatusValidationError, Message: "query is required."}, nil
		}

		assets, err := clients.SearchKnowledgeGraph(ctx, collibraClient, query)
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not search the knowledge graph: %v", err)}, nil
		}

		results := make([]AssetResult, 0, len(assets))
		for _, a := range assets {
			results = append(results, AssetResult{
				ID:         a.ID,
				Labels:     a.Labels,
				Score:      a.Score,
				Properties: a.Properties,
			})
		}

		return Output{
			Status:  StatusSuccess,
			Message: fmt.Sprintf("Found %d matching asset(s).", len(results)),
			Results: results,
			Count:   len(results),
		}, nil
	}
}
