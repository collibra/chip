package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// AssetIdentity is the core identity of an asset from GET /rest/2.0/assets/{id}.
type AssetIdentity struct {
	ID   string       `json:"id"`
	Name string       `json:"name"`
	Type AssetTypeRef `json:"type"`
}

type AssetTypeRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RequireAsset fails when assetID does not exist, naming it with the caller's label.
// Relation queries cannot make this distinction: /rest/2.0/relations answers 200 with an
// empty page both for an unknown id and for a real asset that has no relations, so a
// traversal that starts from relations alone reports "nothing connected" for a typo.
func RequireAsset(ctx context.Context, client *http.Client, label string, assetID string) (*AssetIdentity, error) {
	asset, err := GetAssetIdentity(ctx, client, assetID)
	if err != nil {
		return nil, fmt.Errorf("%s %q: %w", label, assetID, err)
	}
	return asset, nil
}

// GetAssetIdentity fetches an asset by id. Unlike the relation and attribute filter
// endpoints, /rest/2.0/assets/{id} answers 404 for an id that does not exist.
func GetAssetIdentity(ctx context.Context, client *http.Client, assetID string) (*AssetIdentity, error) {
	reqURL := fmt.Sprintf("/rest/2.0/assets/%s", url.PathEscape(assetID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("not found")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var asset AssetIdentity
	if err := json.NewDecoder(resp.Body).Decode(&asset); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	return &asset, nil
}
