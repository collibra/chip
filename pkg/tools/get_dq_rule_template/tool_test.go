package get_dq_rule_template_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/get_dq_rule_template"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func server(t *testing.T, code int, tmpl clients.DQRuleTemplate) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/dq/internal/v1/rules/templates/{templateId}", func(w http.ResponseWriter, r *http.Request) {
		if code != http.StatusOK {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(tmpl)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func TestGetDQRuleTemplate_HappyPath(t *testing.T) {
	c := server(t, http.StatusOK, clients.DQRuleTemplate{ID: "t1", Name: "Not Null Check", Ootb: true, DeploymentCount: 3})
	out, err := get_dq_rule_template.NewTool(c).Handler(t.Context(), get_dq_rule_template.Input{TemplateID: "t1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != get_dq_rule_template.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if out.Template == nil || out.Template.Name != "Not Null Check" || out.Template.DeploymentCount != 3 {
		t.Fatalf("unexpected template: %+v", out.Template)
	}
}

func TestGetDQRuleTemplate_MissingInput(t *testing.T) {
	c := server(t, http.StatusOK, clients.DQRuleTemplate{})
	out, _ := get_dq_rule_template.NewTool(c).Handler(t.Context(), get_dq_rule_template.Input{})
	if out.Status != get_dq_rule_template.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestGetDQRuleTemplate_NotFound(t *testing.T) {
	c := server(t, http.StatusNotFound, clients.DQRuleTemplate{})
	out, _ := get_dq_rule_template.NewTool(c).Handler(t.Context(), get_dq_rule_template.Input{TemplateID: "nope"})
	if out.Status != get_dq_rule_template.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}
