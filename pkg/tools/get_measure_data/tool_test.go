package get_measure_data_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/get_measure_data"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const missingID = "11111111-2222-3333-4444-555555555555"
const realID = "9179b887-04ef-4ce5-ab3a-b5bbd39ea3c8"

// emptyRelations mimics /rest/2.0/relations, a filter query that answers 200 with an empty
// page for an unknown asset id exactly as it does for a real asset with no relations.
func emptyRelations(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(`{"total":0,"offset":0,"limit":1000,"results":[]}`))
}

func TestNonexistentMeasureIsAnError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/2.0/relations", emptyRelations)
	mux.HandleFunc("/rest/2.0/assets/"+missingID, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"statusCode":404,"errorCode":"termNotFoundId"}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{MeasureID: missingID})
	if err == nil {
		t.Fatal("expected an error for a nonexistent measure, got a success payload")
	}
	if !strings.Contains(err.Error(), "not found") || !strings.Contains(err.Error(), missingID) {
		t.Fatalf("expected error naming the missing id, got: %v", err)
	}
}

func TestExistingMeasureWithoutRelationsSucceeds(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/2.0/relations", emptyRelations)
	mux.HandleFunc("/rest/2.0/assets/"+realID, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"` + realID + `","name":"Example","type":{"id":"00000000-0000-0000-0000-000000031002","name":"Measure"}}`))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{MeasureID: realID})
	if err != nil {
		t.Fatalf("expected no error for a real measure with nothing connected, got: %v", err)
	}
	if len(output.DataHierarchy) != 0 {
		t.Fatalf("expected an empty result, got %d", len(output.DataHierarchy))
	}
}
