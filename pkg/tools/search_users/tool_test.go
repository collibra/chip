package search_users_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	tools "github.com/collibra/chip/pkg/tools/search_users"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	groupID = "00000000-0000-0000-0000-000001000003"
	userA   = "00000000-0000-0000-0000-000000900002"
	userB   = "00000000-0000-0000-0000-000000900003"
)

func TestSearchUsers_SendsEveryFilterUnderDGCsNamesAndNeverReturnsEmail(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/2.0/users", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if got := query["userId"]; len(got) != 2 || got[0] != userA || got[1] != userB {
			t.Errorf("userId must repeat once per id, got %v", got)
		}
		for key, want := range map[string]string{"name": "jane", "groupId": groupID, "title": "Owner", "department": "Finance", "includeDisabled": "true", "offset": "5", "limit": "7", "countLimit": "-1"} {
			if query.Get(key) != want {
				t.Errorf("expected %s=%s, got %q", key, want, query.Get(key))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":1,"offset":5,"limit":7,"results":[{"id":"` + userA + `","userName":"jsmith","firstName":"Jane","lastName":"Smith","emailAddress":"jane@example.com","enabled":true}]}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Name: "jane", GroupID: groupID, UserIDs: []string{userA, userB}, Title: "Owner", Department: "Finance",
		IncludeDisabled: chip.Ptr(true), Offset: chip.Ptr(5), Limit: chip.Ptr(7), CountLimit: chip.Ptr(-1),
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.Total != 1 || len(output.Results) != 1 || output.Results[0].UserName != "jsmith" {
		t.Fatalf("unexpected output: %+v", output)
	}
	encoded, _ := json.Marshal(output)
	if strings.Contains(string(encoded), "jane@example.com") || strings.Contains(string(encoded), "emailAddress") {
		t.Fatalf("an email address must never be returned: %s", encoded)
	}
}

func TestSearchUsers_SendsOnlyWhatIsSetAndADefaultLimitOf20(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/2.0/users", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if len(query) != 2 || query.Get("name") != "jane" || query.Get("limit") != "20" {
			t.Errorf("expected only name and the default limit, got %v", query)
		}
		_, _ = w.Write([]byte(`{"total":0,"offset":0,"limit":20,"results":[]}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	if _, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{Name: "jane"}); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestSearchUsers_RefusesAnUnfilteredCallWithoutSendingARequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no request may be sent for an unfiltered call")
	}))
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{Limit: chip.Ptr(5)})
	if err == nil || !strings.Contains(err.Error(), "at least one filter") {
		t.Fatalf("expected the filter requirement, got: %v", err)
	}
}

func TestSearchUsers_RejectsAnIdDGCWouldRefuseWithoutSendingARequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no request may be sent for a malformed id")
	}))
	defer server.Close()

	for name, input := range map[string]tools.Input{
		"groupId": {GroupID: "00000000000000000000000001000003"},
		"userId":  {UserIDs: []string{userA, "not-a-uuid"}},
	} {
		if _, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), input); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: expected an invalid UUID error naming the field, got: %v", name, err)
		}
	}
}

func TestSearchUsers_PassesDGCsErrorThrough(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/2.0/users", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"statusCode":403,"errorCode":"permissionDenied","userMessage":"You are not allowed to see users."}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{Name: "jane"})
	if err == nil || !strings.Contains(err.Error(), "You are not allowed to see users.") {
		t.Fatalf("expected DGC's message, got: %v", err)
	}
}
