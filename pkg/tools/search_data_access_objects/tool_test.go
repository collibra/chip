package search_data_access_objects_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	tool "github.com/collibra/chip/pkg/tools/search_data_access_objects"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	gqlPath = "/dataAccess/query"
	host    = "http://collibra.test"
)

// fakeDataAccess is a Data Access double that records the ListDataObjects queries it received
// and answers them with a canned page of data objects.
type fakeDataAccess struct {
	queries []string
	pages   []string
}

func (f *fakeDataAccess) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(gqlPath, func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		query := string(raw)
		if !strings.Contains(query, "query ListDataObjects") {
			http.Error(w, "unexpected query: "+query, http.StatusBadRequest)
			return
		}
		f.queries = append(f.queries, query)

		page := emptyPage
		if len(f.pages) > 0 {
			page = f.pages[0]
			if len(f.pages) > 1 {
				f.pages = f.pages[1:]
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(page))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Unexpected request: %s %s", r.Method, r.URL.Path)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func (f *fakeDataAccess) run(t *testing.T, input tool.SearchDataAccessObjectsInput) tool.SearchDataAccessObjectsOutput {
	t.Helper()
	ctx := chip.SetCollibraHost(t.Context(), host)
	output, err := tool.NewTool(testutil.NewClient(f.server(t))).Handler(ctx, input)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	return output
}

// filterOf extracts the DataObjectFilterInput the tool sent, so the tests assert on the filter
// rather than on substrings of the serialized query.
func filterOf(t *testing.T, query string) map[string]any {
	t.Helper()
	var body struct {
		Variables struct {
			Filter map[string]any `json:"filter"`
			Limit  *int           `json:"limit"`
		} `json:"variables"`
	}
	if err := json.Unmarshal([]byte(query), &body); err != nil {
		t.Fatalf("Expected a JSON GraphQL request, got: %v (%s)", err, query)
	}
	return body.Variables.Filter
}

const emptyPage = `{"data":{"dataObjects":{"__typename":"DataObjectConnection",` +
	`"pageInfo":{"hasNextPage":false,"startCursor":null},"edges":[]}}}`

// page renders a DataObjectConnection whose nodes are the supplied JSON objects.
func page(hasNextPage bool, nodes ...string) string {
	edges := make([]string, 0, len(nodes))
	for i, node := range nodes {
		edges = append(edges, `{"cursor":"c`+string(rune('0'+i))+`","node":`+node+`}`)
	}
	next := "false"
	if hasNextPage {
		next = "true"
	}
	return `{"data":{"dataObjects":{"__typename":"DataObjectConnection",` +
		`"pageInfo":{"hasNextPage":` + next + `,"startCursor":"c0"},` +
		`"edges":[` + strings.Join(edges, ",") + `]}}}`
}

func objectJSON(id, name string) string {
	return `{"id":"` + id + `","name":"` + name + `","fullName":"DB.PUBLIC.` + name + `",` +
		`"type":"table","dataType":null,"deleted":false,"description":"Customer orders",` +
		`"dataSource":{"id":"ds-1"},"applicablePermissions":[{"name":"SELECT","description":"Read rows"}]}`
}

func TestSearchReturnsMappedObjects(t *testing.T) {
	fake := &fakeDataAccess{pages: []string{page(false, objectJSON("do-1", "customer_orders"))}}

	output := fake.run(t, tool.SearchDataAccessObjectsInput{Name: "customer"})

	if output.Error != "" || output.Status != "" {
		t.Fatalf("Expected a plain successful search, got status %q error %q", output.Status, output.Error)
	}
	if len(output.Results) != 1 {
		t.Fatalf("Expected one result, got: %+v", output.Results)
	}
	got := output.Results[0]
	if got.ID != "do-1" || got.Name != "customer_orders" || got.FullName != "DB.PUBLIC.customer_orders" {
		t.Fatalf("Unexpected identity fields: %+v", got)
	}
	if got.Type != "table" || got.Description != "Customer orders" || got.DataSourceID != "ds-1" {
		t.Fatalf("Unexpected object fields: %+v", got)
	}
	if got.Deleted {
		t.Fatalf("Expected deleted=false, got: %+v", got)
	}
	if len(got.ApplicablePermissions) != 1 || got.ApplicablePermissions[0].Name != "SELECT" {
		t.Fatalf("Expected the applicable permissions to be mapped, got: %+v", got.ApplicablePermissions)
	}
	if got.Url != host+"/data-access/data-objects/do-1" {
		t.Fatalf("Unexpected UI url: %q", got.Url)
	}
}

func TestEmptyResultIsNotAnError(t *testing.T) {
	fake := &fakeDataAccess{}

	output := fake.run(t, tool.SearchDataAccessObjectsInput{Name: "nothing matches"})

	if output.Error != "" {
		t.Fatalf("Expected no error for an empty result, got: %q", output.Error)
	}
	if len(output.Results) != 0 {
		t.Fatalf("Expected no results, got: %+v", output.Results)
	}
}

func TestEveryFilterIsForwarded(t *testing.T) {
	fake := &fakeDataAccess{}

	fake.run(t, tool.SearchDataAccessObjectsInput{
		Name:           "customer",
		DataSources:    []string{"ds-1", "ds-2"},
		Types:          []string{"table", "column"},
		Parents:        []string{"do-parent"},
		Ancestors:      []string{"do-ancestor"},
		IncludeDeleted: true,
	})

	if len(fake.queries) != 1 {
		t.Fatalf("Expected one query, got %d", len(fake.queries))
	}
	filter := filterOf(t, fake.queries[0])
	for key, want := range map[string]any{
		"search":         "customer",
		"dataSources":    []any{"ds-1", "ds-2"},
		"types":          []any{"table", "column"},
		"parents":        []any{"do-parent"},
		"ancestors":      []any{"do-ancestor"},
		"includeDeleted": true,
	} {
		got, ok := filter[key]
		if !ok {
			t.Fatalf("Expected %q in the filter, got: %+v", key, filter)
		}
		if !equal(got, want) {
			t.Fatalf("Expected filter %q to be %v, got %v", key, want, got)
		}
	}
}

func TestUnsetFiltersAreOmittedFromTheRequest(t *testing.T) {
	fake := &fakeDataAccess{}

	fake.run(t, tool.SearchDataAccessObjectsInput{Name: "customer"})

	filter := filterOf(t, fake.queries[0])
	for _, key := range []string{"dataSources", "types", "parents", "ancestors", "includeDeleted"} {
		if _, ok := filter[key]; ok {
			t.Fatalf("Expected %q to be omitted when unset, got: %+v", key, filter)
		}
	}
}

func TestResultsAreCappedAtPageSize(t *testing.T) {
	// Two objects available, one requested: the extra must not be returned even though the
	// page the service sent contains it.
	fake := &fakeDataAccess{pages: []string{
		page(true, objectJSON("do-1", "orders"), objectJSON("do-2", "order_items")),
	}}

	output := fake.run(t, tool.SearchDataAccessObjectsInput{Name: "order", PageSize: 1})

	if len(output.Results) != 1 {
		t.Fatalf("Expected pageSize=1 to cap the results, got: %+v", output.Results)
	}
	if output.Results[0].ID != "do-1" {
		t.Fatalf("Expected the first object, got: %+v", output.Results[0])
	}
}

func TestDownstreamFailureIsReportedAsAnError(t *testing.T) {
	fake := &fakeDataAccess{pages: []string{
		`{"data":{"dataObjects":{"__typename":"PermissionDeniedError","message":"You are not allowed to list data objects."}}}`,
	}}

	output := fake.run(t, tool.SearchDataAccessObjectsInput{Name: "customer"})

	if !strings.Contains(output.Error, "Failed to search data access objects") {
		t.Fatalf("Expected the failure to be reported, got: %q", output.Error)
	}
	if !strings.Contains(output.Error, "not allowed") {
		t.Fatalf("Expected the downstream message to be kept, got: %q", output.Error)
	}
	if output.Results != nil {
		t.Fatalf("Expected no results, got: %+v", output.Results)
	}
}

func TestUnfilteredSearchIsRejected(t *testing.T) {
	tests := map[string]tool.SearchDataAccessObjectsInput{
		"empty":               {},
		"blank name":          {Name: "   "},
		"only includeDeleted": {IncludeDeleted: true},
		"only pageSize":       {PageSize: 25},
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			fake := &fakeDataAccess{}
			output := fake.run(t, input)

			if output.Status != "validation_error" {
				t.Fatalf("Expected status validation_error, got: %q", output.Status)
			}
			if !strings.Contains(output.Message, "at least one filter") {
				t.Fatalf("Expected the message to state what is required, got: %q", output.Message)
			}
			if output.Results != nil {
				t.Fatalf("Expected no results, got: %+v", output.Results)
			}
			if len(fake.queries) != 0 {
				t.Fatalf("Expected no request to Data Access, got: %v", fake.queries)
			}
		})
	}
}

func TestPageSizeOutOfRangeIsRejected(t *testing.T) {
	tests := map[string]int{"just above": 26, "far above": 1000, "negative": -1}

	for name, pageSize := range tests {
		t.Run(name, func(t *testing.T) {
			fake := &fakeDataAccess{}
			output := fake.run(t, tool.SearchDataAccessObjectsInput{Name: "customer", PageSize: pageSize})

			if output.Status != "validation_error" {
				t.Fatalf("Expected status validation_error, got: %q", output.Status)
			}
			if !strings.Contains(output.Message, "between 1 and 25") {
				t.Fatalf("Expected the message to state the valid range, got: %q", output.Message)
			}
			if len(fake.queries) != 0 {
				t.Fatalf("Expected no request to Data Access, got: %v", fake.queries)
			}
		})
	}
}

func TestAnySingleFilterIsAccepted(t *testing.T) {
	tests := map[string]tool.SearchDataAccessObjectsInput{
		"name":        {Name: "customer"},
		"dataSources": {DataSources: []string{"ds-1"}},
		"types":       {Types: []string{"table"}},
		"parents":     {Parents: []string{"do-1"}},
		"ancestors":   {Ancestors: []string{"do-1"}},
		"pageSize 25": {Name: "customer", PageSize: 25},
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			fake := &fakeDataAccess{}
			output := fake.run(t, input)

			if output.Status == "validation_error" {
				t.Fatalf("Expected the filter to be accepted, got: %s", output.Message)
			}
			if len(fake.queries) != 1 {
				t.Fatalf("Expected the search to reach Data Access, got %d queries", len(fake.queries))
			}
		})
	}
}

// equal compares the JSON-decoded filter values the fake received.
func equal(got, want any) bool {
	gotJSON, err := json.Marshal(got)
	if err != nil {
		return false
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		return false
	}
	return string(gotJSON) == string(wantJSON)
}
