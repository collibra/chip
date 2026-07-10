package clients

import (
	"context"
	"fmt"
	"net/http"
)

const technicalLineageBasePath = "/rest/catalog/1.0/technicalLineage"

// StartTechnicalLineageHarvest triggers the technical lineage harvest for a registered
// Database asset via POST /harvester/{assetId}. The endpoint accepts the request with
// 202 and an empty body — the DGC job it spawns is not returned and must be located
// through the jobs API afterwards.
func StartTechnicalLineageHarvest(ctx context.Context, client *http.Client, assetID string) error {
	endpoint := technicalLineageBasePath + "/harvester/" + assetID
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	_, err = executeRequest(client, req)
	return err
}
