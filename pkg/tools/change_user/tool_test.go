package change_user_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/tools/change_user"
	"github.com/collibra/chip/pkg/tools/outcome"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const userID = "00000000-0000-0000-0000-0000000000a1"

// server is a DGC with one enabled user, jane; it records every change.
func server(t *testing.T, changes *[]map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/users", func(w http.ResponseWriter, r *http.Request) {
		if ids := r.URL.Query()["userId"]; len(ids) == 1 && ids[0] == userID {
			_, _ = w.Write([]byte(`{"total":1,"offset":0,"limit":1,"results":[{"id":"` + userID + `","userName":"jane","firstName":"Jane","lastName":"Smith","title":"Steward","department":"Finance","enabled":true}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"total":0,"offset":0,"limit":1,"results":[]}`))
	})
	mux.HandleFunc("PATCH /rest/2.0/users/{id}", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request map[string]any
		_ = json.Unmarshal(body, &request)
		*changes = append(*changes, request)
		_, _ = w.Write([]byte(`{"id":"` + userID + `","userName":"jane","firstName":"Jane","lastName":"Smith","enabled":false}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestChangeUser_PreviewShowsEachChangeWithItsCurrentValueAndWritesNothing(t *testing.T) {
	var changes []map[string]any
	output, _ := change_user.NewTool(testutil.NewClient(server(t, &changes))).Handler(t.Context(), change_user.Input{
		UserID: userID, Title: "Head of Data", Department: "Finance", Enabled: chip.Ptr(false), EmailAddress: "new@example.com",
	})
	if output.Status != outcome.ConfirmRequired {
		t.Fatalf("expected a preview, got %+v", output)
	}
	byField := map[string]change_user.Change{}
	for _, change := range output.Changes {
		byField[change.Field] = change
	}
	if len(byField) != 3 || byField["title"].From != "Steward" || byField["title"].To != "Head of Data" ||
		byField["enabled"].From != "true" || byField["enabled"].To != "false" || byField["emailAddress"].To != "new@example.com" || byField["emailAddress"].From != "" {
		t.Fatalf("expected title, enabled and emailAddress (department is unchanged); the email has no current value: %+v", output.Changes)
	}
	if len(changes) != 0 {
		t.Fatalf("a preview writes nothing, got %v", changes)
	}
}

func TestChangeUser_ConfirmSendsOnlyWhatChangesUnderDGCsNames(t *testing.T) {
	var changes []map[string]any
	output, _ := change_user.NewTool(testutil.NewClient(server(t, &changes))).Handler(t.Context(), change_user.Input{
		UserID: userID, UserName: "jsmith", EmailAddress: "new@example.com", Title: "Head of Data", Department: "Finance", Enabled: chip.Ptr(false), Confirm: true,
	})
	if output.Status != change_user.StatusChanged || output.Enabled {
		t.Fatalf("unexpected output: %+v", output)
	}
	if len(changes) != 1 {
		t.Fatalf("expected one change, got %v", changes)
	}
	sent := changes[0]
	if sent["username"] != "jsmith" || sent["email"] != "new@example.com" || sent["title"] != "Head of Data" || sent["enabled"] != false {
		t.Errorf("unexpected body: %v", sent)
	}
	if _, resent := sent["department"]; resent {
		t.Error("the department did not change and must not be sent")
	}
	if _, wrongName := sent["userName"]; wrongName {
		t.Error("PATCH takes username, not userName")
	}
}

func TestChangeUser_RefusesWhatCannotBeDone(t *testing.T) {
	var changes []map[string]any
	tool := change_user.NewTool(testutil.NewClient(server(t, &changes)))
	for name, in := range map[string]change_user.Input{
		"nothing to change": {UserID: userID, Confirm: true},
		"same values":       {UserID: userID, Title: "Steward", Enabled: chip.Ptr(true), Confirm: true},
		"unknown user":      {UserID: "00000000-0000-0000-0000-0000000000ff", Title: "X", Confirm: true},
		"malformed user id": {UserID: "x", Title: "X", Confirm: true},
	} {
		if output, _ := tool.Handler(t.Context(), in); output.Status != outcome.NeedsInput {
			t.Errorf("%s: expected needs_input, got %+v", name, output)
		}
	}
	if len(changes) != 0 {
		t.Fatalf("nothing may be written, got %v", changes)
	}
}
