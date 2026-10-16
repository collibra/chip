package clients

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// businessRuleAssetTypePublicID is the DGC public id of the Business Rule asset
// type, resolved to a type UUID before searching so the lookup cannot match a
// same-named asset of another type.
const businessRuleAssetTypePublicID = "BusinessRule"

// BusinessRuleAssetMatch is one Business Rule asset matched by display name.
type BusinessRuleAssetMatch struct {
	ID          string
	DisplayName string
	DomainName  string
}

// FindBusinessRuleAssetsByName returns the Business Rule assets whose name is an
// exact match for name — GET /rest/2.0/assets, filtered to the Business Rule
// asset type. Names are not unique in DGC, so this can return several matches.
func FindBusinessRuleAssetsByName(ctx context.Context, collibraHttpClient *http.Client, name string, limit int) ([]BusinessRuleAssetMatch, error) {
	assetType, err := GetAssetTypeByPublicID(ctx, collibraHttpClient, businessRuleAssetTypePublicID)
	if err != nil {
		return nil, fmt.Errorf("resolve Business Rule asset type: %w", err)
	}
	if limit <= 0 {
		limit = 50
	}
	params := url.Values{}
	params.Set("name", name)
	params.Set("nameMatchMode", "EXACT")
	params.Set("typeId", assetType.ID)
	params.Set("limit", strconv.Itoa(limit))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/rest/2.0/assets?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Results []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Domain      struct {
				Name string `json:"name"`
			} `json:"domain"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse assets response: %w", err)
	}
	matches := make([]BusinessRuleAssetMatch, 0, len(resp.Results))
	for _, r := range resp.Results {
		matches = append(matches, BusinessRuleAssetMatch{ID: r.ID, DisplayName: r.DisplayName, DomainName: r.Domain.Name})
	}
	return matches, nil
}

// ResolveBusinessRuleAssetRefs turns a caller's Business Rule references — each
// either an asset UUID or an exact asset name — into the UUIDs the DQ rule
// template API expects.
//
// A reference that is a UUID is checked to exist and to be a Business Rule
// asset, so a wrong id is caught before the write rather than as the DQ API's
// generic 400. Any reference that cannot be resolved to exactly one Business
// Rule asset is reported in problems as a sentence the agent can act on; ids is
// only complete when problems is empty. A non-nil error means the lookup itself
// failed.
func ResolveBusinessRuleAssetRefs(ctx context.Context, collibraHttpClient *http.Client, refs []string) (ids []string, problems []string, err error) {
	seen := make(map[string]bool, len(refs))
	var businessRuleTypeID string
	for _, raw := range refs {
		ref := strings.TrimSpace(raw)
		if ref == "" {
			continue
		}
		if isUUID(ref) {
			if businessRuleTypeID == "" {
				assetType, err := GetAssetTypeByPublicID(ctx, collibraHttpClient, businessRuleAssetTypePublicID)
				if err != nil {
					return nil, nil, fmt.Errorf("resolve Business Rule asset type: %w", err)
				}
				businessRuleTypeID = assetType.ID
			}
			problem, err := checkBusinessRuleAssetID(ctx, collibraHttpClient, ref, businessRuleTypeID)
			if err != nil {
				return nil, nil, err
			}
			if problem != "" {
				problems = append(problems, problem)
				continue
			}
			appendUnique(&ids, seen, ref)
			continue
		}

		matches, err := FindBusinessRuleAssetsByName(ctx, collibraHttpClient, ref, 0)
		if err != nil {
			return nil, nil, fmt.Errorf("looking up Business Rule asset %q: %w", ref, err)
		}
		switch len(matches) {
		case 0:
			problems = append(problems, fmt.Sprintf("no Business Rule asset is named %q — check the name, or pass the asset's UUID instead", ref))
		case 1:
			appendUnique(&ids, seen, matches[0].ID)
		default:
			problems = append(problems, fmt.Sprintf("%d Business Rule assets are named %q (%s) — pass the UUID of the one you mean instead of the name",
				len(matches), ref, describeMatches(matches)))
		}
	}
	return ids, problems, nil
}

// checkBusinessRuleAssetID returns a problem sentence when id is not an existing
// Business Rule asset, or "" when it is.
func checkBusinessRuleAssetID(ctx context.Context, collibraHttpClient *http.Client, id, businessRuleTypeID string) (string, error) {
	asset, err := GetAssetIdentity(ctx, collibraHttpClient, id)
	if errors.Is(err, ErrAssetNotFound) {
		return fmt.Sprintf("no asset has the UUID %s — check the id, or pass the Business Rule asset's exact name instead", id), nil
	}
	if err != nil {
		return "", fmt.Errorf("looking up asset %s: %w", id, err)
	}
	if asset.Type.ID != businessRuleTypeID {
		return fmt.Sprintf("asset %s (%q) is a %s, not a Business Rule asset — only Business Rule assets can be linked to a rule template",
			id, asset.Name, asset.Type.Name), nil
	}
	return "", nil
}

func appendUnique(ids *[]string, seen map[string]bool, id string) {
	if seen[id] {
		return
	}
	seen[id] = true
	*ids = append(*ids, id)
}

// describeMatches renders ambiguous matches as "uuid (domain)" so the agent can
// pick one without another round trip.
func describeMatches(matches []BusinessRuleAssetMatch) string {
	described := make([]string, 0, len(matches))
	for _, m := range matches {
		if m.DomainName != "" {
			described = append(described, fmt.Sprintf("%s in domain %q", m.ID, m.DomainName))
			continue
		}
		described = append(described, m.ID)
	}
	return strings.Join(described, "; ")
}

// isUUID reports whether ref has the canonical 8-4-4-4-12 hexadecimal shape.
func isUUID(ref string) bool {
	groups := strings.Split(ref, "-")
	if len(groups) != 5 {
		return false
	}
	for i, want := range []int{8, 4, 4, 4, 12} {
		if len(groups[i]) != want {
			return false
		}
		for _, r := range groups[i] {
			isHex := (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
			if !isHex {
				return false
			}
		}
	}
	return true
}
