package resolve

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/clients"
	"github.com/google/uuid"
)

// Ref is a resolved reference. Name is empty when the caller supplied a UUID,
// because a UUID passes through without a lookup.
type Ref struct {
	ID   string
	Name string
}

// Community resolves a community given as a UUID or a display name. A UUID
// passes through with no request; a name is matched exactly (case-insensitive)
// against the server-side substring search, and an ambiguous name is an error
// naming every candidate with its UUID.
func Community(ctx context.Context, client *http.Client, param, value string) (Ref, error) {
	value = strings.TrimSpace(value)
	if _, err := uuid.Parse(value); err == nil {
		return Ref{ID: value}, nil
	}
	communities, err := clients.SearchCommunitiesByName(ctx, client, value, 50)
	if err != nil {
		return Ref{}, fmt.Errorf("resolving community %q: %w", value, err)
	}
	refs := make([]NamedRef, len(communities))
	for i, c := range communities {
		refs[i] = NamedRef{ID: c.ID, Name: c.Name}
	}
	hints := Hints{
		NotFound:   "Pass the community's UUID or its exact name.",
		Ambiguity:  fmt.Sprintf("pass the UUID in %s to disambiguate", param),
		Candidates: "Similar communities",
	}
	return pickRef("community", value, refs, hints)
}

// DomainType resolves a domain type given as a UUID or a display name. Domain
// types are a small enumerable set, so the full list is fetched and matched in
// memory; a UUID passes through with no request.
func DomainType(ctx context.Context, client *http.Client, param, value string) (Ref, error) {
	value = strings.TrimSpace(value)
	if _, err := uuid.Parse(value); err == nil {
		return Ref{ID: value}, nil
	}
	types, err := clients.ListDomainTypes(ctx, client)
	if err != nil {
		return Ref{}, fmt.Errorf("resolving domain type %q: %w", value, err)
	}
	refs := make([]NamedRef, len(types))
	for i, t := range types {
		refs[i] = NamedRef{ID: t.ID, Name: t.Name}
	}
	hints := Hints{
		NotFound:  "Pass the domain type's UUID or its exact name.",
		Ambiguity: fmt.Sprintf("pass the UUID in %s to disambiguate", param),
	}
	return pickRef("domain type", value, refs, hints)
}

// pickRef reduces candidates to the single exact match and keeps its name so
// callers can echo it back.
func pickRef(label, value string, refs []NamedRef, hints Hints) (Ref, error) {
	id, err := PickMatch(label, value, refs, hints)
	if err != nil {
		return Ref{}, err
	}
	for _, r := range refs {
		if r.ID == id {
			return Ref{ID: id, Name: r.Name}, nil
		}
	}
	return Ref{ID: id}, nil
}
