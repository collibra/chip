package list_communities_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	tools "github.com/collibra/chip/pkg/tools/list_communities"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const parentID = "00000000-0000-0000-0000-000000000001"

func TestListCommunities_SendsFiltersAndMapsTheParent(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/2.0/communities", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		for key, want := range map[string]string{"name": "Fin", "nameMatchMode": "START", "parentId": parentID, "excludeMeta": "false", "limit": "20"} {
			if query.Get(key) != want {
				t.Errorf("expected %s=%s, got %q", key, want, query.Get(key))
			}
		}
		_, _ = w.Write([]byte(`{"total":2,"offset":0,"limit":20,"results":[
			{"id":"c1","name":"Finance","description":"Money","parent":{"id":"` + parentID + `","name":"Company","resourceType":"Community"}},
			{"id":"c2","name":"Sales"}]}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{Name: "Fin", NameMatchMode: "START", ParentID: parentID, ExcludeMeta: chip.Ptr(false)})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.Total != 2 || output.Results[0].Parent == nil || output.Results[0].Parent.Name != "Company" || output.Results[1].Parent != nil {
		t.Fatalf("unexpected output: %+v", output)
	}
}

func TestListCommunities_TakesAnUnfilteredCallAndPagesIt(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/2.0/communities", func(w http.ResponseWriter, r *http.Request) {
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

func TestListCommunities_RejectsAMalformedParentId(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no request may be sent for a malformed id")
	}))
	defer server.Close()

	if _, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{ParentID: "x"}); err == nil || !strings.Contains(err.Error(), "parentId") {
		t.Fatalf("expected an invalid UUID error naming parentId, got: %v", err)
	}
}
