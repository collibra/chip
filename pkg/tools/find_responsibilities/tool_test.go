package find_responsibilities_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	tools "github.com/collibra/chip/pkg/tools/find_responsibilities"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	roleID     = "00000000-0000-0000-0000-000000005040"
	ownerA     = "00000000-0000-0000-0000-000000900002"
	ownerB     = "00000000-0000-0000-0000-000001000003"
	resourceID = "00000000-0000-0000-0000-000000000001"
)

func TestFindResponsibilities_RepeatsListParametersAndMapsTheRows(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/2.0/responsibilities", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if got := query["ownerIds"]; len(got) != 2 {
			t.Errorf("ownerIds must repeat once per id, got %v", got)
		}
		if query.Get("roleIds") != roleID || query.Get("resourceIds") != resourceID || query.Get("includeInherited") != "false" || query.Get("excludeEmptyGroups") != "true" {
			t.Errorf("unexpected query: %v", query)
		}
		_, _ = w.Write([]byte(`{"total":2,"offset":0,"limit":20,"results":[
			{"id":"a1","role":{"id":"` + roleID + `","name":"Product Owner","resourceType":"Role"},
			 "owner":{"id":"` + ownerA + `","resourceType":"User","resourceDiscriminator":"User"},
			 "baseResource":{"id":"` + resourceID + `","resourceType":"Domain","resourceDiscriminator":"Domain"}},
			{"id":"a2","role":{"id":"` + roleID + `","name":"Product Owner"},
			 "owner":{"id":"` + ownerB + `","resourceDiscriminator":"UserGroup"}}]}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		RoleIDs: []string{roleID}, OwnerIDs: []string{ownerA, ownerB}, ResourceIDs: []string{resourceID},
		IncludeInherited: chip.Ptr(false), ExcludeEmptyGroups: chip.Ptr(true),
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.Total != 2 || len(output.Results) != 2 {
		t.Fatalf("unexpected output: %+v", output)
	}
	first, second := output.Results[0], output.Results[1]
	if first.Role.Name != "Product Owner" || first.Owner.Type != "User" || first.Resource == nil || first.Resource.Type != "Domain" {
		t.Errorf("unexpected first row: %+v", first)
	}
	if second.Owner.Type != "UserGroup" || second.Resource != nil {
		t.Errorf("a global assignment has no resource and a group owner reads as UserGroup: %+v", second)
	}
}

func TestFindResponsibilities_RefusesAnUnfilteredCallButTakesGlobalOnly(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/2.0/responsibilities", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("globalOnly") != "true" {
			t.Errorf("expected globalOnly=true, got %v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"total":0,"offset":0,"limit":20,"results":[]}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	tool := tools.NewTool(testutil.NewClient(server))

	if _, err := tool.Handler(t.Context(), tools.Input{}); err == nil || !strings.Contains(err.Error(), "at least one filter") {
		t.Fatalf("expected the filter requirement, got: %v", err)
	}
	if _, err := tool.Handler(t.Context(), tools.Input{GlobalOnly: chip.Ptr(true)}); err != nil {
		t.Fatalf("globalOnly is a filter, got: %v", err)
	}
}

func TestFindResponsibilities_RejectsMalformedIds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no request may be sent for a malformed id")
	}))
	defer server.Close()

	for name, input := range map[string]tools.Input{
		"roleIds":     {RoleIDs: []string{"x"}},
		"ownerIds":    {OwnerIDs: []string{ownerA, "x"}},
		"resourceIds": {ResourceIDs: []string{"x"}},
	} {
		if _, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), input); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: expected an invalid UUID error naming the field, got: %v", name, err)
		}
	}
}
