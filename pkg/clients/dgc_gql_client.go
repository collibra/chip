package clients

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	AttributesLimit = 100
	RelationsLimit  = 50
	TagsLimit       = 100
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
		"tagsLimit":       TagsLimit,
	}

	// Build query parameters list
	queryParams := "$assetIds: [UUID!]!, $attributesLimit: Int!, $relationsLimit: Int!, $tagsLimit: Int!"

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
    tags(limit: $tagsLimit) {
      id
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
    multiValueAttributes(limit: $attributesLimit) {
      stringValues
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
	ID                   string                `json:"id"`
	DisplayName          string                `json:"displayName"`
	Type                 *AssetType            `json:"type,omitempty"`
	Domain               *Domain               `json:"domain,omitempty"`
	Status               *Status               `json:"status,omitempty"`
	StringAttributes     []StringAttribute     `json:"stringAttributes,omitempty"`
	NumericAttributes    []NumericAttribute    `json:"numericAttributes,omitempty"`
	BooleanAttributes    []BooleanAttribute    `json:"booleanAttributes,omitempty"`
	DateAttributes       []DateAttribute       `json:"dateAttributes,omitempty"`
	MultiValueAttributes []MultiValueAttribute `json:"multiValueAttributes,omitempty"`
	OutgoingRelations    []OutgoingRelation    `json:"outgoingRelations,omitempty"`
	IncomingRelations    []IncomingRelation    `json:"incomingRelations,omitempty"`
	Tags                 []Tag                 `json:"tags,omitempty"`
}

type Tag struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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

// MultiValueAttribute is a Multiple Selection attribute with every selected value.
type MultiValueAttribute struct {
	Values []string       `json:"stringValues"`
	Type   *AttributeType `json:"type,omitempty"`
}

// UnmarshalJSON accepts stringValues either as DGC's single delimited string
// or as an already split list, so a marshalled MultiValueAttribute decodes back
// to the same values.
func (a *MultiValueAttribute) UnmarshalJSON(data []byte) error {
	type alias MultiValueAttribute
	aux := struct {
		*alias
		Values json.RawMessage `json:"stringValues"`
	}{alias: (*alias)(a)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	a.Values = nil
	if len(aux.Values) == 0 || string(aux.Values) == "null" {
		return nil
	}
	if aux.Values[0] == '[' {
		return json.Unmarshal(aux.Values, &a.Values)
	}
	var joined string
	if err := json.Unmarshal(aux.Values, &joined); err != nil {
		return err
	}
	a.Values = splitMultiValues(joined)
	return nil
}

// splitMultiValues splits DGC's encoding of a value list: values joined by ';',
// with a ';' inside a value written as '\;'. Any other backslash is literal.
func splitMultiValues(joined string) []string {
	if joined == "" {
		return nil
	}
	var values []string
	var current strings.Builder
	for i := 0; i < len(joined); i++ {
		switch {
		case joined[i] == '\\' && i+1 < len(joined) && joined[i+1] == ';':
			current.WriteByte(';')
			i++
		case joined[i] == ';':
			values = append(values, current.String())
			current.Reset()
		default:
			current.WriteByte(joined[i])
		}
	}
	return append(values, current.String())
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
