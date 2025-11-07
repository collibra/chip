package clients

import (
	"encoding/json"
	"fmt"
)

// AssetTypePagedResponse represents the response from the Collibra asset types API
type AssetTypePagedResponse struct {
	Total   int64              `json:"total"`
	Offset  int64              `json:"offset"`
	Limit   int64              `json:"limit"`
	Results []AssetTypeDetails `json:"results"`
}

type AssetTypeDetails struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Description        string `json:"description,omitempty"`
	PublicId           string `json:"publicId,omitempty"`
	DisplayNameEnabled bool   `json:"displayNameEnabled"`
	RatingEnabled      bool   `json:"ratingEnabled"`
	FinalType          bool   `json:"finalType"`
	System             bool   `json:"system"`
	Product            string `json:"product,omitempty"`
}

type AssetTypesQueryParams struct {
	ExcludeMeta bool `url:"excludeMeta,omitempty"`
	Limit       int  `url:"limit,omitempty"`
	Offset      int  `url:"offset,omitempty"`
}

func ParseAssetTypesResponse(jsonData []byte) (*AssetTypePagedResponse, error) {
	var assetTypeResponse AssetTypePagedResponse
	if err := json.Unmarshal(jsonData, &assetTypeResponse); err != nil {
		return nil, fmt.Errorf("failed to parse asset types response: %w", err)
	}

	return &assetTypeResponse, nil
}
