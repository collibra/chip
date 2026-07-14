package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// This client queries the DGC Knowledge Graph GraphQL API
// (POST /graphql/knowledgeGraph/v1) to find catalog Column assets by metadata
// that the public REST search cannot filter on — attribute values, assigned
// responsibilities (roles), and relations to other assets. It is built against
// the deployed KG schema (assets(where: AssetFilter)); if a target environment
// runs a different KG version the filter shapes may differ.

const kgEndpoint = "/graphql/knowledgeGraph/v1"

// Column asset type and the OOTB (is_system) relation-type public ids that link
// a Column to each catalog asset type, with the direction of the relation
// relative to the Column and which end holds the other asset.
const (
	kgColumnAssetType = "Column"

	// Business Term (Business Asset) --represents--> Column (Data Asset): the
	// Business Term is the source, so from the Column it is an incoming relation.
	relBusinessTermPublicID = "BusinessAssetRepresentsDataAsset"
	// Column (Asset) --governed by--> Business Rule (Governance Asset): Column is
	// the source, so it is an outgoing relation with the rule as the target.
	relBusinessRulePublicID = "AssetGovernedByGovernanceAsset"
	// Data Element --targets--> Data Element (lineage); Column is a Data Element.
	relDataElementPublicID = "DataElementTargetsDataElement"
	// Data Attribute --represents--> Column: Data Attribute is the source, so from
	// the Column it is an incoming relation.
	relDataAttributePublicID = "DataAttributeRepresentsColumn"

	// Attribute type names used for the value filters.
	attrDescription = "Description"
	attrDataType    = "Data Type"
)

// CatalogColumnSearchParams are the metadata filters, ANDed together. Empty
// fields are omitted. A specific attribute-type name / relation public-id is
// applied per field (see constants above).
type CatalogColumnSearchParams struct {
	Domain        string
	Community     string
	Description   string
	DataType      string
	StewardRole   string
	BusinessTerm  string
	BusinessRule  string
	DataElement   string
	DataAttribute string
	Limit         int
	Offset        int
}

// CatalogColumn is one matching column returned by the search.
type CatalogColumn struct {
	ID          string `json:"id"`
	FullName    string `json:"fullName"`
	DisplayName string `json:"displayName"`
	Type        struct {
		Name string `json:"name"`
	} `json:"type"`
	Domain struct {
		Name string `json:"name"`
	} `json:"domain"`
}

type kgSearchResponse struct {
	Data struct {
		Assets []CatalogColumn `json:"assets"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

const kgColumnsQuery = `query Search($where: AssetFilter, $limit: Int, $offset: Int) {
  assets(where: $where, limit: $limit, offset: $offset) {
    id
    fullName
    displayName
    type { name }
    domain { name }
  }
}`

// eq builds a StringFilter {eq: v}; contains builds {contains: v}.
func kgEq(v string) map[string]any       { return map[string]any{"name": map[string]any{"eq": v}} }
func kgStringEq(v string) map[string]any { return map[string]any{"eq": v} }

// relationFragment builds an incoming/outgoing relation filter fragment matching
// the other end's displayName. dir is "incomingRelations" or "outgoingRelations";
// end is "source" or "target" (the end that holds the related asset).
func relationFragment(dir, publicID, end, value string) map[string]any {
	return map[string]any{
		dir: map[string]any{
			"typePublicId": publicID,
			"any": map[string]any{
				end: map[string]any{"displayName": kgStringEq(value)},
			},
		},
	}
}

// stringAttributeFragment matches a string attribute of the given type name whose
// value contains v.
func stringAttributeFragment(attrTypeName, v string) map[string]any {
	return map[string]any{
		"stringAttributes": map[string]any{
			"any": map[string]any{
				"type":        kgEq(attrTypeName),
				"stringValue": map[string]any{"contains": v},
			},
		},
	}
}

// buildWhere assembles the AssetFilter. Fragments that reuse the same top-level
// key (stringAttributes, incomingRelations, outgoingRelations) cannot coexist in
// one object, so all fragments are chained through the singular `_and`.
func buildWhere(p CatalogColumnSearchParams) map[string]any {
	var frags []map[string]any

	// Always scope to Column.
	frags = append(frags, map[string]any{"type": kgEq(kgColumnAssetType)})

	if p.Domain != "" || p.Community != "" {
		domain := map[string]any{}
		if p.Domain != "" {
			domain["name"] = kgStringEq(p.Domain)
		}
		if p.Community != "" {
			domain["parent"] = map[string]any{"name": kgStringEq(p.Community)}
		}
		frags = append(frags, map[string]any{"domain": domain})
	}
	if p.Description != "" {
		frags = append(frags, stringAttributeFragment(attrDescription, p.Description))
	}
	if p.DataType != "" {
		frags = append(frags, stringAttributeFragment(attrDataType, p.DataType))
	}
	if p.StewardRole != "" {
		frags = append(frags, map[string]any{
			"responsibilities": map[string]any{
				"any": map[string]any{"role": kgEq(p.StewardRole)},
			},
		})
	}
	if p.BusinessTerm != "" {
		frags = append(frags, relationFragment("incomingRelations", relBusinessTermPublicID, "source", p.BusinessTerm))
	}
	if p.BusinessRule != "" {
		frags = append(frags, relationFragment("outgoingRelations", relBusinessRulePublicID, "target", p.BusinessRule))
	}
	if p.DataElement != "" {
		frags = append(frags, relationFragment("outgoingRelations", relDataElementPublicID, "target", p.DataElement))
	}
	if p.DataAttribute != "" {
		frags = append(frags, relationFragment("incomingRelations", relDataAttributePublicID, "source", p.DataAttribute))
	}

	// Fold the fragments into a single object chained by `_and`.
	root := frags[0]
	cur := root
	for _, f := range frags[1:] {
		cur["_and"] = f
		cur = f
	}
	return root
}

// SearchCatalogColumns finds catalog Column assets matching the given metadata
// filters via the Knowledge Graph GraphQL API. Requires that KG endpoint to be
// available on the target instance.
func SearchCatalogColumns(ctx context.Context, client *http.Client, params CatalogColumnSearchParams) ([]CatalogColumn, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 25
	}
	body := map[string]any{
		"query": kgColumnsQuery,
		"variables": map[string]any{
			"where":  buildWhere(params),
			"limit":  limit,
			"offset": params.Offset,
		},
	}
	respBody, status, err := dqDo(ctx, client, http.MethodPost, kgEndpoint, body)
	if err != nil {
		return nil, fmt.Errorf("searching catalog columns: %w", err)
	}
	if status != http.StatusOK {
		if status == http.StatusNotFound {
			return nil, fmt.Errorf("searching catalog columns: knowledge graph endpoint not available on this instance (HTTP 404): %s", string(respBody))
		}
		return nil, fmt.Errorf("searching catalog columns: unexpected status %d: %s", status, string(respBody))
	}
	var resp kgSearchResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("searching catalog columns: decoding response: %w", err)
	}
	if len(resp.Errors) > 0 {
		return nil, fmt.Errorf("searching catalog columns: graphql error: %s", resp.Errors[0].Message)
	}
	return resp.Data.Assets, nil
}
