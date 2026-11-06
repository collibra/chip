package get_asset_operating_model

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/clients"
	"github.com/google/uuid"
)

// maxNameMatches caps how many name matches are fetched while looking for an
// exact one.
const maxNameMatches = 50

// resolveAssetType tries UUID, then publicId, then a case-insensitive exact name
// match, and returns the first strategy that succeeds.
func resolveAssetType(ctx context.Context, collibraClient *http.Client, value string) (*clients.PrepareCreateAssetType, error) {
	if isUUID(value) {
		if assetType, err := clients.GetAssetTypeByID(ctx, collibraClient, value); err == nil {
			return assetType, nil
		}
	}
	if assetType, err := clients.GetAssetTypeByPublicID(ctx, collibraClient, value); err == nil {
		return assetType, nil
	}
	matches, _, err := clients.SearchAssetTypesByName(ctx, collibraClient, value, maxNameMatches)
	if err != nil {
		return nil, fmt.Errorf("searching for asset type %q: %w", value, err)
	}
	var exact []clients.PrepareCreateAssetType
	for _, match := range matches {
		if strings.EqualFold(match.Name, value) {
			exact = append(exact, match)
		}
	}
	switch len(exact) {
	case 1:
		return &exact[0], nil
	case 0:
		return nil, fmt.Errorf("no asset type matches %q", value)
	default:
		return nil, fmt.Errorf("asset type %q is ambiguous: %d exact matches, use its UUID or publicId", value, len(exact))
	}
}

// resolveDomain tries UUID, then a case-insensitive exact name match. Domains
// have no publicId.
func resolveDomain(ctx context.Context, collibraClient *http.Client, value string) (*clients.PrepareCreateDomain, error) {
	if isUUID(value) {
		if domain, err := clients.GetDomainByID(ctx, collibraClient, value); err == nil {
			return domain, nil
		}
	}
	matches, _, err := clients.SearchDomainsByName(ctx, collibraClient, value, maxNameMatches)
	if err != nil {
		return nil, fmt.Errorf("searching for domain %q: %w", value, err)
	}
	var exact []clients.PrepareCreateDomain
	for _, match := range matches {
		if strings.EqualFold(match.Name, value) {
			exact = append(exact, match)
		}
	}
	switch len(exact) {
	case 1:
		return &exact[0], nil
	case 0:
		return nil, fmt.Errorf("no domain matches %q", value)
	default:
		return nil, fmt.Errorf("domain %q is ambiguous: %d exact matches, use its UUID", value, len(exact))
	}
}

// isUUID reports whether s is a UUID in the canonical 8-4-4-4-12 form.
func isUUID(s string) bool {
	return len(s) == 36 && uuid.Validate(s) == nil
}
