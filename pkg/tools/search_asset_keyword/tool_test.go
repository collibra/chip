package search_asset_keyword_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/search_asset_keyword"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

// idName is the minimal {id, name} result shape shared by the statuses,
// domainTypes, assetTypes and communities list endpoints.
type idName struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type listResp struct {
	Results []idName `json:"results"`
	Total   int      `json:"total"`
}

// captureFilters registers a /rest/2.0/search handler that records the filters
// it receives, so a test can assert which UUIDs resolution produced.
func captureFilters(mux *http.ServeMux, got *[]clients.SearchFilter) {
	mux.Handle("/rest/2.0/search", testutil.JsonHandlerInOut(func(_ *http.Request, in clients.SearchRequest) (int, clients.SearchResponse) {
		*got = in.Filters
		return http.StatusOK, clients.SearchResponse{Total: 0, Results: []clients.SearchResult{}}
	}))
}

func filterValues(filters []clients.SearchFilter, field string) []string {
	for _, f := range filters {
		if f.Field == field {
			return f.Values
		}
	}
	return nil
}

func TestStatusFilterResolvesNameToUUID(t *testing.T) {
	obsoleteID := uuid.New().String()
	mux := http.NewServeMux()
	var got []clients.SearchFilter
	captureFilters(mux, &got)
	mux.Handle("/rest/2.0/statuses", testutil.JsonHandlerOut(func(*http.Request) (int, listResp) {
		return http.StatusOK, listResp{Total: 2, Results: []idName{
			{ID: uuid.New().String(), Name: "Candidate"},
			{ID: obsoleteID, Name: "Obsolete"},
		}}
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Query:        "revenue",
		StatusFilter: []string{"Obsolete"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vals := filterValues(got, "status"); len(vals) != 1 || vals[0] != obsoleteID {
		t.Fatalf("expected status filter [%s], got %v", obsoleteID, vals)
	}
}

func TestStatusFilterCaseInsensitive(t *testing.T) {
	obsoleteID := uuid.New().String()
	mux := http.NewServeMux()
	var got []clients.SearchFilter
	captureFilters(mux, &got)
	mux.Handle("/rest/2.0/statuses", testutil.JsonHandlerOut(func(*http.Request) (int, listResp) {
		return http.StatusOK, listResp{Total: 1, Results: []idName{{ID: obsoleteID, Name: "Obsolete"}}}
	}))
	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Query:        "revenue",
		StatusFilter: []string{"  oBsOlEtE "},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vals := filterValues(got, "status"); len(vals) != 1 || vals[0] != obsoleteID {
		t.Fatalf("expected status filter [%s], got %v", obsoleteID, vals)
	}
}

func TestStatusFilterUUIDPassthrough(t *testing.T) {
	statusID := uuid.New().String()
	mux := http.NewServeMux()
	var got []clients.SearchFilter
	captureFilters(mux, &got)
	// No /statuses handler registered: a UUID must not trigger a lookup.
	mux.Handle("/rest/2.0/statuses", testutil.JsonHandlerOut(func(*http.Request) (int, listResp) {
		t.Fatalf("statuses endpoint should not be called for a UUID value")
		return http.StatusInternalServerError, listResp{}
	}))
	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Query:        "revenue",
		StatusFilter: []string{statusID},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vals := filterValues(got, "status"); len(vals) != 1 || vals[0] != statusID {
		t.Fatalf("expected status filter [%s], got %v", statusID, vals)
	}
}

func TestStatusFilterUnknownNameErrorsWithSuggestions(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/2.0/statuses", testutil.JsonHandlerOut(func(*http.Request) (int, listResp) {
		return http.StatusOK, listResp{Total: 2, Results: []idName{
			{ID: uuid.New().String(), Name: "Candidate"},
			{ID: uuid.New().String(), Name: "Obsolete"},
		}}
	}))
	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Query:        "revenue",
		StatusFilter: []string{"Deprecated"},
	})
	if err == nil {
		t.Fatal("expected error for unknown status name")
	}
	if !strings.Contains(err.Error(), "Candidate") || !strings.Contains(err.Error(), "Obsolete") {
		t.Fatalf("expected suggestions in error, got: %v", err)
	}
}

func TestAssetTypeFilterResolvesNameToUUID(t *testing.T) {
	tableID := uuid.New().String()
	mux := http.NewServeMux()
	var got []clients.SearchFilter
	captureFilters(mux, &got)
	mux.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(*http.Request) (int, listResp) {
		return http.StatusOK, listResp{Total: 1, Results: []idName{{ID: tableID, Name: "Table"}}}
	}))
	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Query:           "revenue",
		AssetTypeFilter: []string{"Table"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vals := filterValues(got, "assetType"); len(vals) != 1 || vals[0] != tableID {
		t.Fatalf("expected assetType filter [%s], got %v", tableID, vals)
	}
}

func TestDomainTypeFilterResolvesNameToUUID(t *testing.T) {
	glossaryID := uuid.New().String()
	mux := http.NewServeMux()
	var got []clients.SearchFilter
	captureFilters(mux, &got)
	mux.Handle("/rest/2.0/domainTypes", testutil.JsonHandlerOut(func(*http.Request) (int, listResp) {
		return http.StatusOK, listResp{Total: 1, Results: []idName{{ID: glossaryID, Name: "Glossary"}}}
	}))
	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Query:            "revenue",
		DomainTypeFilter: []string{"Glossary"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vals := filterValues(got, "domainType"); len(vals) != 1 || vals[0] != glossaryID {
		t.Fatalf("expected domainType filter [%s], got %v", glossaryID, vals)
	}
}

func TestCommunityFilterResolvesNameToUUID(t *testing.T) {
	commID := uuid.New().String()
	mux := http.NewServeMux()
	var got []clients.SearchFilter
	captureFilters(mux, &got)
	mux.Handle("/rest/2.0/communities", testutil.JsonHandlerOut(func(*http.Request) (int, listResp) {
		return http.StatusOK, listResp{Total: 1, Results: []idName{{ID: commID, Name: "Marketing"}}}
	}))
	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Query:           "revenue",
		CommunityFilter: []string{"Marketing"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vals := filterValues(got, "community"); len(vals) != 1 || vals[0] != commID {
		t.Fatalf("expected community filter [%s], got %v", commID, vals)
	}
}

func TestDomainFilterAmbiguousNameErrors(t *testing.T) {
	mux := http.NewServeMux()
	type domainType struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	type domainRec struct {
		ID   string      `json:"id"`
		Name string      `json:"name"`
		Type *domainType `json:"type,omitempty"`
	}
	type domainResp struct {
		Results []domainRec `json:"results"`
		Total   int         `json:"total"`
	}
	mux.Handle("/rest/2.0/domains", testutil.JsonHandlerOut(func(*http.Request) (int, domainResp) {
		return http.StatusOK, domainResp{Total: 2, Results: []domainRec{
			{ID: uuid.New().String(), Name: "Customers", Type: &domainType{Name: "Glossary"}},
			{ID: uuid.New().String(), Name: "Customers", Type: &domainType{Name: "Data Asset Domain"}},
		}}
	}))
	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Query:        "revenue",
		DomainFilter: []string{"Customers"},
	})
	if err == nil {
		t.Fatal("expected ambiguity error for duplicate domain name")
	}
	if !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "Glossary") {
		t.Fatalf("expected ambiguity error with disambiguating context, got: %v", err)
	}
}

func TestCreatedByFilterResolvesUsernameToUUID(t *testing.T) {
	userID := uuid.New().String()
	mux := http.NewServeMux()
	var got []clients.SearchFilter
	captureFilters(mux, &got)
	type userRec struct {
		ID       string `json:"id"`
		UserName string `json:"userName"`
	}
	type userResp struct {
		Results []userRec `json:"results"`
		Total   int       `json:"total"`
	}
	mux.Handle("/rest/2.0/users", testutil.JsonHandlerOut(func(*http.Request) (int, userResp) {
		return http.StatusOK, userResp{Total: 1, Results: []userRec{{ID: userID, UserName: "jsmith"}}}
	}))
	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Query:           "revenue",
		CreatedByFilter: []string{"jsmith"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if vals := filterValues(got, "createdBy"); len(vals) != 1 || vals[0] != userID {
		t.Fatalf("expected createdBy filter [%s], got %v", userID, vals)
	}
}

func TestKeywordSearch(t *testing.T) {
	assetId, _ := uuid.NewUUID()
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/search", testutil.JsonHandlerInOut(func(httpRequest *http.Request, request clients.SearchRequest) (int, clients.SearchResponse) {
		return http.StatusOK, clients.SearchResponse{
			Total: 1,
			Results: []clients.SearchResult{
				{
					Resource: clients.SearchResource{
						ResourceType: "Asset",
						ID:           assetId.String(),
						Name:         "My Asset Name",
					},
				},
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Query: "revenue",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Total != 1 {
		t.Fatalf("No results found")
	}
	expectedAnswer := "My Asset Name"
	asset := output.Results[0]
	if asset.Name != expectedAnswer {
		t.Fatalf("Expected answer '%s', got: '%s'", expectedAnswer, asset.Name)
	}
}
