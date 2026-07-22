package clients_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestFindDomainTypes(t *testing.T) {
	typeID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/2.0/domainTypes", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != "Physical Data Dictionary" {
			t.Fatalf("unexpected name query param: %s", r.URL.Query().Get("name"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":1,"offset":0,"limit":50,"results":[{"id":"` + typeID.String() + `","name":"Physical Data Dictionary","publicId":"PhysicalDataDictionary"}]}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	response, err := clients.FindDomainTypes(t.Context(), client, clients.FindDomainTypesQueryParams{Name: "Physical Data Dictionary", Limit: 50})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if response.Total != 1 || len(response.Results) != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.Results[0].ID != typeID.String() {
		t.Fatalf("expected id %s, got %s", typeID.String(), response.Results[0].ID)
	}
}
