package create_user_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/create_user"
	"github.com/collibra/chip/pkg/tools/outcome"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const groupID = "00000000-0000-0000-0000-000001000003"

// server is a DGC with one group and one user, jsmith; it records every create.
func server(t *testing.T, creates *[]map[string]any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/userGroups/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != groupID {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"statusCode":404,"errorCode":"notFound","userMessage":"User group not found."}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"` + groupID + `","name":"Data Stewards","system":false}`))
	})
	mux.HandleFunc("GET /rest/2.0/users", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains("jsmith", strings.ToLower(r.URL.Query().Get("name"))) {
			_, _ = w.Write([]byte(`{"total":1,"offset":0,"limit":50,"results":[{"id":"u-existing","userName":"jsmith","enabled":true}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"total":0,"offset":0,"limit":50,"results":[]}`))
	})
	mux.HandleFunc("POST /rest/2.0/users", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request map[string]any
		_ = json.Unmarshal(body, &request)
		*creates = append(*creates, request)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"u-new","userName":"jane.smith","firstName":"Jane","lastName":"Smith","enabled":true}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func input() create_user.Input {
	return create_user.Input{
		UserName: "jane.smith", EmailAddress: "jane.smith@example.com", FirstName: "Jane", LastName: "Smith",
		Title: "Steward", Department: "Finance", Language: "en", UserGroupIDs: []string{groupID},
	}
}

func TestCreateUser_PreviewEchoesEveryPropertyAndWritesNothing(t *testing.T) {
	var creates []map[string]any
	output, _ := create_user.NewTool(testutil.NewClient(server(t, &creates))).Handler(t.Context(), input())
	if output.Status != outcome.ConfirmRequired || output.UserName != "jane.smith" || output.EmailAddress != "jane.smith@example.com" ||
		output.FirstName != "Jane" || output.LastName != "Smith" || output.Title != "Steward" || output.Department != "Finance" ||
		output.Language != "en" || len(output.UserGroups) != 1 || output.UserGroups[0] != "Data Stewards" {
		t.Fatalf("expected every property in the preview, got %+v", output)
	}
	if len(creates) != 0 {
		t.Fatalf("a preview writes nothing, got %v", creates)
	}
}

func TestCreateUser_ConfirmSendsEveryPropertyUnderDGCsNamesAndNoPassword(t *testing.T) {
	var creates []map[string]any
	in := input()
	in.Confirm = true
	output, _ := create_user.NewTool(testutil.NewClient(server(t, &creates))).Handler(t.Context(), in)
	if output.Status != create_user.StatusCreated || output.ID != "u-new" || len(creates) != 1 {
		t.Fatalf("unexpected output %+v, creates=%v", output, creates)
	}
	sent := creates[0]
	for key, want := range map[string]string{"userName": "jane.smith", "emailAddress": "jane.smith@example.com", "firstName": "Jane", "lastName": "Smith", "title": "Steward", "department": "Finance", "language": "en"} {
		if sent[key] != want {
			t.Errorf("expected %s=%s, got %v", key, want, sent[key])
		}
	}
	if groups, _ := sent["userGroupIds"].([]any); len(groups) != 1 || groups[0] != groupID {
		t.Errorf("expected the group id, got %v", sent["userGroupIds"])
	}
	if _, hasPassword := sent["password"]; hasPassword {
		t.Error("the tool must never send a password")
	}
}

func TestCreateUser_RefusesWhatCannotBeDone(t *testing.T) {
	var creates []map[string]any
	tool := create_user.NewTool(testutil.NewClient(server(t, &creates)))
	taken := input()
	taken.UserName, taken.Confirm = "JSmith", true
	noEmail := input()
	noEmail.EmailAddress, noEmail.Confirm = "not-an-email", true
	noName := input()
	noName.UserName, noName.Confirm = " ", true
	badGroup := input()
	badGroup.UserGroupIDs, badGroup.Confirm = []string{"x"}, true
	for name, in := range map[string]create_user.Input{"taken user name": taken, "no email": noEmail, "no user name": noName, "bad group id": badGroup} {
		if output, _ := tool.Handler(t.Context(), in); output.Status != outcome.NeedsInput {
			t.Errorf("%s: expected needs_input, got %+v", name, output)
		}
	}
	unknownGroup := input()
	unknownGroup.UserGroupIDs, unknownGroup.Confirm = []string{"00000000-0000-0000-0000-0000000000ee"}, true
	if output, _ := tool.Handler(t.Context(), unknownGroup); output.Status != outcome.Error {
		t.Errorf("an unknown group must be an error status, got %+v", output)
	}
	if len(creates) != 0 {
		t.Fatalf("nothing may be written, got %v", creates)
	}
}
