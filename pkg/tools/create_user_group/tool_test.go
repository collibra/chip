package create_user_group_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/create_user_group"
	"github.com/collibra/chip/pkg/tools/outcome"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// server is a DGC with the user groups in existing, that records every create it receives.
func server(t *testing.T, existing string, creates *[]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/userGroups", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("nameMatchMode") != "EXACT" {
			t.Errorf("the name check must be exact, got %v", r.URL.Query())
		}
		if existing != "" && strings.EqualFold(r.URL.Query().Get("name"), existing) {
			_, _ = w.Write([]byte(`{"total":1,"offset":0,"limit":1,"results":[{"id":"g-existing","name":"` + existing + `","system":false}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"total":0,"offset":0,"limit":1,"results":[]}`))
	})
	mux.HandleFunc("POST /rest/2.0/userGroups", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request map[string]string
		_ = json.Unmarshal(body, &request)
		*creates = append(*creates, request["name"])
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"g-new","name":"` + request["name"] + `","system":false}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestCreateUserGroup_PreviewWritesNothing(t *testing.T) {
	var creates []string
	output, err := tools.NewTool(testutil.NewClient(server(t, "", &creates))).Handler(t.Context(), tools.Input{Name: "  Product Owners "})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.Status != outcome.ConfirmRequired || output.Name != "Product Owners" || len(creates) != 0 {
		t.Fatalf("expected a preview of the trimmed name and no write, got %+v, creates=%v", output, creates)
	}
}

func TestCreateUserGroup_ConfirmCreatesTheGroup(t *testing.T) {
	var creates []string
	output, err := tools.NewTool(testutil.NewClient(server(t, "", &creates))).Handler(t.Context(), tools.Input{Name: "Product Owners", Confirm: true})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.Status != tools.StatusCreated || output.ID != "g-new" || len(creates) != 1 || creates[0] != "Product Owners" {
		t.Fatalf("unexpected output %+v, creates=%v", output, creates)
	}
}

func TestCreateUserGroup_RefusesATakenNameAndReturnsTheExistingGroup(t *testing.T) {
	var creates []string
	output, _ := tools.NewTool(testutil.NewClient(server(t, "Product Owners", &creates))).Handler(t.Context(), tools.Input{Name: "product owners", Confirm: true})
	if output.Status != outcome.NeedsInput || output.ID != "g-existing" || len(creates) != 0 {
		t.Fatalf("expected a refusal naming the existing group and no write, got %+v, creates=%v", output, creates)
	}
}

func TestCreateUserGroup_AnEmptyNameNeedsInput(t *testing.T) {
	var creates []string
	output, _ := tools.NewTool(testutil.NewClient(server(t, "", &creates))).Handler(t.Context(), tools.Input{Name: "   ", Confirm: true})
	if output.Status != outcome.NeedsInput || len(creates) != 0 {
		t.Fatalf("expected needs_input and no write, got %+v", output)
	}
}

func TestCreateUserGroup_ReadsDGCsRefusalIntoAStatusAndNextStep(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/userGroups", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"total":0,"offset":0,"limit":1,"results":[]}`))
	})
	mux.HandleFunc("POST /rest/2.0/userGroups", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"statusCode":403,"errorCode":"permissionDenied","userMessage":"You may not create user groups."}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	output, err := tools.NewTool(testutil.NewClient(srv)).Handler(t.Context(), tools.Input{Name: "X", Confirm: true})
	if err != nil {
		t.Fatalf("a DGC refusal is a status, not a Go error: %v", err)
	}
	if output.Status != outcome.Error || !strings.Contains(output.Message, "You may not create user groups.") || !strings.Contains(output.Guidance, "permission") {
		t.Fatalf("unexpected output: %+v", output)
	}
}
