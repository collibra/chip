package resolve_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/resolve"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	janeID  = "11111111-1111-1111-1111-111111111111"
	otherID = "22222222-2222-2222-2222-222222222222"
)

// userDir is a test double for the Collibra user directory. It records every
// request so a test can assert not just the outcome but which endpoint (if
// any) was called.
type userDir struct {
	users        []clients.EditAssetUser
	total        int // reported `total`; 0 means "as many as returned" (a complete page)
	nameQueries  []url.Values
	emailLookups []string
}

// client starts the double and returns a client pointed at it.
func (d *userDir) client(t *testing.T) *http.Client {
	t.Helper()
	mux := http.NewServeMux()

	// GET /rest/2.0/users: a loose `name` filter — a partial, case-insensitive
	// match over the fields in nameSearchFields, including the two
	// concatenations of first and last name.
	mux.HandleFunc("GET /rest/2.0/users", func(w http.ResponseWriter, r *http.Request) {
		d.nameQueries = append(d.nameQueries, r.URL.Query())
		needle := strings.ToLower(r.URL.Query().Get("name"))
		var matches []clients.EditAssetUser
		for _, u := range d.users {
			if needle == "" || matchesUserSearch(u, needle) {
				matches = append(matches, u)
			}
		}
		reported := d.total
		if reported <= 0 {
			reported = len(matches)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total": reported, "results": matches})
	})

	// GET /rest/2.0/users/email/{email}: the dedicated exact-match endpoint.
	mux.HandleFunc("GET /rest/2.0/users/email/{email}", func(w http.ResponseWriter, r *http.Request) {
		email := r.PathValue("email")
		d.emailLookups = append(d.emailLookups, email)
		for _, u := range d.users {
			if strings.EqualFold(u.EmailAddress, email) {
				_ = json.NewEncoder(w).Encode(u)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return testutil.NewClient(server)
}

// matchesUserSearch mirrors the server's nameSearchFields semantics: USERNAME,
// FIRSTNAME, LASTNAME, FIRSTNAME_LASTNAME and LASTNAME_FIRSTNAME.
func matchesUserSearch(u clients.EditAssetUser, needle string) bool {
	first, last := strings.TrimSpace(u.FirstName), strings.TrimSpace(u.LastName)
	for _, field := range []string{
		u.UserName, first, last,
		strings.TrimSpace(first + " " + last),
		strings.TrimSpace(last + " " + first),
	} {
		if field != "" && strings.Contains(strings.ToLower(field), needle) {
			return true
		}
	}
	return false
}

func jane() clients.EditAssetUser {
	return clients.EditAssetUser{
		ID: janeID, UserName: "jane.smith", EmailAddress: "jane.smith@example.com",
		FirstName: "Jane", LastName: "Smith",
	}
}

// AC-1: a first name plus last name matching exactly one user resolves.
func TestUserRef_FullNameResolvesToUUID(t *testing.T) {
	dir := &userDir{users: []clients.EditAssetUser{
		jane(),
		{ID: otherID, UserName: "bjones", FirstName: "Bob", LastName: "Jones"},
	}}
	user, err := resolve.UserRef(t.Context(), dir.client(t), "Jane Smith", resolve.Hints{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != janeID || user.UserName != "jane.smith" || user.FullName != "Jane Smith" {
		t.Fatalf("unexpected user: %+v", user)
	}
}

// AC-2: a name several users share is an error listing every candidate with
// its username and UUID — and never an email address.
func TestUserRef_AmbiguousNameListsEveryCandidate(t *testing.T) {
	dir := &userDir{users: []clients.EditAssetUser{
		jane(),
		{ID: otherID, UserName: "jsmith2", EmailAddress: "j.smith@example.com", FirstName: "Jane", LastName: "Smith"},
	}}
	_, err := resolve.UserRef(t.Context(), dir.client(t), "jane smith", resolve.Hints{})
	if err == nil {
		t.Fatal("expected an ambiguity error")
	}
	for _, want := range []string{"ambiguous", janeID, otherID, "jane.smith", "jsmith2", "Jane Smith"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "@") {
		t.Fatalf("error must not leak an email address: %q", err)
	}
}

// AC-3: nothing matched — the error names every accepted input form.
func TestUserRef_NoMatchNamesAcceptedForms(t *testing.T) {
	dir := &userDir{users: []clients.EditAssetUser{jane()}}
	_, err := resolve.UserRef(t.Context(), dir.client(t), "Nobody Here", resolve.Hints{})
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	for _, want := range []string{"UUID", "email address", "username", "First Last"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name the %q form", err, want)
		}
	}
}

// AC-4: a UUID resolves without issuing any user lookup at all.
func TestUserID_UUIDPassesThroughWithoutLookup(t *testing.T) {
	dir := &userDir{users: []clients.EditAssetUser{jane()}}
	id, err := resolve.UserID(t.Context(), dir.client(t), janeID, resolve.Hints{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != janeID {
		t.Fatalf("id = %q, want %q", id, janeID)
	}
	if len(dir.nameQueries) != 0 || len(dir.emailLookups) != 0 {
		t.Fatalf("expected no lookup, got name=%v email=%v", dir.nameQueries, dir.emailLookups)
	}
}

// AC-5: an email goes to the exact email endpoint, not the name filter.
func TestUserRef_EmailUsesExactEmailEndpoint(t *testing.T) {
	dir := &userDir{users: []clients.EditAssetUser{jane()}}
	user, err := resolve.UserRef(t.Context(), dir.client(t), "jane.smith@example.com", resolve.Hints{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != janeID {
		t.Fatalf("id = %q, want %q", user.ID, janeID)
	}
	if len(dir.emailLookups) != 1 || dir.emailLookups[0] != "jane.smith@example.com" {
		t.Fatalf("expected one email lookup, got %v", dir.emailLookups)
	}
	if len(dir.nameQueries) != 0 {
		t.Fatalf("expected no name query, got %v", dir.nameQueries)
	}
}

// AC-6: an exact username wins over another user's display name. The username
// with a space in it is contrived — it is the only way to make one value match
// both forms and so pin the precedence.
func TestUserRef_ExactUsernameBeatsDisplayName(t *testing.T) {
	dir := &userDir{users: []clients.EditAssetUser{
		{ID: janeID, UserName: "Jane Smith", FirstName: "Janet", LastName: "Smithers"},
		{ID: otherID, UserName: "jsmith2", FirstName: "Jane", LastName: "Smith"},
	}}
	id, err := resolve.UserID(t.Context(), dir.client(t), "jane smith", resolve.Hints{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != janeID {
		t.Fatalf("id = %q, want the exact username match %q", id, janeID)
	}
}

// AC-7 / AC-8: the name search states its fields and its disabled-user
// handling explicitly rather than inheriting the server's defaults.
func TestUserRef_NameSearchSendsFieldsAndIncludeDisabled(t *testing.T) {
	dir := &userDir{users: []clients.EditAssetUser{jane()}}
	if _, err := resolve.UserID(t.Context(), dir.client(t), "Jane Smith", resolve.Hints{}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(dir.nameQueries) != 1 {
		t.Fatalf("expected exactly one name query, got %d", len(dir.nameQueries))
	}
	q := dir.nameQueries[0]
	want := []string{"USERNAME", "FIRSTNAME", "LASTNAME", "FIRSTNAME_LASTNAME", "LASTNAME_FIRSTNAME"}
	if got := q["nameSearchFields"]; !equalStrings(got, want) {
		t.Fatalf("nameSearchFields = %v, want %v", got, want)
	}
	if got := q.Get("includeDisabled"); got != "false" {
		t.Fatalf("includeDisabled = %q, want %q", got, "false")
	}
}

// A name cannot be called unambiguous over a page the server says is
// incomplete: a second holder of it may sit outside the window, and resolving
// anyway would be the silent wrong pick this package exists to prevent.
func TestUserRef_TruncatedSearchDoesNotResolveByName(t *testing.T) {
	dir := &userDir{users: []clients.EditAssetUser{jane()}, total: 250}
	_, err := resolve.UserRef(t.Context(), dir.client(t), "Jane Smith", resolve.Hints{})
	if err == nil {
		t.Fatal("expected a truncation error rather than a confident single match")
	}
	for _, want := range []string{"250", "Jane Smith", "UUID"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
}

// An exact username still resolves over a truncated page — usernames are
// unique, so the window cannot hide a second holder.
func TestUserRef_ExactUsernameResolvesDespiteTruncation(t *testing.T) {
	dir := &userDir{users: []clients.EditAssetUser{jane()}, total: 250}
	id, err := resolve.UserID(t.Context(), dir.client(t), "jane.smith", resolve.Hints{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != janeID {
		t.Fatalf("id = %q, want %q", id, janeID)
	}
}

// The not-found error says only enabled accounts are searched, so a model
// hunting a deactivated leaver stops trying name variants.
func TestUserRef_NotFoundMentionsEnabledAccountsOnly(t *testing.T) {
	dir := &userDir{users: []clients.EditAssetUser{jane()}}
	_, err := resolve.UserRef(t.Context(), dir.client(t), "Nobody Here", resolve.Hints{})
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	if !strings.Contains(err.Error(), "enabled") {
		t.Fatalf("error %q does not mention that only enabled accounts are searched", err)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
