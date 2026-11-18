package rename_user_group_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/outcome"
	tools "github.com/collibra/chip/pkg/tools/rename_user_group"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const groupID = "00000000-0000-0000-0000-000001000003"

// server is a DGC with one user group, Stewards, and another group named taken; it records every rename.
func server(t *testing.T, taken string, renames *[]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/userGroups/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != groupID {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"statusCode":404,"errorCode":"notFound","userMessage":"User group not found."}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"` + groupID + `","name":"Stewards","system":false}`))
	})
	mux.HandleFunc("GET /rest/2.0/userGroups", func(w http.ResponseWriter, r *http.Request) {
		if taken != "" && r.URL.Query().Get("name") == taken {
			_, _ = w.Write([]byte(`{"total":1,"offset":0,"limit":1,"results":[{"id":"other","name":"` + taken + `","system":false}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"total":0,"offset":0,"limit":1,"results":[]}`))
	})
	mux.HandleFunc("PATCH /rest/2.0/userGroups/{id}", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request map[string]string
		_ = json.Unmarshal(body, &request)
		*renames = append(*renames, r.PathValue("id")+"="+request["name"])
		_, _ = w.Write([]byte(`{"id":"` + groupID + `","name":"` + request["name"] + `","system":false}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRenameUserGroup_PreviewShowsOldAndNewNameAndWritesNothing(t *testing.T) {
	var renames []string
	output, _ := tools.NewTool(testutil.NewClient(server(t, "", &renames))).Handler(t.Context(), tools.Input{UserGroupID: groupID, Name: "Data Stewards"})
	if output.Status != outcome.ConfirmRequired || output.PreviousName != "Stewards" || output.Name != "Data Stewards" || len(renames) != 0 {
		t.Fatalf("expected a preview and no write, got %+v, renames=%v", output, renames)
	}
}

func TestRenameUserGroup_ConfirmRenames(t *testing.T) {
	var renames []string
	output, _ := tools.NewTool(testutil.NewClient(server(t, "", &renames))).Handler(t.Context(), tools.Input{UserGroupID: groupID, Name: "Data Stewards", Confirm: true})
	if output.Status != tools.StatusRenamed || output.Name != "Data Stewards" || len(renames) != 1 || renames[0] != groupID+"=Data Stewards" {
		t.Fatalf("unexpected output %+v, renames=%v", output, renames)
	}
}

func TestRenameUserGroup_RefusesWhatCannotBeDone(t *testing.T) {
	var renames []string
	tool := tools.NewTool(testutil.NewClient(server(t, "Taken", &renames)))
	for name, input := range map[string]tools.Input{
		"same name":       {UserGroupID: groupID, Name: "Stewards", Confirm: true},
		"name taken":      {UserGroupID: groupID, Name: "Taken", Confirm: true},
		"empty name":      {UserGroupID: groupID, Name: " ", Confirm: true},
		"malformed id":    {UserGroupID: "abc", Name: "X", Confirm: true},
		"id without dash": {UserGroupID: strings.ReplaceAll(groupID, "-", ""), Name: "X", Confirm: true},
	} {
		if output, _ := tool.Handler(t.Context(), input); output.Status != outcome.NeedsInput {
			t.Errorf("%s: expected needs_input, got %+v", name, output)
		}
	}
	if len(renames) != 0 {
		t.Fatalf("nothing may be written, got %v", renames)
	}
}

func TestRenameUserGroup_AnUnknownGroupIsAStatusWithTheNextStep(t *testing.T) {
	var renames []string
	output, _ := tools.NewTool(testutil.NewClient(server(t, "", &renames))).Handler(t.Context(), tools.Input{UserGroupID: "00000000-0000-0000-0000-0000000000ff", Name: "X", Confirm: true})
	if output.Status != outcome.Error || !strings.Contains(output.Message, "User group not found.") || !strings.Contains(output.Guidance, "does not know") {
		t.Fatalf("unexpected output: %+v", output)
	}
}
