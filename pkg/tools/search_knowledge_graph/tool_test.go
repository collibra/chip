package search_knowledge_graph_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/search_knowledge_graph"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// server mocks the Knowledge Graph hackathon search endpoint. It captures the
// raw request body and responds with the given status and body.
func server(t *testing.T, status int, body string, captured *string) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/knowledgeGraph/v1/hackaton/search", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		if captured != nil {
			*captured = string(raw)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func TestSearchKnowledgeGraph_HappyPath_ReturnsMatchingAssets(t *testing.T) {
	var reqBody string
	resp := `[{"id":"4:abc:1","labels":["Asset","Table"],"score":9.0,"properties":{"name":"sales_fact"}}]`
	c := server(t, http.StatusOK, resp, &reqBody)

	out, err := search_knowledge_graph.NewTool(c).Handler(t.Context(), search_knowledge_graph.Input{Query: "@type:Table sales"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != search_knowledge_graph.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if out.Count != 1 || out.Results[0].ID != "4:abc:1" || out.Results[0].Properties["name"] != "sales_fact" {
		t.Fatalf("unexpected results: %+v", out.Results)
	}
	if !strings.Contains(reqBody, `"query":"@type:Table sales"`) {
		t.Fatalf("expected query in request body: %s", reqBody)
	}
}

func TestSearchKnowledgeGraph_RequiresNonEmptyQuery(t *testing.T) {
	c := server(t, http.StatusOK, `[]`, nil)
	out, _ := search_knowledge_graph.NewTool(c).Handler(t.Context(), search_knowledge_graph.Input{Query: "   "})
	if out.Status != search_knowledge_graph.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestSearchKnowledgeGraph_DownstreamErrorSurfaces(t *testing.T) {
	c := server(t, http.StatusInternalServerError, `{"message":"boom"}`, nil)
	out, _ := search_knowledge_graph.NewTool(c).Handler(t.Context(), search_knowledge_graph.Input{Query: "revenue"})
	if out.Status != search_knowledge_graph.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}
