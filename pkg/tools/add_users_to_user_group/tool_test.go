package add_users_to_user_group_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/add_users_to_user_group"
	"github.com/collibra/chip/pkg/tools/outcome"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	groupID = "00000000-0000-0000-0000-000001000003"
	jane    = "00000000-0000-0000-0000-0000000000a1" // exists, not a member
	john    = "00000000-0000-0000-0000-0000000000a2" // exists, already a member
	ghost   = "00000000-0000-0000-0000-0000000000ff" // does not exist
)

var users = map[string]string{
	jane: `{"id":"` + jane + `","userName":"jane","firstName":"Jane","enabled":true}`,
	john: `{"id":"` + john + `","userName":"john","firstName":"John","enabled":true}`,
}

// server is a DGC with a group that john is in, and two users: jane and john. It records every add.
func server(t *testing.T, adds *[][]string) *httptest.Server {
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
	mux.HandleFunc("POST /rest/2.0/userGroups/{id}/users", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request struct {
			UserIDs []string `json:"userIds"`
		}
		_ = json.Unmarshal(body, &request)
		*adds = append(*adds, request.UserIDs)
		_, _ = w.Write([]byte(`[` + users[jane] + `]`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestAddUsersToUserGroup_PreviewListsWhoWouldBeAddedAndWritesNothing(t *testing.T) {
	var adds [][]string
	output, _ := add_users_to_user_group.NewTool(testutil.NewClient(server(t, &adds))).Handler(t.Context(),
		add_users_to_user_group.Input{UserGroupID: groupID, UserIDs: []string{jane, john, ghost, jane}})
	if output.Status != outcome.ConfirmRequired || output.UserGroupName != "Data Stewards" {
		t.Fatalf("expected a preview, got %+v", output)
	}
	if len(output.Users) != 1 || output.Users[0].ID != jane || len(output.AlreadyMembers) != 1 || output.AlreadyMembers[0].ID != john ||
		len(output.UnknownUserIDs) != 1 || output.UnknownUserIDs[0] != ghost {
		t.Fatalf("expected jane to add, john already in, ghost unknown (jane named twice counts once): %+v", output)
	}
	if len(adds) != 0 {
		t.Fatalf("a preview writes nothing, got %v", adds)
	}
}

func TestAddUsersToUserGroup_ConfirmSendsOnlyTheUsersWhoAreNotMembersYet(t *testing.T) {
	var adds [][]string
	output, _ := add_users_to_user_group.NewTool(testutil.NewClient(server(t, &adds))).Handler(t.Context(),
		add_users_to_user_group.Input{UserGroupID: groupID, UserIDs: []string{jane, john, ghost}, Confirm: true})
	if output.Status != add_users_to_user_group.StatusAdded || len(output.Users) != 1 {
		t.Fatalf("unexpected output: %+v", output)
	}
	if len(adds) != 1 || len(adds[0]) != 1 || adds[0][0] != jane {
		t.Fatalf("only jane may be sent, got %v", adds)
	}
}

func TestAddUsersToUserGroup_NobodyToAddIsNeedsInputAndWritesNothing(t *testing.T) {
	var adds [][]string
	output, _ := add_users_to_user_group.NewTool(testutil.NewClient(server(t, &adds))).Handler(t.Context(),
		add_users_to_user_group.Input{UserGroupID: groupID, UserIDs: []string{john, ghost}, Confirm: true})
	if output.Status != outcome.NeedsInput || len(adds) != 0 {
		t.Fatalf("expected needs_input and no write, got %+v, adds=%v", output, adds)
	}
}

func TestAddUsersToUserGroup_LooksUpManyIdsInBatches(t *testing.T) {
	var adds [][]string
	ids := make([]string, 0, 120)
	for i := 0; i < 120; i++ {
		ids = append(ids, fmt.Sprintf("00000000-0000-0000-0000-%012d", 1000+i))
	}
	mux := http.NewServeMux()
	requests := 0
	mux.HandleFunc("GET /rest/2.0/userGroups/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"` + groupID + `","name":"G","system":false}`))
	})
	mux.HandleFunc("GET /rest/2.0/users", func(w http.ResponseWriter, r *http.Request) {
		requests++
		if n := len(r.URL.Query()["userId"]); n > 50 {
			t.Errorf("at most 50 ids per request, got %d", n)
		}
		_, _ = w.Write([]byte(`{"total":0,"offset":0,"limit":50,"results":[]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	output, _ := add_users_to_user_group.NewTool(testutil.NewClient(srv)).Handler(t.Context(), add_users_to_user_group.Input{UserGroupID: groupID, UserIDs: ids})
	if len(output.UnknownUserIDs) != 120 || requests != 6 || len(adds) != 0 {
		t.Fatalf("expected 120 unknown over 3 batches x 2 lookups, got unknown=%d requests=%d", len(output.UnknownUserIDs), requests)
	}
}

func TestAddUsersToUserGroup_RejectsBadInputWithoutAnyRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no request may be sent for bad input")
	}))
	defer srv.Close()
	tool := add_users_to_user_group.NewTool(testutil.NewClient(srv))

	for name, input := range map[string]add_users_to_user_group.Input{
		"no users":       {UserGroupID: groupID},
		"bad group id":   {UserGroupID: "x", UserIDs: []string{jane}},
		"bad user id":    {UserGroupID: groupID, UserIDs: []string{jane, "x"}},
		"undashed group": {UserGroupID: strings.ReplaceAll(groupID, "-", ""), UserIDs: []string{jane}},
	} {
		if output, _ := tool.Handler(t.Context(), input); output.Status != outcome.NeedsInput {
			t.Errorf("%s: expected needs_input, got %+v", name, output)
		}
	}
}

func TestAddUsersToUserGroup_AnUnknownGroupIsAStatus(t *testing.T) {
	var adds [][]string
	output, _ := add_users_to_user_group.NewTool(testutil.NewClient(server(t, &adds))).Handler(t.Context(),
		add_users_to_user_group.Input{UserGroupID: "00000000-0000-0000-0000-0000000000ee", UserIDs: []string{jane}, Confirm: true})
	if output.Status != outcome.Error || len(adds) != 0 {
		t.Fatalf("expected an error status and no write, got %+v", output)
	}
}
