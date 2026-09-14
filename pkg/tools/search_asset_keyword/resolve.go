package search_asset_keyword

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/resolve"
	"github.com/google/uuid"
)

// The search filters (status, community, domain, domain type, asset type,
// created-by) all key off UUIDs server-side, but users — and the LLM relaying
// them — speak in names ("Obsolete", "Marketing"). Without resolution the model
// has no reliable way to discover those UUIDs and ends up guessing OOTB
// defaults, which silently break on instances with custom values. resolveFilters
// lets every filter accept a name OR a UUID: UUIDs pass through untouched
// (backward compatible) and names are resolved to UUIDs here, mirroring the
// forgiving name matching edit_asset already does.

// resolveFilter maps each value in a filter slice to a UUID. A value that is
// already a UUID passes through; otherwise it is looked up via find and reduced
// by resolve.PickMatch. find is invoked only for non-UUID values, so a filter
// given purely as UUIDs costs no extra requests. Blank values are dropped.
func resolveFilter(label, param string, values []string, find func(string) ([]resolve.NamedRef, error)) ([]string, error) {
	if len(values) == 0 {
		return values, nil
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, err := uuid.Parse(v); err == nil {
			out = append(out, v)
			continue
		}
		candidates, err := find(v)
		if err != nil {
			return nil, fmt.Errorf("resolving %s %q: %w", label, v, err)
		}
		id, err := resolve.PickMatch(label, v, candidates, filterHints(param))
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// filterHints tells the caller how to disambiguate a name that several
// resources share: this tool takes the UUID in the very filter parameter the
// name came from.
func filterHints(param string) resolve.Hints {
	return resolve.Hints{Ambiguity: fmt.Sprintf("pass the UUID in %s to disambiguate", param)}
}

// memoize wraps a full-set loader so the underlying fetch runs at most once,
// only when a name (not a UUID) actually needs resolving. The same cached set is
// returned for every name in the filter.
func memoize(load func() ([]resolve.NamedRef, error)) func(string) ([]resolve.NamedRef, error) {
	var (
		cached []resolve.NamedRef
		loaded bool
	)
	return func(string) ([]resolve.NamedRef, error) {
		if !loaded {
			refs, err := load()
			if err != nil {
				return nil, err
			}
			cached = refs
			loaded = true
		}
		return cached, nil
	}
}

// resolveFilters rewrites every name-or-UUID filter on input into UUIDs in
// place, so the rest of the handler can build the search request unchanged.
func resolveFilters(ctx context.Context, client *http.Client, input *Input) error {
	// status — small enumerable set; fetch once (lazily) and match in memory.
	resolved, err := resolveFilter("status", "statusFilter", input.StatusFilter,
		memoize(func() ([]resolve.NamedRef, error) {
			statuses, err := clients.ListStatuses(ctx, client)
			if err != nil {
				return nil, err
			}
			refs := make([]resolve.NamedRef, len(statuses))
			for i, s := range statuses {
				refs[i] = resolve.NamedRef{ID: s.ID, Name: s.Name}
			}
			return refs, nil
		}))
	if err != nil {
		return err
	}
	input.StatusFilter = resolved

	// domain type — small enumerable set; fetch once (lazily) and match in memory.
	resolved, err = resolveFilter("domain type", "domainTypeFilter", input.DomainTypeFilter,
		memoize(func() ([]resolve.NamedRef, error) {
			domainTypes, err := clients.ListDomainTypes(ctx, client)
			if err != nil {
				return nil, err
			}
			refs := make([]resolve.NamedRef, len(domainTypes))
			for i, dt := range domainTypes {
				refs[i] = resolve.NamedRef{ID: dt.ID, Name: dt.Name}
			}
			return refs, nil
		}))
	if err != nil {
		return err
	}
	input.DomainTypeFilter = resolved

	// asset type — potentially large; search by name per value.
	resolved, err = resolveFilter("asset type", "assetTypeFilter", input.AssetTypeFilter,
		func(name string) ([]resolve.NamedRef, error) {
			types, _, err := clients.SearchAssetTypesByName(ctx, client, name, 50)
			if err != nil {
				return nil, err
			}
			refs := make([]resolve.NamedRef, len(types))
			for i, t := range types {
				refs[i] = resolve.NamedRef{ID: t.ID, Name: t.Name}
			}
			return refs, nil
		})
	if err != nil {
		return err
	}
	input.AssetTypeFilter = resolved

	// domain — potentially large; search by name per value, with domain type as
	// disambiguating context when names collide.
	resolved, err = resolveFilter("domain", "domainFilter", input.DomainFilter,
		func(name string) ([]resolve.NamedRef, error) {
			domains, _, err := clients.SearchDomainsByName(ctx, client, name, 50)
			if err != nil {
				return nil, err
			}
			refs := make([]resolve.NamedRef, len(domains))
			for i, d := range domains {
				ref := resolve.NamedRef{ID: d.ID, Name: d.Name}
				if d.Type != nil && d.Type.Name != "" {
					ref.Ctx = "type: " + d.Type.Name
				}
				refs[i] = ref
			}
			return refs, nil
		})
	if err != nil {
		return err
	}
	input.DomainFilter = resolved

	// community — potentially large; search by name per value.
	resolved, err = resolveFilter("community", "communityFilter", input.CommunityFilter,
		func(name string) ([]resolve.NamedRef, error) {
			communities, err := clients.SearchCommunitiesByName(ctx, client, name, 50)
			if err != nil {
				return nil, err
			}
			refs := make([]resolve.NamedRef, len(communities))
			for i, c := range communities {
				refs[i] = resolve.NamedRef{ID: c.ID, Name: c.Name}
			}
			return refs, nil
		})
	if err != nil {
		return err
	}
	input.CommunityFilter = resolved

	// created-by — users have their own precedence chain (UUID, email, exact
	// username, then display name), so they bypass the generic finder.
	resolved, err = resolveUsers(ctx, client, input.CreatedByFilter)
	if err != nil {
		return err
	}
	input.CreatedByFilter = resolved

	return nil
}

// resolveUsers maps each createdByFilter value to a user UUID via the shared
// user resolver. Blank values are dropped; UUID pass-through and the "which
// user did you mean" errors are the resolver's, so a name several people share
// comes back as the candidate list rather than as no match.
func resolveUsers(ctx context.Context, client *http.Client, values []string) ([]string, error) {
	if len(values) == 0 {
		return values, nil
	}
	out := make([]string, 0, len(values))
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			continue
		}
		id, err := resolve.UserID(ctx, client, v, filterHints("createdByFilter"))
		if err != nil {
			// Same wrapping the generic resolveFilter applies, so all six
			// filters of this tool report a failure the same shape.
			return nil, fmt.Errorf("resolving user %q: %w", v, err)
		}
		out = append(out, id)
	}
	return out, nil
}
