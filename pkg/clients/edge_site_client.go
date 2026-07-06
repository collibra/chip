package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// EdgeSite is the response shape returned by the Edge site management API.
type EdgeSite struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	SiteType          string `json:"siteType,omitempty"`
	Status            string `json:"status"`
	VersionSyncStatus string `json:"versionSyncStatus,omitempty"`
	InstalledVersion  string `json:"installedVersion,omitempty"`
	DesiredVersion    string `json:"desiredVersion,omitempty"`
}

// CapabilityType describes an available Edge capability type (e.g. "jdbc-ingestion")
// and the manifest that defines its expected install parameters.
type CapabilityType struct {
	ID string `json:"id"`
	// Manifest is arbitrary, capability-type-defined JSON. any (not json.RawMessage)
	// is deliberate: json.RawMessage is a []byte under the hood, and chip's schema
	// reflector doesn't know its custom marshaling, so it advertises an array/string
	// schema for a value that's actually a JSON object at runtime.
	Manifest any `json:"manifest,omitempty"`
}

// ConnectionType describes an available Edge connection type and the manifest that
// defines its expected parameters.
type ConnectionType struct {
	ID       string `json:"id"`
	Manifest any    `json:"manifest,omitempty"`
}

// GetEdgeSites lists all Edge sites via GET /edge/api/rest/v2/sites.
func GetEdgeSites(ctx context.Context, client *http.Client) ([]EdgeSite, error) {
	var sites []EdgeSite
	if err := getJSON(ctx, client, "/edge/api/rest/v2/sites", "listing edge sites", &sites); err != nil {
		return nil, err
	}
	return sites, nil
}

// GetCapabilityTypes lists the capability types available on an Edge site via
// GET /edge/api/rest/v2/sites/{siteId}/capabilityTypes.
func GetCapabilityTypes(ctx context.Context, client *http.Client, edgeSiteID string) ([]CapabilityType, error) {
	var types []CapabilityType
	endpoint := "/edge/api/rest/v2/sites/" + edgeSiteID + "/capabilityTypes"
	if err := getJSON(ctx, client, endpoint, "listing capability types", &types); err != nil {
		return nil, err
	}
	return types, nil
}

// GetConnectionTypes lists the connection types available on an Edge site via
// GET /edge/api/rest/v2/sites/{siteId}/connectionTypes.
func GetConnectionTypes(ctx context.Context, client *http.Client, edgeSiteID string) ([]ConnectionType, error) {
	var types []ConnectionType
	endpoint := "/edge/api/rest/v2/sites/" + edgeSiteID + "/connectionTypes"
	if err := getJSON(ctx, client, endpoint, "listing connection types", &types); err != nil {
		return nil, err
	}
	return types, nil
}

func getJSON(ctx context.Context, client *http.Client, endpoint, action string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("%s: building request: %w", action, err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: sending request: %w", action, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("%s: reading response: %w", action, err)
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: unexpected status %d: %s", action, resp.StatusCode, string(body))
	}

	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: decoding response: %w", action, err)
	}

	return nil
}
