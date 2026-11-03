package resolve

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/clients"
	"github.com/google/uuid"
)

// Community resolves a community reference — a UUID or an exact
// (case-insensitive) name — to that community. A UUID is looked up directly so
// a missing one fails loudly; a name that matches several communities is an
// error listing each with its UUID and parent, never an arbitrary pick.
func Community(ctx context.Context, client *http.Client, value string, hints Hints) (clients.CommunityDetails, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return clients.CommunityDetails{}, fmt.Errorf("a community is required (its UUID or name)")
	}
	if _, err := uuid.Parse(v); err == nil {
		c, err := clients.GetCommunity(ctx, client, v)
		if err != nil {
			return clients.CommunityDetails{}, fmt.Errorf("community %s not found or unreadable: %w", v, err)
		}
		return *c, nil
	}

	found, err := clients.SearchCommunities(ctx, client, v)
	if err != nil {
		return clients.CommunityDetails{}, fmt.Errorf("looking up community %q: %w", v, err)
	}
	byID := make(map[string]clients.CommunityDetails, len(found))
	candidates := make([]NamedRef, len(found))
	for i, c := range found {
		byID[c.ID] = c
		ctxText := "top-level"
		if c.Parent != nil {
			ctxText = "parent " + c.Parent.Name
		}
		candidates[i] = NamedRef{ID: c.ID, Name: c.Name, Ctx: ctxText}
	}
	id, err := PickMatch("community", v, candidates, hints)
	if err != nil {
		return clients.CommunityDetails{}, err
	}
	return byID[id], nil
}

// Domain resolves a domain reference — a UUID or an exact (case-insensitive)
// name — to that domain, with the same contract as Community.
func Domain(ctx context.Context, client *http.Client, value string, hints Hints) (clients.DomainDetails, error) {
	v := strings.TrimSpace(value)
	if v == "" {
		return clients.DomainDetails{}, fmt.Errorf("a domain is required (its UUID or name)")
	}
	if _, err := uuid.Parse(v); err == nil {
		d, err := clients.GetDomain(ctx, client, v)
		if err != nil {
			return clients.DomainDetails{}, fmt.Errorf("domain %s not found or unreadable: %w", v, err)
		}
		return *d, nil
	}

	found, err := clients.SearchDomains(ctx, client, v)
	if err != nil {
		return clients.DomainDetails{}, fmt.Errorf("looking up domain %q: %w", v, err)
	}
	byID := make(map[string]clients.DomainDetails, len(found))
	candidates := make([]NamedRef, len(found))
	for i, d := range found {
		byID[d.ID] = d
		ctxText := ""
		if d.Community != nil {
			ctxText = "community " + d.Community.Name
		}
		candidates[i] = NamedRef{ID: d.ID, Name: d.Name, Ctx: ctxText}
	}
	id, err := PickMatch("domain", v, candidates, hints)
	if err != nil {
		return clients.DomainDetails{}, err
	}
	return byID[id], nil
}
