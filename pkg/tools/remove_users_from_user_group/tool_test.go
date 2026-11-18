package remove_users_from_user_group_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/outcome"
	"github.com/collibra/chip/pkg/tools/remove_users_from_user_group"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	groupID = "00000000-0000-0000-0000-000001000003"
	jane    = "00000000-0000-0000-0000-0000000000a1" // exists, not a member
	john    = "00000000-0000-0000-0000-0000000000a2" // exists, a member
	ghost   = "00000000-0000-0000-0000-0000000000ff" // does not exist
)

var users = map[string]string{
	jane: `{"id":"` + jane + `","userName":"jane","enabled":true}`,
	john: `{"id":"` + john + `","userName":"john","enabled":true}`,
}

// server is a DGC with a group that john is in; it records every removal, which DGC takes as a DELETE with a body.
func server(t *testing.T, removals *[][]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/userGroups/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"` + r.PathValue("id") + `","name":"Data Stewards","system":false}`))
	})
	mux.HandleFunc("GET /rest/2.0/users", func(w http.ResponseWriter, r *http.Request) {
		var rows []string
		for _, id := range r.URL.Query()["userId"] {
			row, known := users[id]
			if !known || (r.URL.Query().Get("groupId") != "" && id != john) {
				continue
			}
			rows = append(rows, row)
		}
		_, _ = fmt.Fprintf(w, `{"total":%d,"offset":0,"limit":50,"results":[%s]}`, len(rows), strings.Join(rows, ","))
	})
	mux.HandleFunc("DELETE /rest/2.0/userGroups/{id}/users", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			UserIDs []string `json:"userIds"`
		}
		_ = json.Unmarshal(body, &request)
		*removals = append(*removals, request.UserIDs)
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestRemoveUsersFromUserGroup_PreviewListsWhoWouldBeRemovedAndWritesNothing(t *testing.T) {
	var removals [][]string
	output, _ := remove_users_from_user_group.NewTool(testutil.NewClient(server(t, &removals))).Handler(t.Context(),
		remove_users_from_user_group.Input{UserGroupID: groupID, UserIDs: []string{john, jane, ghost}})
	if output.Status != outcome.ConfirmRequired || len(output.Users) != 1 || output.Users[0].ID != john ||
		len(output.NotMembers) != 1 || output.NotMembers[0].ID != jane || len(output.UnknownUserIDs) != 1 {
		t.Fatalf("expected john to remove, jane not a member, ghost unknown: %+v", output)
	}
	if len(removals) != 0 {
		t.Fatalf("a preview writes nothing, got %v", removals)
	}
}

func TestRemoveUsersFromUserGroup_ConfirmSendsOnlyTheMembers(t *testing.T) {
	var removals [][]string
	output, _ := remove_users_from_user_group.NewTool(testutil.NewClient(server(t, &removals))).Handler(t.Context(),
		remove_users_from_user_group.Input{UserGroupID: groupID, UserIDs: []string{john, jane}, Confirm: true})
	if output.Status != remove_users_from_user_group.StatusRemoved {
		t.Fatalf("unexpected output: %+v", output)
	}
	if len(removals) != 1 || len(removals[0]) != 1 || removals[0][0] != john {
		t.Fatalf("only john may be sent, got %v", removals)
	}
}

func TestRemoveUsersFromUserGroup_NobodyToRemoveIsNeedsInput(t *testing.T) {
	var removals [][]string
	output, _ := remove_users_from_user_group.NewTool(testutil.NewClient(server(t, &removals))).Handler(t.Context(),
		remove_users_from_user_group.Input{UserGroupID: groupID, UserIDs: []string{jane}, Confirm: true})
	if output.Status != outcome.NeedsInput || len(removals) != 0 {
		t.Fatalf("expected needs_input and no write, got %+v, removals=%v", output, removals)
	}
}

func TestRemoveUsersFromUserGroup_RejectsBadInputWithoutAnyRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no request may be sent for bad input")
	}))
	defer srv.Close()
	tool := remove_users_from_user_group.NewTool(testutil.NewClient(srv))

	for name, input := range map[string]remove_users_from_user_group.Input{
		"no users":     {UserGroupID: groupID},
		"bad group id": {UserGroupID: "x", UserIDs: []string{john}},
		"bad user id":  {UserGroupID: groupID, UserIDs: []string{"x"}},
	} {
		if output, _ := tool.Handler(t.Context(), input); output.Status != outcome.NeedsInput {
			t.Errorf("%s: expected needs_input, got %+v", name, output)
		}
	}
}
