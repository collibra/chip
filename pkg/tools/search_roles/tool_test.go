package search_roles_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	tools "github.com/collibra/chip/pkg/tools/search_roles"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestSearchRoles_SendsFiltersUnderDGCsNamesAndLeavesPermissionsOut(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/2.0/roles", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		for key, want := range map[string]string{"name": "owner", "type": "RESOURCE", "roleInheritanceMode": "DIRECTLY", "includeDisabled": "false", "limit": "20"} {
			if query.Get(key) != want {
				t.Errorf("expected %s=%s, got %q", key, want, query.Get(key))
			}
		}
		_, _ = w.Write([]byte(`{"total":1,"offset":0,"limit":20,"results":[{"id":"r1","name":"Product Owner","description":"Owns the product","global":false,"disabled":false,"system":false,"permissionList":["A","B"]}]}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Name: "owner", Type: "RESOURCE", RoleInheritanceMode: "DIRECTLY", IncludeDisabled: chip.Ptr(false),
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(output.Results) != 1 || output.Results[0].Name != "Product Owner" || output.Results[0].Global {
		t.Fatalf("unexpected output: %+v", output)
	}
}

func TestSearchRoles_AcceptsAnUnfilteredCallBecauseRolesAreASmallSet(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/2.0/roles", func(w http.ResponseWriter, r *http.Request) {
		if len(r.URL.Query()) != 1 || r.URL.Query().Get("limit") != "20" {
			t.Errorf("expected only the default limit, got %v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"total":0,"offset":0,"limit":20,"results":[]}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	if _, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{}); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestSearchRoles_RejectsAMalformedOwnerId(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no request may be sent for a malformed id")
	}))
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{OwnerID: "nope"})
	if err == nil || !strings.Contains(err.Error(), "ownerId") {
		t.Fatalf("expected an invalid UUID error naming ownerId, got: %v", err)
	}
}
