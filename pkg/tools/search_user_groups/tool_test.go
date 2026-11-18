package search_user_groups_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	tools "github.com/collibra/chip/pkg/tools/search_user_groups"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const userID = "00000000-0000-0000-0000-000000900002"

func TestSearchUserGroups_SendsFiltersUnderDGCsNamesAndReturnsTheRows(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/2.0/userGroups", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		for key, want := range map[string]string{"name": "Product", "nameMatchMode": "START", "userId": userID, "limit": "20"} {
			if query.Get(key) != want {
				t.Errorf("expected %s=%s, got %q", key, want, query.Get(key))
			}
		}
		_, _ = w.Write([]byte(`{"total":1,"offset":0,"limit":20,"results":[{"id":"g1","name":"Product Owners","system":false,"createdBy":"x"}]}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{Name: "Product", NameMatchMode: "START", UserID: userID})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.Total != 1 || output.Results[0].ID != "g1" || output.Results[0].Name != "Product Owners" {
		t.Fatalf("unexpected output: %+v", output)
	}
}

func TestSearchUserGroups_RefusesAnUnfilteredCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no request may be sent for an unfiltered call")
	}))
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{Limit: chip.Ptr(3)})
	if err == nil || !strings.Contains(err.Error(), "at least one filter") {
		t.Fatalf("expected the filter requirement, got: %v", err)
	}
}

func TestSearchUserGroups_RejectsAMalformedUserId(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no request may be sent for a malformed id")
	}))
	defer server.Close()

	if _, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{UserID: "abc"}); err == nil {
		t.Fatal("expected an invalid UUID error")
	}
}
