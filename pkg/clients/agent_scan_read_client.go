package clients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	importedAgentsPath   = "/rest/guardianFingerprint/v1/importedAgents"
	latestFingerprintFmt = "/rest/guardianFingerprint/v1/fingerprints/by/aiAgent/%s"
)

// ImportedAgent is one agent Guardian has imported as an AI Agent asset.
type ImportedAgent struct {
	ExternalID       string `json:"externalId"`
	AIAgentID        string `json:"aiAgentId"`
	AIAgentVersionID string `json:"aiAgentVersionId,omitempty"`
}

// ImportedAgentPage is one page of a source system's imported agents.
type ImportedAgentPage struct {
	Results    []ImportedAgent `json:"results"`
	NextCursor string          `json:"nextCursor,omitempty"`
}

// AgentFingerprint is one distinct state of one agent, with its document.
type AgentFingerprint struct {
	ID                 string          `json:"id"`
	ExternalID         string          `json:"externalId"`
	AIAgentID          string          `json:"aiAgentId,omitempty"`
	ContentHash        string          `json:"contentHash"`
	FingerprintVersion string          `json:"fingerprintVersion"`
	ParserVersion      string          `json:"parserVersion"`
	ScanCount          int64           `json:"scanCount"`
	FirstSeenOn        string          `json:"firstSeenOn"`
	LastSeenOn         string          `json:"lastSeenOn"`
	Document           json.RawMessage `json:"document"`
}

// ListImportedAgents reads one page of the agents imported from a source
// system (the provider and instance of an external id, e.g.
// "databricks:1234567890123456"). limit 0 means the service default.
func ListImportedAgents(ctx context.Context, collibraHttpClient *http.Client, sourceSystem, cursor string, limit int) (*ImportedAgentPage, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Listing imported agents of source system: %s", sourceSystem))

	query := url.Values{"sourceSystem": {sourceSystem}}
	if cursor != "" {
		query.Set("cursor", cursor)
	}
	if limit > 0 {
		query.Set("limit", strconv.Itoa(limit))
	}

	var page ImportedAgentPage
	if err := getAgentScanJSON(ctx, collibraHttpClient, importedAgentsPath+"?"+query.Encode(), &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// GetLatestAgentFingerprint reads the newest state Guardian holds for an AI
// Agent asset, with its document.
func GetLatestAgentFingerprint(ctx context.Context, collibraHttpClient *http.Client, aiAgentID string) (*AgentFingerprint, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Reading latest agent fingerprint of AI Agent: %s", aiAgentID))

	var fingerprint AgentFingerprint
	path := fmt.Sprintf(latestFingerprintFmt, url.PathEscape(aiAgentID))
	if err := getAgentScanJSON(ctx, collibraHttpClient, path, &fingerprint); err != nil {
		return nil, err
	}
	if fingerprint.ID == "" {
		return nil, errors.New("Collibra answered success but the response did not contain a fingerprint")
	}
	return &fingerprint, nil
}

// getAgentScanJSON runs one read request and decodes the JSON answer. A read
// is safe to repeat, but it is sent once: the caller decides about retries.
func getAgentScanJSON(ctx context.Context, collibraHttpClient *http.Client, pathAndQuery string, into any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pathAndQuery, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	body, status, err := executeRequestWithStatus(collibraHttpClient, req)
	if err != nil {
		if status == 0 {
			return err
		}
		if status >= 300 && status < 400 {
			return signInRequiredError()
		}
		return parseAgentScanAPIError(status, body)
	}
	if err := json.Unmarshal(body, into); err != nil {
		if looksLikeHTML(body) {
			return signInRequiredError()
		}
		return fmt.Errorf("Collibra answered success but the response could not be read: %w", err)
	}
	return nil
}

// ReadFailure explains a failed agent scan read in plain language, with what
// to tell or ask the customer next.
func (e *AgentScanAPIError) ReadFailure() (message, guidance string) {
	switch {
	case e.StatusCode == http.StatusNotFound:
		return "Collibra has no scan for this AI Agent asset: it was never fingerprinted here, or the id is not an AI Agent asset.",
			"This is an answer, not a fault: treat the agent as having no earlier scan and continue without one."
	case e.StatusCode == http.StatusUnauthorized:
		return "Collibra did not accept the sign-in for this read (HTTP 401).",
			"Do not retry. Tell the customer the earlier scan could not be looked up, carry on without it, and mention that signing in to Collibra again may fix it."
	case e.StatusCode == http.StatusForbidden:
		return "Collibra refused this read for the current user or environment (HTTP 403); reading scans needs permission to read fingerprints.",
			"Do not retry. Tell the customer the earlier scan could not be looked up and carry on without it."
	}
	return fmt.Sprintf("Collibra could not answer the read (HTTP %d).", e.StatusCode),
		"Tell the customer the earlier scan could not be looked up and carry on without it; trying once more later is fine."
}

// AgentAsset is a root AI Agent asset found by name.
type AgentAsset struct {
	AssetID    string
	ExternalID string
	AgentName  string
	DomainName string
}

const agentAssetNameMarker = ">agent>"

// FindAgentAssetsByName searches the catalog's assets whose name contains text
// and returns the root AI Agent assets among them. A root agent asset is named
// "<externalId>>agent>NAME", so the external id and the agent's own name are
// read back from the asset name; sub-agent, version and tool assets are left
// out. The asset id is the aiAgentId the agent scan reads ask for.
func FindAgentAssetsByName(ctx context.Context, collibraHttpClient *http.Client, text string, limit int) ([]AgentAsset, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Searching AI Agent assets by name containing: %s", text))

	if limit <= 0 {
		limit = 200
	}
	params := url.Values{}
	params.Set("name", text)
	params.Set("nameMatchMode", "ANYWHERE")
	params.Set("limit", strconv.Itoa(limit))

	var resp struct {
		Results []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Domain struct {
				Name string `json:"name"`
			} `json:"domain"`
		} `json:"results"`
	}
	if err := getAgentScanJSON(ctx, collibraHttpClient, "/rest/2.0/assets?"+params.Encode(), &resp); err != nil {
		return nil, err
	}

	var found []AgentAsset
	for _, r := range resp.Results {
		externalID, agentName, ok := strings.Cut(r.Name, agentAssetNameMarker)
		if !ok || externalID == "" || agentName == "" || strings.Contains(agentName, ">") {
			continue
		}
		found = append(found, AgentAsset{AssetID: r.ID, ExternalID: externalID, AgentName: agentName, DomainName: r.Domain.Name})
	}
	return found, nil
}
