package get_data_access_data_source_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	tool "github.com/collibra/chip/pkg/tools/get_data_access_data_source"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// The data access SDK posts all queries to the same GraphQL endpoint (<host>/dataAccess/query).
// Tests dispatch on the operation name embedded in the query body.
const gqlPath = "/dataAccess/query"

const dataSourceResp = `{"data":{"dataSource":{"__typename":"DataSource",` +
	`"id":"ds-1","name":"Snowflake Prod","type":"snowflake","description":"Production warehouse",` +
	`"createdAt":"2024-01-01T00:00:00Z","modifiedAt":"2024-02-01T00:00:00Z","parent":null,"edgeSiteInfo":null}}}`

const notFoundResp = `{"data":{"dataSource":{"__typename":"NotFoundError","message":"no such data source"}}}`

// newGQLServer returns a test server answering the GetDataSource operation with the supplied body.
func newGQLServer(t *testing.T, getDataSourceResp string) *httptest.Server {
	t.Helper()
	handler := http.NewServeMux()
	handler.HandleFunc(gqlPath, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		q := string(body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(q, "query GetDataSource"):
			_, _ = io.WriteString(w, getDataSourceResp)
		default:
			http.Error(w, "unexpected query: "+q, http.StatusBadRequest)
		}
	})
	return httptest.NewServer(handler)
}

func TestGetDataAccessDataSource_Found(t *testing.T) {
	server := newGQLServer(t, dataSourceResp)
	defer server.Close()

	ctx := chip.SetCollibraHost(t.Context(), "http://collibra.test")
	output, err := tool.NewTool(testutil.NewClient(server)).Handler(ctx, tool.Input{
		DataSourceID: "ds-1",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if output.Error != "" {
		t.Fatalf("Expected no tool error, got: %q", output.Error)
	}
	if output.Message != "" {
		t.Fatalf("Expected no message, got: %q", output.Message)
	}
	if output.DataSource == nil {
		t.Fatal("Expected a data source")
	}
	ds := output.DataSource
	if ds.ID != "ds-1" || ds.Name != "Snowflake Prod" || ds.Type != "snowflake" {
		t.Fatalf("Unexpected data source: %+v", ds)
	}
	if ds.Description != "Production warehouse" {
		t.Fatalf("Unexpected description: %q", ds.Description)
	}
	if ds.CreatedAt.IsZero() || ds.ModifiedAt.IsZero() {
		t.Fatalf("Expected timestamps to be populated, got: %+v", ds)
	}
}

func TestGetDataAccessDataSource_NotFound(t *testing.T) {
	server := newGQLServer(t, notFoundResp)
	defer server.Close()

	ctx := chip.SetCollibraHost(t.Context(), "http://collibra.test")
	output, err := tool.NewTool(testutil.NewClient(server)).Handler(ctx, tool.Input{
		DataSourceID: "ghost",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if output.DataSource != nil {
		t.Fatalf("Expected no data source, got: %+v", output.DataSource)
	}
	if output.Error != "" {
		t.Fatalf("Expected no tool error, got: %q", output.Error)
	}
	if output.Message == "" {
		t.Fatal("Expected a message asking the user to correct the ID")
	}
}

func TestGetDataAccessDataSource_RequiresID(t *testing.T) {
	output, err := tool.NewTool(nil).Handler(t.Context(), tool.Input{DataSourceID: "  "})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if output.Error == "" {
		t.Fatal("Expected an error when no data source ID is supplied")
	}
}
