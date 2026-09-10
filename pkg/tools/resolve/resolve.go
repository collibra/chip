// Package resolve provides the shared name-to-UUID resolution helpers CHIP
// tools use to accept human-readable values where the Collibra APIs demand
// UUIDs. Users — and the LLM relaying them — speak in names ("Obsolete",
// "Marketing", "Jane Smith"); without resolution the model has no reliable way
// to discover the UUIDs and ends up guessing.
//
// The contract is the same for every kind of reference: exactly one match
// resolves silently, no match returns a self-correcting error listing what
// would have been valid, and an ambiguous name returns an error naming every
// candidate with its UUID — never a silent pick of the first row.
//
// Helpers return Go errors that the MCP SDK wraps as tool execution errors
// (isError: true), letting the calling model see the problem and self-correct.
// Mirrors the pkg/tools/validation precedent: one small shared package rather
// than a copy per tool.
package resolve

import (
	"fmt"
	"sort"
	"strings"
)

// NamedRef is a candidate match for a typed-in value. Ctx holds optional
// disambiguating context (e.g. a domain's type, a user's username) shown only
// when a name is ambiguous.
type NamedRef struct {
	ID   string
	Name string
	Ctx  string
}

// Hints customise the two failure messages for the calling tool's shape.
// Both are optional: an empty field falls back to the generic wording.
//
// NotFound is appended to the "no <label> matching <value> found" error and
// should name the input forms the caller accepts. Ambiguity replaces the
// instruction clause in the ambiguity error — a filter passes "pass the UUID in
// <param> to disambiguate", a lookup tool with no such parameter says what to
// do instead.
type Hints struct {
	NotFound  string
	Ambiguity string
}

// defaultAmbiguityHint is used when a caller supplies no Ambiguity wording.
const defaultAmbiguityHint = "pass the UUID instead to disambiguate"

// PickMatch reduces the candidate matches for a single typed-in name to one
// UUID, or returns a self-correcting error. The "did you mean" list is drawn
// from the candidates themselves — for the enumerable sets (status, domain
// type) those are the full set; for name-searched sets they are the server-side
// substring matches.
func PickMatch(label, query string, candidates []NamedRef, hints Hints) (string, error) {
	exact := exactMatches(query, candidates)

	switch len(exact) {
	case 1:
		return exact[0].ID, nil
	case 0:
		return "", notFoundError(label, query, candidates, hints)
	default:
		return "", ambiguousError(label, query, exact, hints)
	}
}

// exactMatches keeps the candidates whose name equals the query, ignoring case
// and surrounding whitespace. The server-side searches are substring matches,
// so this is what turns "Table" into the Table asset type rather than every
// type containing the word.
func exactMatches(query string, candidates []NamedRef) []NamedRef {
	var exact []NamedRef
	for _, c := range candidates {
		if normalize(c.Name) == normalize(query) {
			exact = append(exact, c)
		}
	}
	return exact
}

// notFoundError reports that nothing matched, listing the near misses the
// search did return (and, when the caller supplies one, the input forms it
// accepts) so the model can retry without a round trip to the user.
func notFoundError(label, query string, candidates []NamedRef, hints Hints) error {
	names := make([]string, 0, len(candidates))
	for _, c := range candidates {
		names = append(names, c.Name)
	}
	msg := fmt.Sprintf("no %s matching %q found.%s", label, query, SuggestionSuffix("Valid "+label+"s", names, 15))
	if hints.NotFound != "" {
		msg += " " + hints.NotFound
	}
	return fmt.Errorf("%s", msg)
}

// ambiguousError reports that several candidates share the name, listing each
// one with its UUID. It stays an error rather than a success payload: a model
// holding a list of plausible ids and a pending call will otherwise pick one.
func ambiguousError(label, query string, exact []NamedRef, hints Hints) error {
	hint := hints.Ambiguity
	if hint == "" {
		hint = defaultAmbiguityHint
	}
	lines := make([]string, 0, len(exact))
	for _, c := range exact {
		if c.Ctx != "" {
			lines = append(lines, fmt.Sprintf("%s (id %s, %s)", c.Name, c.ID, c.Ctx))
		} else {
			lines = append(lines, fmt.Sprintf("%s (id %s)", c.Name, c.ID))
		}
	}
	return fmt.Errorf("%q is ambiguous — %d %ss share that name; %s: %s",
		query, len(exact), label, hint, strings.Join(lines, "; "))
}

// SuggestionSuffix renders a short list of valid names to append to a
// "not found" error so the model can self-correct in one step instead of
// round-tripping through another tool.
func SuggestionSuffix(label string, names []string, max int) string {
	if len(names) == 0 {
		return ""
	}
	sort.Strings(names)
	if len(names) <= max {
		return fmt.Sprintf(" %s available: %s.", label, strings.Join(names, ", "))
	}
	return fmt.Sprintf(" %s available: %s (and %d more).", label, strings.Join(names[:max], ", "), len(names)-max)
}

// normalize lowercases and trims so a typed-in name matches regardless of case
// or stray whitespace.
func normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
