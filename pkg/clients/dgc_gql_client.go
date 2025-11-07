package clients

import (
	"encoding/json"
	"fmt"
)

const (
	AttributesLimit = 100
	RelationsLimit  = 50
)

func CreateAssetDetailsGraphQLQuery(
	assetIds []string,
	outgoingRelationsCursor string,
	incomingRelationsCursor string,
) Request {
	variables := map[string]interface{}{
		"assetIds":        assetIds,
		"attributesLimit": AttributesLimit,
		"relationsLimit":  RelationsLimit,
	}

	// Build query parameters list
	queryParams := "$assetIds: [UUID!]!, $attributesLimit: Int!, $relationsLimit: Int!"

	// Build outgoing relations clause
	outgoingClause := "outgoingRelations(order: { id: asc }, limit: $relationsLimit)"
	if outgoingRelationsCursor != "" {
		queryParams += ", $outgoingCursor: UUID!"
		outgoingClause = "outgoingRelations(order: { id: asc }, where: { id: { gt: $outgoingCursor } }, limit: $relationsLimit)"
		variables["outgoingCursor"] = outgoingRelationsCursor
	}

	// Build incoming relations clause
	incomingClause := "incomingRelations(order: { id: asc }, limit: $relationsLimit)"
	if incomingRelationsCursor != "" {
		queryParams += ", $incomingCursor: UUID!"
		incomingClause = "incomingRelations(order: { id: asc }, where: { id: { gt: $incomingCursor } }, limit: $relationsLimit)"
		variables["incomingCursor"] = incomingRelationsCursor
	}

	query := fmt.Sprintf(`
query GetAssetDetails(%s) {
  assets(where: { id: { in: $assetIds } }) {
    id
    displayName
    type {
      name
    }
    domain {
      name
    }
    status {
      name
    }
    stringAttributes(limit: $attributesLimit) {
      stringValue
      type {
        name
      }
    }
    numericAttributes(limit: $attributesLimit) {
      numericValue
      type {
        name
      }
    }
    booleanAttributes(limit: $attributesLimit) {
      booleanValue
      type {
        name
      }
    }
    dateAttributes(limit: $attributesLimit) {
      dateValue
      type {
        name
      }
    }
    %s {
      type {
        id
        role
      }
      target {
        id
        displayName
        type {
          name
        }
      }
    }
    %s {
      type {
        id
        role
      }
      source {
        id
        displayName
        type {
          name
        }
      }
    }
  }
}
`, queryParams, outgoingClause, incomingClause)

	return Request{
		Query:     query,
		Variables: variables,
	}
}

func ParseAssetDetailsGraphQLResponse(jsonData []byte) ([]Asset, error) {
	var response Response
	err := json.Unmarshal(jsonData, &response)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal GraphQL response: %w", err)
	}

	if len(response.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL errors: %v", response.Errors)
	}

	if response.Data == nil {
		return nil, fmt.Errorf("no data in GraphQL response")
	}

	return response.Data.Assets, nil
}

type Request struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables,omitempty"`
}

type Response struct {
	Data   *AssetQueryData `json:"data,omitempty"`
	Errors []Error         `json:"errors,omitempty"`
}

type Error struct {
	Message string        `json:"message"`
	Path    []interface{} `json:"path,omitempty"`
}

type AssetQueryData struct {
	Assets []Asset `json:"assets"`
}

type Asset struct {
	ID                string             `json:"id"`
	DisplayName       string             `json:"displayName"`
	Type              *AssetType         `json:"type,omitempty"`
	Domain            *Domain            `json:"domain,omitempty"`
	Status            *Status            `json:"status,omitempty"`
	StringAttributes  []StringAttribute  `json:"stringAttributes,omitempty"`
	NumericAttributes []NumericAttribute `json:"numericAttributes,omitempty"`
	BooleanAttributes []BooleanAttribute `json:"booleanAttributes,omitempty"`
	DateAttributes    []DateAttribute    `json:"dateAttributes,omitempty"`
	OutgoingRelations []OutgoingRelation `json:"outgoingRelations,omitempty"`
	IncomingRelations []IncomingRelation `json:"incomingRelations,omitempty"`
}

type AssetType struct {
	Name string `json:"name"`
}

type Domain struct {
	Name string `json:"name"`
}

type Status struct {
	Name string `json:"name"`
}

type StringAttribute struct {
	Value string         `json:"stringValue"`
	Type  *AttributeType `json:"type,omitempty"`
}

type NumericAttribute struct {
	Value float64        `json:"numericValue"`
	Type  *AttributeType `json:"type,omitempty"`
}

type BooleanAttribute struct {
	Value bool           `json:"booleanValue"`
	Type  *AttributeType `json:"type,omitempty"`
}

type DateAttribute struct {
	Value string         `json:"dateValue"`
	Type  *AttributeType `json:"type,omitempty"`
}

type AttributeType struct {
	Name string `json:"name"`
}

type OutgoingRelation struct {
	Type   *RelationType `json:"type,omitempty"`
	Target *RelatedAsset `json:"target,omitempty"`
}

type IncomingRelation struct {
	Type   *RelationType `json:"type,omitempty"`
	Source *RelatedAsset `json:"source,omitempty"`
}

type RelationType struct {
	ID   string `json:"id"`
	Role string `json:"role,omitempty"`
}

type RelatedAsset struct {
	ID          string     `json:"id"`
	DisplayName string     `json:"displayName"`
	Type        *AssetType `json:"type,omitempty"`
}
