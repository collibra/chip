package list_domains_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	tools "github.com/collibra/chip/pkg/tools/list_domains"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	communityID = "00000000-0000-0000-0000-000000000001"
	typeID      = "00000000-0000-0000-0000-000000030001"
)

func TestListDomains_SendsFiltersAndMapsTypeAndCommunity(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/2.0/domains", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		for key, want := range map[string]string{"name": "Cust", "communityId": communityID, "typeId": typeID, "includeSubCommunities": "true", "excludeMeta": "false", "limit": "20"} {
			if query.Get(key) != want {
				t.Errorf("expected %s=%s, got %q", key, want, query.Get(key))
			}
		}
		_, _ = w.Write([]byte(`{"total":1,"offset":0,"limit":20,"results":[
			{"id":"d1","name":"Customer Glossary","description":"Terms",
			 "type":{"id":"` + typeID + `","name":"Glossary","resourceType":"DomainType"},
			 "community":{"id":"` + communityID + `","name":"Finance","resourceType":"Community"}}]}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Name: "Cust", CommunityID: communityID, TypeID: typeID, IncludeSubCommunities: chip.Ptr(true), ExcludeMeta: chip.Ptr(false),
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	row := output.Results[0]
	if row.Type == nil || row.Type.Name != "Glossary" || row.Community == nil || row.Community.Name != "Finance" {
		t.Fatalf("unexpected row: %+v", row)
	}
}

func TestListDomains_RefusesAnUnfilteredCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no request may be sent for an unfiltered call")
	}))
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{Limit: chip.Ptr(3)})
	if err == nil || !strings.Contains(err.Error(), "at least one filter") {
		t.Fatalf("expected the filter requirement, got: %v", err)
	}
}

func TestListDomains_RejectsMalformedIds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("no request may be sent for a malformed id")
	}))
	defer server.Close()

	for name, input := range map[string]tools.Input{"communityId": {CommunityID: "x"}, "typeId": {TypeID: "x"}} {
		if _, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), input); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: expected an invalid UUID error naming the field, got: %v", name, err)
		}
	}
}
