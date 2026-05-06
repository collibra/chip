package resolve_domain

import (
	"context"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Query string `json:"query" jsonschema:"Required. Domain name (free-form, used for fuzzy lookup) or domain UUID. UUIDs are matched first via GET /domains/{id}; otherwise treated as a name and resolved via GET /domains?name=..."`
}

// Output is the union of three outcomes — exactly one of Match / Candidates /
// NotFound is populated. Reason is set on NotFound or Candidates to help the
// caller compose the next user prompt.
type Output struct {
	Match      *Domain  `json:"match,omitempty" jsonschema:"Set when exactly one domain matches the query"`
	Candidates []Domain `json:"candidates,omitempty" jsonschema:"Set when multiple domains match the name; the caller must pick one"`
	NotFound   bool     `json:"notFound,omitempty" jsonschema:"True when no domain matches"`
	Reason     string   `json:"reason,omitempty" jsonschema:"Explanation when match is empty (notFound or multi-match)"`
}

type Domain struct {
	ID        string `json:"id" jsonschema:"Domain UUID"`
	Name      string `json:"name" jsonschema:"Domain display name"`
	Community string `json:"community,omitempty" jsonschema:"Parent community name"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "resolve_domain",
		Description: "Resolve a Collibra domain by name or UUID. Returns a single match, a list of candidates if the name is ambiguous, or notFound. Used by the create-control flow to confirm the default save-time domain.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		res, err := clients.ResolveDomain(ctx, collibraClient, input.Query)
		if err != nil {
			return Output{}, err
		}
		return mapResolution(res), nil
	}
}

func mapResolution(res *clients.DomainResolution) Output {
	if res.Single != nil {
		return Output{Match: &Domain{ID: res.Single.ID, Name: res.Single.Name, Community: res.Single.Community}}
	}
	if len(res.Candidates) > 0 {
		out := Output{Reason: "multiple matches; pick one by id and re-call with the UUID"}
		out.Candidates = make([]Domain, len(res.Candidates))
		for i, d := range res.Candidates {
			out.Candidates[i] = Domain{ID: d.ID, Name: d.Name, Community: d.Community}
		}
		return out
	}
	return Output{NotFound: true, Reason: res.Reason}
}
