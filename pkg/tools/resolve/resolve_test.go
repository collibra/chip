package resolve_test

import (
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/resolve"
)

func TestPickMatch_SingleExactMatchWins(t *testing.T) {
	candidates := []resolve.NamedRef{
		{ID: "id-1", Name: "Marketing"},
		{ID: "id-2", Name: "Marketing Analytics"}, // the search is a substring match
	}
	id, err := resolve.PickMatch("domain", " marketing ", candidates, resolve.Hints{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "id-1" {
		t.Fatalf("id = %q, want %q", id, "id-1")
	}
}

func TestPickMatch_NotFoundSuggestsCandidatesAndHint(t *testing.T) {
	candidates := []resolve.NamedRef{{ID: "id-2", Name: "Marketing Analytics"}}
	_, err := resolve.PickMatch("domain", "Markting", candidates, resolve.Hints{NotFound: "Accepted forms: a name or a UUID."})
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	for _, want := range []string{`no domain matching "Markting" found`, "Marketing Analytics", "Accepted forms: a name or a UUID."} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
}

func TestPickMatch_AmbiguityCarriesCandidatesContextAndHint(t *testing.T) {
	candidates := []resolve.NamedRef{
		{ID: "id-1", Name: "Marketing", Ctx: "type: Business Asset Domain"},
		{ID: "id-2", Name: "Marketing", Ctx: "type: Glossary"},
	}
	_, err := resolve.PickMatch("domain", "Marketing", candidates, resolve.Hints{Ambiguity: "pass the UUID in domainFilter to disambiguate"})
	if err == nil {
		t.Fatal("expected an ambiguity error")
	}
	for _, want := range []string{"ambiguous", "2 domains", "id-1", "id-2", "type: Glossary", "pass the UUID in domainFilter to disambiguate"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not contain %q", err, want)
		}
	}
}

func TestSuggestionSuffix_TruncatesAndSorts(t *testing.T) {
	got := resolve.SuggestionSuffix("Valid statuses", []string{"Obsolete", "Accepted", "Candidate"}, 2)
	want := " Valid statuses available: Accepted, Candidate (and 1 more)."
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if resolve.SuggestionSuffix("Valid statuses", nil, 2) != "" {
		t.Fatal("expected an empty suffix when there is nothing to suggest")
	}
}
