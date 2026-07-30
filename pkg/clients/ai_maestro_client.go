package clients

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
)

// AI Maestro API v1 (/rest/aiMaestro/v1) — the AI Maestro application's own REST
// API, where Maestro Agent definitions live. Agents are NOT catalog assets, so
// they are reached here rather than through /rest/2.0. Served on the same host as
// the DGC API, so the shared RoundTripper injects host + auth transparently.
const aiMaestroAPIBasePath = "/rest/aiMaestro/v1"

// GetAgentConfigurationFile exports a Maestro Agent as a YAML document, returning
// the response body unmodified. The endpoint answers 404 both for an unknown agent
// and for one the caller cannot see, and 403 when the caller can see the agent but
// is not one of its owners.
func GetAgentConfigurationFile(ctx context.Context, collibraHttpClient *http.Client, agentID string) ([]byte, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Exporting configuration for Maestro agent ID: %s", agentID))

	endpoint := fmt.Sprintf("%s/agents/%s/configurationFile", aiMaestroAPIBasePath, agentID)

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	// The endpoint also advertises application/json; ask for YAML explicitly since
	// the shared RoundTripper only defaults Content-Type, never Accept.
	req.Header.Set("Accept", "application/yaml")

	configuration, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}

	return configuration, nil
}
