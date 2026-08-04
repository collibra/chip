package search_catalog_columns_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/search_catalog_columns"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// server mocks the KG GraphQL endpoint. It captures the raw request body and
// returns either the given assets or a graphql error.
func server(t *testing.T, body string, graphqlErr bool, captured *string) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /graphql/knowledgeGraph/v1", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if captured != nil {
			*captured = string(raw)
		}
		w.Header().Set("Content-Type", "application/json")
		if graphqlErr {
			_, _ = w.Write([]byte(`{"errors":[{"message":"boom"}]}`))
			return
		}
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func TestSearchCatalogColumns_HappyPath_BuildsColumnAndRelationFilter(t *testing.T) {
	var reqBody string
	resp := `{"data":{"assets":[
		{"id":"c1","fullName":"schema.table.email","displayName":"email","type":{"name":"Column"},"domain":{"name":"Sales"}}
	]}}`
	c := server(t, resp, false, &reqBody)

	out, err := search_catalog_columns.NewTool(c).Handler(t.Context(), search_catalog_columns.Input{
		Domain:       "Sales",
		BusinessTerm: "Customer Email",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != search_catalog_columns.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if out.Count != 1 || out.Columns[0].FullName != "schema.table.email" || out.Columns[0].Domain != "Sales" {
		t.Fatalf("unexpected columns: %+v", out.Columns)
	}
	// The where clause must scope to Column and carry the Business Term relation
	// public-id.
	if !strings.Contains(reqBody, "\"Column\"") {
		t.Fatalf("expected type=Column in where clause: %s", reqBody)
	}
	if !strings.Contains(reqBody, "BusinessAssetRepresentsDataAsset") {
		t.Fatalf("expected business-term relation public id in where clause: %s", reqBody)
	}
	// Sanity: the request body must be valid JSON with a variables.where object.
	var parsed struct {
		Variables struct {
			Where map[string]any `json:"where"`
		} `json:"variables"`
	}
	if err := json.Unmarshal([]byte(reqBody), &parsed); err != nil {
		t.Fatalf("request body not valid JSON: %v", err)
	}
	if parsed.Variables.Where == nil {
		t.Fatalf("expected variables.where to be set")
	}
}

func TestSearchCatalogColumns_RequiresAtLeastOneFilter(t *testing.T) {
	c := server(t, `{"data":{"assets":[]}}`, false, nil)
	out, _ := search_catalog_columns.NewTool(c).Handler(t.Context(), search_catalog_columns.Input{})
	if out.Status != search_catalog_columns.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestSearchCatalogColumns_GraphqlErrorSurfaces(t *testing.T) {
	c := server(t, "", true, nil)
	out, _ := search_catalog_columns.NewTool(c).Handler(t.Context(), search_catalog_columns.Input{Description: "pii"})
	if out.Status != search_catalog_columns.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}
