package delete_user_group_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/delete_user_group"
	"github.com/collibra/chip/pkg/tools/outcome"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const groupID = "00000000-0000-0000-0000-000001000003"

// server is a DGC with one group that has 3 members and 2 role assignments; it records every delete.
func server(t *testing.T, system bool, deletes *[]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/userGroups/{id}", func(w http.ResponseWriter, r *http.Request) {
		if r.PathValue("id") != groupID {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"statusCode":404,"errorCode":"notFound","userMessage":"User group not found."}`))
			return
		}
		flag := "false"
		if system {
			flag = "true"
		}
		_, _ = w.Write([]byte(`{"id":"` + groupID + `","name":"Old Stewards","system":` + flag + `}`))
	})
	mux.HandleFunc("GET /rest/2.0/users", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("groupId") != groupID {
			t.Errorf("members must be counted for the group, got %v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"total":3,"offset":0,"limit":1,"results":[{"id":"u1","enabled":true}]}`))
	})
	mux.HandleFunc("GET /rest/2.0/responsibilities", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ownerIds") != groupID {
			t.Errorf("assignments must be counted for the group, got %v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"total":2,"offset":0,"limit":1,"results":[]}`))
	})
	mux.HandleFunc("DELETE /rest/2.0/userGroups/{id}", func(w http.ResponseWriter, r *http.Request) {
		*deletes = append(*deletes, r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestDeleteUserGroup_PreviewCountsWhatIsLostAndDeletesNothing(t *testing.T) {
	var deletes []string
	output, _ := tools.NewTool(testutil.NewClient(server(t, false, &deletes))).Handler(t.Context(), tools.Input{UserGroupID: groupID})
	if output.Status != outcome.ConfirmRequired || output.MemberCount != 3 || output.AssignmentCount != 2 || output.Name != "Old Stewards" || len(deletes) != 0 {
		t.Fatalf("expected a preview with the counts and no delete, got %+v, deletes=%v", output, deletes)
	}
}

func TestDeleteUserGroup_ConfirmDeletes(t *testing.T) {
	var deletes []string
	output, _ := tools.NewTool(testutil.NewClient(server(t, false, &deletes))).Handler(t.Context(), tools.Input{UserGroupID: groupID, Confirm: true})
	if output.Status != tools.StatusDeleted || len(deletes) != 1 || deletes[0] != groupID {
		t.Fatalf("unexpected output %+v, deletes=%v", output, deletes)
	}
}

func TestDeleteUserGroup_NeverDeletesASystemGroup(t *testing.T) {
	var deletes []string
	output, _ := tools.NewTool(testutil.NewClient(server(t, true, &deletes))).Handler(t.Context(), tools.Input{UserGroupID: groupID, Confirm: true})
	if output.Status != outcome.NeedsInput || !strings.Contains(output.Message, "system group") || len(deletes) != 0 {
		t.Fatalf("expected a refusal and no delete, got %+v, deletes=%v", output, deletes)
	}
}

func TestDeleteUserGroup_RejectsAMalformedIdAndReportsAnUnknownGroup(t *testing.T) {
	var deletes []string
	tool := tools.NewTool(testutil.NewClient(server(t, false, &deletes)))
	if output, _ := tool.Handler(t.Context(), tools.Input{UserGroupID: "nope", Confirm: true}); output.Status != outcome.NeedsInput {
		t.Errorf("expected needs_input for a malformed id, got %+v", output)
	}
	if output, _ := tool.Handler(t.Context(), tools.Input{UserGroupID: "00000000-0000-0000-0000-0000000000ff", Confirm: true}); output.Status != outcome.Error {
		t.Errorf("expected an error status for an unknown group, got %+v", output)
	}
	if len(deletes) != 0 {
		t.Fatalf("nothing may be deleted, got %v", deletes)
	}
}
