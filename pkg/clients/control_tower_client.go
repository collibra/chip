// Package clients — control_tower_client.go covers Collibra Control Tower
// management + execution endpoints, plus the DGC discovery calls that the
// create-control flow needs (domain resolution, ManagedControl attribute
// types with allowedValues).
//
// All endpoints share the same base URL the rest of chip uses.

package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
)

// ===== Domain resolution (DGC) =====

// DomainSummary is the shape returned to tools — id + name + community name.
type DomainSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Community string `json:"community,omitempty"`
}

type rawDomain struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Community struct {
		Name string `json:"name"`
	} `json:"community"`
}

type rawDomainPage struct {
	Total   int64       `json:"total"`
	Results []rawDomain `json:"results"`
}

// DomainResolution is the union of the three possible outcomes of resolving
// a domain by name. Exactly one of Single / Candidates / NotFound is set.
type DomainResolution struct {
	Single     *DomainSummary
	Candidates []DomainSummary // populated when name resolves to >1
	NotFound   bool
	Reason     string
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func ResolveDomain(ctx context.Context, client *http.Client, query string) (*DomainResolution, error) {
	if uuidRe.MatchString(query) {
		req, err := http.NewRequestWithContext(ctx, "GET", "/rest/2.0/domains/"+query, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to build domain GET: %w", err)
		}
		body, err := executeRequest(client, req)
		if err != nil {
			// 404 surfaces as "HTTP 404: ..." from executeRequest — treat as not-found
			// only when the caller can disambiguate; for now return the raw error.
			return nil, err
		}
		var d rawDomain
		if err := json.Unmarshal(body, &d); err != nil {
			return nil, fmt.Errorf("failed to parse domain response: %w", err)
		}
		return &DomainResolution{Single: &DomainSummary{ID: d.ID, Name: d.Name, Community: d.Community.Name}}, nil
	}

	endpoint := "/rest/2.0/domains?name=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build domains search: %w", err)
	}
	body, err := executeRequest(client, req)
	if err != nil {
		return nil, err
	}
	var page rawDomainPage
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("failed to parse domains page: %w", err)
	}
	if len(page.Results) == 0 {
		return &DomainResolution{NotFound: true, Reason: fmt.Sprintf("no domain matches name=%q", query)}, nil
	}
	if len(page.Results) == 1 {
		d := page.Results[0]
		return &DomainResolution{Single: &DomainSummary{ID: d.ID, Name: d.Name, Community: d.Community.Name}}, nil
	}
	candidates := make([]DomainSummary, len(page.Results))
	for i, d := range page.Results {
		candidates[i] = DomainSummary{ID: d.ID, Name: d.Name, Community: d.Community.Name}
	}
	return &DomainResolution{Candidates: candidates}, nil
}

// ===== ManagedControl attribute discovery (DGC) =====

// ManagedControlAttribute is the focused projection: per-publicId metadata
// plus allowedValues for SingleValueList / MultiValueList kinds.
type ManagedControlAttribute struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	PublicID      string   `json:"publicId"`
	Kind          string   `json:"kind"`
	AllowedValues []string `json:"allowedValues"`
}

type rawAssignedAttributeType struct {
	AttributeType struct {
		ID                          string   `json:"id"`
		Name                        string   `json:"name"`
		PublicID                    string   `json:"publicId"`
		AttributeTypeDiscriminator  string   `json:"attributeTypeDiscriminator"`
		AllowedValues               []string `json:"allowedValues"`
	} `json:"attributeType"`
	AssignedCharacteristicTypeDiscriminator string `json:"assignedCharacteristicTypeDiscriminator"`
	AssignedResourceType                    string `json:"assignedResourceType"`
}

type rawAssignment struct {
	CharacteristicTypes []rawAssignedAttributeType `json:"characteristicTypes"`
}

type rawAssetType struct {
	ID string `json:"id"`
}

const managedControlPublicID = "ManagedControl"

var managedControlAttributesCache catalogCache[map[string]ManagedControlAttribute]

// GetManagedControlAttributes returns the AttributeTypes assigned to the
// ManagedControl asset type, keyed by publicId. The control-tower save
// flow uses this as the source of truth for category / controlType /
// severity allowed values (the OAS-documented enums are not stable).
//
// Cached for catalogCacheTTL — see catalog_cache.go.
func GetManagedControlAttributes(ctx context.Context, client *http.Client) (map[string]ManagedControlAttribute, error) {
	return managedControlAttributesCache.get(ctx, client, fetchManagedControlAttributes)
}

func fetchManagedControlAttributes(ctx context.Context, client *http.Client) (map[string]ManagedControlAttribute, error) {
	atReq, err := http.NewRequestWithContext(ctx, "GET", "/rest/2.0/assetTypes/publicId/"+managedControlPublicID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build asset-type GET: %w", err)
	}
	atBody, err := executeRequest(client, atReq)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch %s asset type: %w", managedControlPublicID, err)
	}
	var at rawAssetType
	if err := json.Unmarshal(atBody, &at); err != nil {
		return nil, fmt.Errorf("failed to parse asset type: %w", err)
	}
	if at.ID == "" {
		return nil, fmt.Errorf("asset type %q not found", managedControlPublicID)
	}

	asReq, err := http.NewRequestWithContext(ctx, "GET", "/rest/2.0/assignments/assetType/"+at.ID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build assignments GET: %w", err)
	}
	asBody, err := executeRequest(client, asReq)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch assignments: %w", err)
	}
	var assignments []rawAssignment
	if err := json.Unmarshal(asBody, &assignments); err != nil {
		return nil, fmt.Errorf("failed to parse assignments: %w", err)
	}

	out := make(map[string]ManagedControlAttribute)
	for _, assignment := range assignments {
		for _, ct := range assignment.CharacteristicTypes {
			disc := ct.AssignedCharacteristicTypeDiscriminator
			if disc == "" {
				disc = ct.AssignedResourceType
			}
			if disc != "AttributeType" {
				continue
			}
			pid := ct.AttributeType.PublicID
			if pid == "" {
				continue
			}
			kind := ct.AttributeType.AttributeTypeDiscriminator
			allowed := ct.AttributeType.AllowedValues
			if allowed == nil {
				allowed = []string{}
			}
			out[pid] = ManagedControlAttribute{
				ID:            ct.AttributeType.ID,
				Name:          ct.AttributeType.Name,
				PublicID:      pid,
				Kind:          kind,
				AllowedValues: allowed,
			}
		}
	}
	return out, nil
}

// ===== Control Tower: dry-run =====

type ControlQuery = json.RawMessage

type DryRunRequest struct {
	IncludeFailedAssetsCount bool         `json:"includeFailedAssetsCount"`
	IncludeSampleFailedAssets bool        `json:"includeSampleFailedAssets"`
	SampleFailedAssetsLimit  int          `json:"sampleFailedAssetsLimit"`
	Query                    ControlQuery `json:"query"`
}

// DryRunResult mirrors the management API response shape for the
// /controlExecution/v1/controlQueries/dryRun endpoint. Kept loose
// (json.RawMessage for the inner payload) so we don't lose fields the
// API may add.
type DryRunResult = json.RawMessage

func DryRunControlQuery(ctx context.Context, client *http.Client, query ControlQuery, sampleLimit int) (DryRunResult, error) {
	body := DryRunRequest{
		IncludeFailedAssetsCount: true,
		IncludeSampleFailedAssets: true,
		SampleFailedAssetsLimit: sampleLimit,
		Query:                    query,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal dry-run body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "/rest/controlExecution/v1/controlQueries/dryRun", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to build dry-run request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	respBody, err := executeRequest(client, req)
	if err != nil {
		return nil, err
	}
	return DryRunResult(respBody), nil
}

// ===== Control Tower: create =====

type CreateControlRequest struct {
	Name                  string          `json:"name"`
	Description           string          `json:"description"`
	Category              string          `json:"category"`
	ControlType           string          `json:"controlType"`
	Severity              string          `json:"severity"`
	DomainID              string          `json:"domainId"`
	Query                 ControlQuery    `json:"query"`
	ExecutionSchedule     json.RawMessage `json:"executionSchedule,omitempty"`
	NotificationSettings  json.RawMessage `json:"notificationSettings,omitempty"`
}

type CreatedControl = json.RawMessage

func CreateControl(ctx context.Context, client *http.Client, req CreateControlRequest) (CreatedControl, error) {
	jsonBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal create-control body: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", "/rest/controlManagement/v1/controls", bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to build create-control request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	respBody, err := executeRequest(client, httpReq)
	if err != nil {
		return nil, err
	}
	return CreatedControl(respBody), nil
}

// ===== Control Tower: enable / execute =====

func EnableControl(ctx context.Context, client *http.Client, controlID string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", "/rest/controlManagement/v1/controls/"+url.PathEscape(controlID)+"/enable", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build enable request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return executeRequest(client, req)
}

func ExecuteControl(ctx context.Context, client *http.Client, controlID string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", "/rest/controlExecution/v1/controls/"+url.PathEscape(controlID)+"/execute", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build execute request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return executeRequest(client, req)
}
