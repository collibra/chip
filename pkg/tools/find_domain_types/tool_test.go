package find_domain_types_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/find_domain_types"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestFindDomainTypes(t *testing.T) {
	typeID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/2.0/domainTypes", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "50" {
			t.Fatalf("expected default limit 50, got: %s", r.URL.Query().Get("limit"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":1,"offset":0,"limit":50,"results":[{"id":"` + typeID.String() + `","name":"Physical Data Dictionary"}]}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: "Physical"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.Total != 1 || len(output.DomainTypes) != 1 {
		t.Fatalf("unexpected output: %+v", output)
	}
	if output.DomainTypes[0].ID != typeID.String() {
		t.Fatalf("expected id %s, got %s", typeID.String(), output.DomainTypes[0].ID)
	}
}
