package clients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// ErrAssignmentNotFound is returned when no assignment exists for the asset type
// in the domain, i.e. the asset type is not allowed there.
var ErrAssignmentNotFound = errors.New("assignment not found")

// GetAssignmentForAssetTypeAndDomain returns the effective assignment for an
// asset of the given type created in the given domain, via
// GET /rest/2.0/assignments/assetType/{assetTypeId}/domain/{domainId}.
// Scope selection and inheritance are resolved server-side, so the result lists
// the attributes and relations that apply, including those inherited from traits
// and ancestor assignments.
func GetAssignmentForAssetTypeAndDomain(ctx context.Context, client *http.Client, assetTypeID, domainID string) (*PrepareCreateScopedAssignment, error) {
	reqURL := fmt.Sprintf("/rest/2.0/assignments/assetType/%s/domain/%s", url.PathEscape(assetTypeID), url.PathEscape(domainID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building get assignment request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("getting assignment for asset type and domain: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("no assignment found for asset type %q in domain %q: %w", assetTypeID, domainID, ErrAssignmentNotFound)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("getting assignment for asset type and domain: status %d: %s", resp.StatusCode, string(body))
	}

	var raw rawScopedAssignment
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decoding assignment response: %w", err)
	}
	return emitAssignmentCharacteristics(raw), nil
}

// ErrNoDefaultAssignment is returned when an asset type has assignments, but
// only scoped ones, so none applies without knowing the domain.
var ErrNoDefaultAssignment = errors.New("no default assignment")

// GetDefaultAssignmentForAssetType returns the assignment that applies to an
// asset type regardless of domain, via GET /rest/2.0/assignments/assetType/{id}.
// That endpoint returns one assignment per scope; the unscoped one is the
// default. It returns ErrAssignmentNotFound when the asset type has no
// assignments and ErrNoDefaultAssignment when it has only scoped ones.
func GetDefaultAssignmentForAssetType(ctx context.Context, client *http.Client, assetTypeID string) (*PrepareCreateScopedAssignment, error) {
	raws, err := fetchRawAssignments(ctx, client, assetTypeID)
	if err != nil {
		return nil, err
	}
	if len(raws) == 0 {
		return nil, fmt.Errorf("no assignment found for asset type %q: %w", assetTypeID, ErrAssignmentNotFound)
	}
	for _, raw := range raws {
		if raw.Scope == nil {
			return emitAssignmentCharacteristics(raw), nil
		}
	}
	return nil, fmt.Errorf("asset type %q has only scoped assignments: %w", assetTypeID, ErrNoDefaultAssignment)
}
