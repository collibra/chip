package validate_dq_rule_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/collibra/chip/pkg/tools/validate_dq_rule"
)

func server(t *testing.T, code int, resp clients.ValidateDQRuleResponse) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/dq/internal/v1/rules/validate", func(w http.ResponseWriter, r *http.Request) {
		if code != http.StatusOK {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func input() validate_dq_rule.Input {
	return validate_dq_rule.Input{
		EdgeSiteID: "site", ConnectionID: "conn", SchemaName: "public", JobName: "DS", PreviewRule: "SELECT 1",
	}
}

func TestValidateDQRule_Valid(t *testing.T) {
	c := server(t, http.StatusOK, clients.ValidateDQRuleResponse{IsValid: true, Message: "ok"})
	out, err := validate_dq_rule.NewTool(c).Handler(t.Context(), input())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != validate_dq_rule.StatusSuccess || !out.Valid {
		t.Fatalf("expected success+valid, got status=%q valid=%v (%s)", out.Status, out.Valid, out.Message)
	}
}

func TestValidateDQRule_InvalidRuleStillSuccess(t *testing.T) {
	c := server(t, http.StatusOK, clients.ValidateDQRuleResponse{IsValid: false, Message: "bad SQLG"})
	out, _ := validate_dq_rule.NewTool(c).Handler(t.Context(), input())
	if out.Status != validate_dq_rule.StatusSuccess {
		t.Fatalf("status = %q, want success (validation ran)", out.Status)
	}
	if out.Valid {
		t.Fatalf("expected valid=false")
	}
}

func TestValidateDQRule_MissingInput(t *testing.T) {
	c := server(t, http.StatusOK, clients.ValidateDQRuleResponse{})
	in := input()
	in.PreviewRule = ""
	out, _ := validate_dq_rule.NewTool(c).Handler(t.Context(), in)
	if out.Status != validate_dq_rule.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}
