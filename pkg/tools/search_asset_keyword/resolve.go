package search_asset_keyword

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/collibra/chip/pkg/clients"
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

// normalize lowercases and trims so a typed-in name matches regardless of case
// or stray whitespace.
func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// namedRef is a candidate match for a typed-in filter value. ctx holds optional
// disambiguating context (e.g. a domain's type) shown only when a name is
// ambiguous.
type namedRef struct {
	id   string
	name string
	ctx  string
}

// suggestionSuffix renders a short list of valid names to append to a
// "not found" error so the model can self-correct in one step instead of
// round-tripping through another tool. Mirrors edit_asset's helper.
func suggestionSuffix(label string, names []string, max int) string {
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	if len(names) <= max {
		return fmt.Sprintf(" %s available: %s.", label, strings.Join(names, ", "))
	}
	return fmt.Sprintf(" %s available: %s (and %d more).", label, strings.Join(names[:max], ", "), len(names)-max)
}

// pickMatch reduces the candidate matches for a single typed-in name to one
// UUID, or returns a self-correcting error. The "did you mean" list is drawn
// from the candidates themselves — for the enumerable sets (status, domain
// type) those are the full set; for name-searched sets they are the server-side
// substring matches.
func pickMatch(label, param, query string, candidates []namedRef) (string, error) {
	var exact []namedRef
	for _, c := range candidates {
		if normalize(c.name) == normalize(query) {
			exact = append(exact, c)
		}
	}

	switch len(exact) {
	case 1:
		return exact[0].id, nil
	case 0:
		names := make([]string, 0, len(candidates))
		for _, c := range candidates {
			names = append(names, c.name)
		}
		return "", fmt.Errorf("no %s matching %q found.%s", label, query, suggestionSuffix("Valid "+label+"s", names, 15))
	default:
		lines := make([]string, 0, len(exact))
		for _, c := range exact {
			if c.ctx != "" {
				lines = append(lines, fmt.Sprintf("%s (id %s, %s)", c.name, c.id, c.ctx))
			} else {
				lines = append(lines, fmt.Sprintf("%s (id %s)", c.name, c.id))
			}
		}
		return "", fmt.Errorf("%q is ambiguous — %d %ss share that name; pass the UUID in %s to disambiguate: %s",
			query, len(exact), label, param, strings.Join(lines, "; "))
	}
}

// resolveFilter maps each value in a filter slice to a UUID. A value that is
// already a UUID passes through; otherwise it is looked up via find and reduced
// by pickMatch. find is invoked only for non-UUID values, so a filter given
// purely as UUIDs costs no extra requests. Blank values are dropped.
func resolveFilter(label, param string, values []string, find func(string) ([]namedRef, error)) ([]string, error) {
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
		id, err := pickMatch(label, param, v, candidates)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// memoize wraps a full-set loader so the underlying fetch runs at most once,
// only when a name (not a UUID) actually needs resolving. The same cached set is
// returned for every name in the filter.
func memoize(load func() ([]namedRef, error)) func(string) ([]namedRef, error) {
	var (
		cached []namedRef
		loaded bool
	)
	return func(string) ([]namedRef, error) {
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
		memoize(func() ([]namedRef, error) {
			statuses, err := clients.ListStatuses(ctx, client)
			if err != nil {
				return nil, err
			}
			refs := make([]namedRef, len(statuses))
			for i, s := range statuses {
				refs[i] = namedRef{id: s.ID, name: s.Name}
			}
			return refs, nil
		}))
	if err != nil {
		return err
	}
	input.StatusFilter = resolved

	// domain type — small enumerable set; fetch once (lazily) and match in memory.
	resolved, err = resolveFilter("domain type", "domainTypeFilter", input.DomainTypeFilter,
		memoize(func() ([]namedRef, error) {
			domainTypes, err := clients.ListDomainTypes(ctx, client)
			if err != nil {
				return nil, err
			}
			refs := make([]namedRef, len(domainTypes))
			for i, dt := range domainTypes {
				refs[i] = namedRef{id: dt.ID, name: dt.Name}
			}
			return refs, nil
		}))
	if err != nil {
		return err
	}
	input.DomainTypeFilter = resolved

	// asset type — potentially large; search by name per value.
	resolved, err = resolveFilter("asset type", "assetTypeFilter", input.AssetTypeFilter,
		func(name string) ([]namedRef, error) {
			types, _, err := clients.SearchAssetTypesByName(ctx, client, name, 50)
			if err != nil {
				return nil, err
			}
			refs := make([]namedRef, len(types))
			for i, t := range types {
				refs[i] = namedRef{id: t.ID, name: t.Name}
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
		func(name string) ([]namedRef, error) {
			domains, _, err := clients.SearchDomainsByName(ctx, client, name, 50)
			if err != nil {
				return nil, err
			}
			refs := make([]namedRef, len(domains))
			for i, d := range domains {
				ref := namedRef{id: d.ID, name: d.Name}
				if d.Type != nil && d.Type.Name != "" {
					ref.ctx = "type: " + d.Type.Name
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
		func(name string) ([]namedRef, error) {
			communities, err := clients.SearchCommunitiesByName(ctx, client, name, 50)
			if err != nil {
				return nil, err
			}
			refs := make([]namedRef, len(communities))
			for i, c := range communities {
				refs[i] = namedRef{id: c.ID, name: c.Name}
			}
			return refs, nil
		})
	if err != nil {
		return err
	}
	input.CommunityFilter = resolved

	// created-by — resolve a username to its user UUID via the exact-match
	// finder (the /users name filter is a loose partial search).
	resolved, err = resolveFilter("user", "createdByFilter", input.CreatedByFilter,
		func(name string) ([]namedRef, error) {
			user, err := clients.FindUserByUsername(ctx, client, name)
			if err != nil {
				return nil, err
			}
			if user == nil {
				return nil, nil
			}
			return []namedRef{{id: user.ID, name: user.UserName}}, nil
		})
	if err != nil {
		return err
	}
	input.CreatedByFilter = resolved

	return nil
}
