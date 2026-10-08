package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// This client queries the Knowledge Graph service's query-syntax search
// endpoint (POST /rest/knowledgeGraph/v1/hackaton/search). That endpoint is a
// preview surface under active development — the "hackaton" path segment is
// not a typo, it has not yet been promoted to a stable, versioned REST
// contract — which is why search_knowledge_graph is gated behind the
// knowledge-graph-search experimental feature rather than shipping generally
// available.

const knowledgeGraphSearchEndpoint = "/rest/knowledgeGraph/v1/hackaton/search"
const knowledgeGraphDescriptionsEndpoint = "/rest/knowledgeGraph/v1/hackaton/descriptions"

// knowledgeGraphAttributesEndpoint: "hackation" (not "hackaton") is the real,
// live path for this endpoint upstream — verified against the running
// service, not a typo on this side.
const knowledgeGraphAttributesEndpoint = "/rest/knowledgeGraph/v1/hackation/attributes"

type knowledgeGraphSearchRequest struct {
	Query string `json:"query"`
}

// KnowledgeGraphAsset is one matching asset returned by the search.
type KnowledgeGraphAsset struct {
	ID         string         `json:"id"`
	Labels     []string       `json:"labels"`
	Score      float64        `json:"score"`
	Properties map[string]any `json:"properties"`
}

// KnowledgeGraphSchemaAttribute is one attribute or relation name in the
// schema, as returned by the attributes endpoint (before descriptions are
// merged in).
type KnowledgeGraphSchemaAttribute struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"` // "Attribute" or "Relation"
	AttributeType string   `json:"attributeType,omitempty"`
	SourceTypes   []string `json:"sourceTypes,omitempty"` // Relation only: asset type publicIds at the source end.
	TargetTypes   []string `json:"targetTypes,omitempty"` // Relation only: asset type publicIds at the target end.
}

// SearchKnowledgeGraph runs a query-syntax search against the Knowledge Graph
// service and returns the matching assets.
func SearchKnowledgeGraph(ctx context.Context, client *http.Client, query string) ([]KnowledgeGraphAsset, error) {
	respBody, status, err := dqDo(ctx, client, http.MethodPost, knowledgeGraphSearchEndpoint, knowledgeGraphSearchRequest{Query: query})
	if err != nil {
		return nil, fmt.Errorf("searching knowledge graph: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("searching knowledge graph: unexpected status %d: %s", status, string(respBody))
	}
	var results []KnowledgeGraphAsset
	if err := json.Unmarshal(respBody, &results); err != nil {
		return nil, fmt.Errorf("searching knowledge graph: decoding response: %w", err)
	}
	return results, nil
}

// GetKnowledgeGraphDescriptions returns the publicId -> description map for
// every AssetType, AttributeType, RelationType and Trait known to the
// Knowledge Graph service.
func GetKnowledgeGraphDescriptions(ctx context.Context, client *http.Client) (map[string]string, error) {
	respBody, status, err := dqDo(ctx, client, http.MethodGet, knowledgeGraphDescriptionsEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("getting knowledge graph descriptions: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("getting knowledge graph descriptions: unexpected status %d: %s", status, string(respBody))
	}
	var descriptions map[string]string
	if err := json.Unmarshal(respBody, &descriptions); err != nil {
		return nil, fmt.Errorf("getting knowledge graph descriptions: decoding response: %w", err)
	}
	return descriptions, nil
}

// GetKnowledgeGraphSchemaAttributes returns every attribute and relation name
// usable in a search_knowledge_graph query.
func GetKnowledgeGraphSchemaAttributes(ctx context.Context, client *http.Client) ([]KnowledgeGraphSchemaAttribute, error) {
	respBody, status, err := dqDo(ctx, client, http.MethodGet, knowledgeGraphAttributesEndpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("getting knowledge graph schema attributes: %w", err)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("getting knowledge graph schema attributes: unexpected status %d: %s", status, string(respBody))
	}
	var attributes []KnowledgeGraphSchemaAttribute
	if err := json.Unmarshal(respBody, &attributes); err != nil {
		return nil, fmt.Errorf("getting knowledge graph schema attributes: decoding response: %w", err)
	}
	return attributes, nil
}
