package deploy_dq_rule_template_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/tools/deploy_dq_rule_template"
	"github.com/collibra/chip/pkg/tools/testutil"
)

type capture struct {
	path    string
	targets []map[string]string
}

func server(t *testing.T, code int, rec *capture) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/dq/internal/v1/rules/templates/{templateId}/deploy", func(w http.ResponseWriter, r *http.Request) {
		if rec != nil {
			rec.path = r.URL.Path
			var body struct {
				Targets []map[string]string `json:"targets"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			rec.targets = body.Targets
		}
		if code == 0 {
			code = http.StatusNoContent
		}
		if code != http.StatusNoContent {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func TestDeployDQRuleTemplate_HappyPath(t *testing.T) {
	var rec capture
	c := server(t, http.StatusNoContent, &rec)
	out, err := deploy_dq_rule_template.NewTool(c).Handler(t.Context(), deploy_dq_rule_template.Input{
		TemplateID: "t1",
		Targets: []deploy_dq_rule_template.Target{
			{JobName: "PUBLIC.CUSTOMERS", ColumnName: "email"},
			{JobName: "PUBLIC.CUSTOMERS", ColumnName: "name"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != deploy_dq_rule_template.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if out.Count != 2 {
		t.Fatalf("count = %d, want 2", out.Count)
	}
	if rec.path != "/rest/dq/internal/v1/rules/templates/t1/deploy" {
		t.Fatalf("unexpected path: %s", rec.path)
	}
	if len(rec.targets) != 2 || rec.targets[0]["columnName"] != "email" {
		t.Fatalf("unexpected targets: %+v", rec.targets)
	}
}

func TestDeployDQRuleTemplate_MissingTargets(t *testing.T) {
	c := server(t, http.StatusNoContent, nil)
	out, _ := deploy_dq_rule_template.NewTool(c).Handler(t.Context(), deploy_dq_rule_template.Input{TemplateID: "t1"})
	if out.Status != deploy_dq_rule_template.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestDeployDQRuleTemplate_MissingJobName(t *testing.T) {
	c := server(t, http.StatusNoContent, nil)
	out, _ := deploy_dq_rule_template.NewTool(c).Handler(t.Context(), deploy_dq_rule_template.Input{
		TemplateID: "t1",
		Targets:    []deploy_dq_rule_template.Target{{ColumnName: "email"}},
	})
	if out.Status != deploy_dq_rule_template.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestDeployDQRuleTemplate_DownstreamErrorSurfaces(t *testing.T) {
	c := server(t, http.StatusNotFound, nil)
	out, _ := deploy_dq_rule_template.NewTool(c).Handler(t.Context(), deploy_dq_rule_template.Input{
		TemplateID: "t1",
		Targets:    []deploy_dq_rule_template.Target{{JobName: "DS"}},
	})
	if out.Status != deploy_dq_rule_template.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}
