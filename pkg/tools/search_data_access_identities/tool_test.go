package search_data_access_identities_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	tool "github.com/collibra/chip/pkg/tools/search_data_access_identities"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	gqlPath = "/dataAccess/query"
	host    = "http://collibra.test"
)

// fakeDataAccess is a Data Access double that records which query the tool chose — the exact
// email lookup or the paged user list — and answers with canned users.
type fakeDataAccess struct {
	byEmailQueries []string
	listQueries    []string
	byEmail        string
	list           string
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
		body := ""
		switch {
		case strings.Contains(query, "query GetUserByEmail"):
			f.byEmailQueries = append(f.byEmailQueries, query)
			body = f.byEmail
			if body == "" {
				body = notFoundResponse
			}
		case strings.Contains(query, "query ListUsers"):
			f.listQueries = append(f.listQueries, query)
			body = f.list
			if body == "" {
				body = userPage(false)
			}
		default:
			http.Error(w, "unexpected query: "+query, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("Unexpected request: %s %s", r.Method, r.URL.Path)
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func (f *fakeDataAccess) run(t *testing.T, input tool.SearchDataAccessIdentitiesInput) tool.SearchDataAccessIdentitiesOutput {
	t.Helper()
	ctx := chip.SetCollibraHost(t.Context(), host)
	output, err := tool.NewTool(testutil.NewClient(f.server(t))).Handler(ctx, input)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	return output
}

const notFoundResponse = `{"data":{"userByEmail":{"__typename":"NotFoundError",` +
	`"message":"No user found for the given email address."}}}`

func userJSON(id, name, email string) string {
	return `{"__typename":"User","id":"` + id + `","name":"` + name + `","email":"` + email + `","type":"Human"}`
}

func byEmailResponse(id, name, email string) string {
	return `{"data":{"userByEmail":` + userJSON(id, name, email) + `}}`
}

// userPage renders a UserConnection holding the supplied users.
func userPage(hasNextPage bool, users ...string) string {
	edges := make([]string, 0, len(users))
	for i, user := range users {
		edges = append(edges, `{"cursor":"c`+string(rune('0'+i))+`","node":`+user+`}`)
	}
	next := "false"
	if hasNextPage {
		next = "true"
	}
	return `{"data":{"users":{"__typename":"UserConnection",` +
		`"pageInfo":{"hasNextPage":` + next + `,"startCursor":"c0"},` +
		`"edges":[` + strings.Join(edges, ",") + `]}}}`
}

// searchOf extracts the UserFilterInput search term the tool sent.
func searchOf(t *testing.T, query string) (string, bool) {
	t.Helper()
	var body struct {
		Variables struct {
			Filter map[string]any `json:"filter"`
		} `json:"variables"`
	}
	if err := json.Unmarshal([]byte(query), &body); err != nil {
		t.Fatalf("Expected a JSON GraphQL request, got: %v (%s)", err, query)
	}
	search, ok := body.Variables.Filter["search"].(string)
	return search, ok
}

func TestEmailPerformsAnExactLookup(t *testing.T) {
	fake := &fakeDataAccess{byEmail: byEmailResponse("da-user-1", "Alice", "alice@example.com")}

	output := fake.run(t, tool.SearchDataAccessIdentitiesInput{Email: "alice@example.com"})

	if output.Error != "" || output.Status != "" {
		t.Fatalf("Expected a plain successful lookup, got status %q error %q", output.Status, output.Error)
	}
	if len(fake.listQueries) != 0 {
		t.Fatalf("Expected no user listing for an email lookup, got: %v", fake.listQueries)
	}
	if len(output.Results) != 1 {
		t.Fatalf("Expected one result, got: %+v", output.Results)
	}
	got := output.Results[0]
	if got.ID != "da-user-1" || got.Name != "Alice" || got.Type != "Human" {
		t.Fatalf("Unexpected identity: %+v", got)
	}
	if got.Email == nil || *got.Email != "alice@example.com" {
		t.Fatalf("Expected the email to be mapped, got: %+v", got.Email)
	}
	if got.Url != host+"/data-access/identities/da-user-1" {
		t.Fatalf("Unexpected UI url: %q", got.Url)
	}
}

func TestUnknownEmailReturnsNoResultsRatherThanAnError(t *testing.T) {
	fake := &fakeDataAccess{byEmail: notFoundResponse}

	output := fake.run(t, tool.SearchDataAccessIdentitiesInput{Email: "nobody@example.com"})

	if output.Error != "" {
		t.Fatalf("Expected an unknown email not to be an error, got: %q", output.Error)
	}
	if len(output.Results) != 0 {
		t.Fatalf("Expected no results, got: %+v", output.Results)
	}
}

func TestNamePerformsAServerSideSearch(t *testing.T) {
	fake := &fakeDataAccess{list: userPage(false, userJSON("da-user-1", "Alice", "alice@example.com"))}

	output := fake.run(t, tool.SearchDataAccessIdentitiesInput{Name: "ali"})

	if len(fake.byEmailQueries) != 0 {
		t.Fatalf("Expected no email lookup for a name search, got: %v", fake.byEmailQueries)
	}
	if len(fake.listQueries) != 1 {
		t.Fatalf("Expected one user listing, got %d", len(fake.listQueries))
	}
	search, ok := searchOf(t, fake.listQueries[0])
	if !ok || search != "ali" {
		t.Fatalf("Expected the name to be sent as the search filter, got %q (present=%v)", search, ok)
	}
	if len(output.Results) != 1 || output.Results[0].Name != "Alice" {
		t.Fatalf("Expected the matching user, got: %+v", output.Results)
	}
}

func TestNameIsTrimmedBeforeItIsSent(t *testing.T) {
	fake := &fakeDataAccess{}

	fake.run(t, tool.SearchDataAccessIdentitiesInput{Name: "  alice  "})

	search, ok := searchOf(t, fake.listQueries[0])
	if !ok || search != "alice" {
		t.Fatalf("Expected a trimmed search term, got %q (present=%v)", search, ok)
	}
}

func TestNameFiltersTheEmailResultClientSide(t *testing.T) {
	fake := &fakeDataAccess{byEmail: byEmailResponse("da-user-1", "Alice", "alice@example.com")}

	kept := fake.run(t, tool.SearchDataAccessIdentitiesInput{Email: "alice@example.com", Name: "ALI"})
	if len(kept.Results) != 1 {
		t.Fatalf("Expected the case-insensitive name match to keep the user, got: %+v", kept.Results)
	}

	dropped := fake.run(t, tool.SearchDataAccessIdentitiesInput{Email: "alice@example.com", Name: "bob"})
	if len(dropped.Results) != 0 {
		t.Fatalf("Expected a non-matching name to drop the user, got: %+v", dropped.Results)
	}
}

func TestResultsAreCappedAtPageSize(t *testing.T) {
	fake := &fakeDataAccess{list: userPage(true,
		userJSON("da-user-1", "Alice", "alice@example.com"),
		userJSON("da-user-2", "Alicia", "alicia@example.com"),
	)}

	output := fake.run(t, tool.SearchDataAccessIdentitiesInput{Name: "ali", PageSize: 1})

	if len(output.Results) != 1 {
		t.Fatalf("Expected pageSize=1 to cap the results, got: %+v", output.Results)
	}
	if output.Results[0].ID != "da-user-1" {
		t.Fatalf("Expected the first user, got: %+v", output.Results[0])
	}
}

func TestDownstreamFailureIsReportedAsAnError(t *testing.T) {
	fake := &fakeDataAccess{list: `{"data":{"users":{"__typename":"PermissionDeniedError",` +
		`"message":"You are not allowed to list users."}}}`}

	output := fake.run(t, tool.SearchDataAccessIdentitiesInput{Name: "ali"})

	if !strings.Contains(output.Error, "Failed to search Data Access identities") {
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
	tests := map[string]tool.SearchDataAccessIdentitiesInput{
		"empty":                {},
		"blank name":           {Name: "  "},
		"blank name and email": {Name: " ", Email: "  "},
		"only pageSize":        {PageSize: 25},
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
			if len(fake.byEmailQueries)+len(fake.listQueries) != 0 {
				t.Fatal("Expected no request to Data Access")
			}
		})
	}
}

func TestPageSizeOutOfRangeIsRejected(t *testing.T) {
	tests := map[string]int{"just above": 26, "far above": 1000, "negative": -1}

	for name, pageSize := range tests {
		t.Run(name, func(t *testing.T) {
			fake := &fakeDataAccess{}
			output := fake.run(t, tool.SearchDataAccessIdentitiesInput{Name: "alice", PageSize: pageSize})

			if output.Status != "validation_error" {
				t.Fatalf("Expected status validation_error, got: %q", output.Status)
			}
			if !strings.Contains(output.Message, "between 1 and 25") {
				t.Fatalf("Expected the message to state the valid range, got: %q", output.Message)
			}
			if len(fake.byEmailQueries)+len(fake.listQueries) != 0 {
				t.Fatal("Expected no request to Data Access")
			}
		})
	}
}

func TestPageSizeAtTheCapIsAccepted(t *testing.T) {
	fake := &fakeDataAccess{}

	output := fake.run(t, tool.SearchDataAccessIdentitiesInput{Name: "alice", PageSize: 25})

	if output.Status == "validation_error" {
		t.Fatalf("Expected pageSize 25 to be accepted, got: %s", output.Message)
	}
	if len(fake.listQueries) != 1 {
		t.Fatalf("Expected the search to reach Data Access, got %d queries", len(fake.listQueries))
	}
}
