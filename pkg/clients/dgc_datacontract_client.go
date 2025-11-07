package clients

import (
	"encoding/json"
	"fmt"
)

// DataContractListPaginated represents the paginated response from the data contracts API
type DataContractListPaginated struct {
	Items      []DataContract `json:"items"`
	Limit      int            `json:"limit"`
	NextCursor string         `json:"nextCursor,omitempty"`
	Total      *int           `json:"total,omitempty"`
}

// DataContract represents metadata attributes of a data contract
type DataContract struct {
	ID         string `json:"id"`
	DomainID   string `json:"domainId"`
	ManifestID string `json:"manifestId"`
}

type DataContractsQueryParams struct {
	ManifestID   string `url:"manifestId,omitempty"`
	IncludeTotal bool   `url:"includeTotal,omitempty"`
	Cursor       string `url:"cursor,omitempty"`
	Limit        int    `url:"limit,omitempty"`
}

func ParseDataContractsResponse(jsonData []byte) (*DataContractListPaginated, error) {
	var response DataContractListPaginated
	if err := json.Unmarshal(jsonData, &response); err != nil {
		return nil, fmt.Errorf("failed to parse data contracts response: %w", err)
	}

	return &response, nil
}
