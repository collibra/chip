package get_user_id_by_name_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tool "github.com/collibra/chip/pkg/tools/get_user_id_by_name"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// usersServer serves GET /rest/2.0/users the way the real endpoint does: a
// partial, case-insensitive match over username, first name, last name and
// both concatenations of first and last name.
func usersServer(t *testing.T, users ...clients.EditAssetUser) *http.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/users", func(w http.ResponseWriter, r *http.Request) {
		needle := strings.ToLower(r.URL.Query().Get("name"))
		var matches []clients.EditAssetUser
		for _, u := range users {
			full := strings.TrimSpace(u.FirstName + " " + u.LastName)
			reversed := strings.TrimSpace(u.LastName + " " + u.FirstName)
			for _, field := range []string{u.UserName, u.FirstName, u.LastName, full, reversed} {
				if field != "" && strings.Contains(strings.ToLower(field), needle) {
					matches = append(matches, u)
					break
				}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total": len(matches), "results": matches})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return testutil.NewClient(server)
}

// AC-9: an unambiguous name returns that user's UUID.
func TestGetUserIDByName_UnambiguousName(t *testing.T) {
	client := usersServer(t,
		clients.EditAssetUser{ID: "u-1", UserName: "jane.smith", EmailAddress: "jane@example.com", FirstName: "Jane", LastName: "Smith"},
		clients.EditAssetUser{ID: "u-2", UserName: "bjones", FirstName: "Bob", LastName: "Jones"},
	)
	out, err := tool.NewTool(client).Handler(t.Context(), tool.Input{Name: "Jane Smith"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.UserID != "u-1" || out.FullName != "Jane Smith" || out.Username != "jane.smith" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

// A shared name is an error carrying the candidates, not a successful payload
// the model could pick from.
func TestGetUserIDByName_AmbiguousNameIsAnError(t *testing.T) {
	client := usersServer(t,
		clients.EditAssetUser{ID: "u-1", UserName: "jane.smith", EmailAddress: "jane@example.com", FirstName: "Jane", LastName: "Smith"},
		clients.EditAssetUser{ID: "u-2", UserName: "jsmith2", EmailAddress: "j.smith@example.com", FirstName: "Jane", LastName: "Smith"},
	)
	out, err := tool.NewTool(client).Handler(t.Context(), tool.Input{Name: "Jane Smith"})
	if err == nil {
		t.Fatalf("expected an ambiguity error, got %+v", out)
	}
	if out.UserID != "" {
		t.Fatalf("expected no userId alongside the error, got %q", out.UserID)
	}
	for _, want := range []string{"ambiguous", "u-1", "u-2", "jane.smith", "jsmith2"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "@") {
		t.Fatalf("error must not leak an email address: %q", err)
	}
}

func TestGetUserIDByName_UnknownNameNamesAcceptedForms(t *testing.T) {
	client := usersServer(t, clients.EditAssetUser{ID: "u-1", UserName: "jane.smith", FirstName: "Jane", LastName: "Smith"})
	_, err := tool.NewTool(client).Handler(t.Context(), tool.Input{Name: "Nobody Here"})
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	for _, want := range []string{"UUID", "email address", "username", "First Last"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not name the %q form", err, want)
		}
	}
}

// The not-found error itself has to carry the group carve-out: a model told to
// resolve "Data Stewards" learns from the error that groups are out of scope,
// not just from the tool description it may no longer be reading.
func TestGetUserIDByName_UnknownNameSaysGroupsAreNotResolved(t *testing.T) {
	client := usersServer(t, clients.EditAssetUser{ID: "u-1", UserName: "jane.smith", FirstName: "Jane", LastName: "Smith"})
	_, err := tool.NewTool(client).Handler(t.Context(), tool.Input{Name: "Data Stewards"})
	if err == nil {
		t.Fatal("expected a not-found error")
	}
	for _, want := range []string{"GROUP", "UUID", "search_asset_keyword"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
}
