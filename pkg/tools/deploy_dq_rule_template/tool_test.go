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

// server mocks the public deploy endpoint. When code is 200 (or 0) it returns
// the given per-target results array (RuleTemplateDeployResult); otherwise it
// returns an error body with the given status.
func server(t *testing.T, code int, results []map[string]any, rec *capture) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/dq/1.0/ruleTemplates/{ruleTemplateName}/deploy", func(w http.ResponseWriter, r *http.Request) {
		if rec != nil {
			rec.path = r.URL.Path
			var body struct {
				Targets []map[string]string `json:"targets"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			rec.targets = body.Targets
		}
		if code == 0 {
			code = http.StatusOK
		}
		if code != http.StatusOK {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func TestDeployDQRuleTemplate_HappyPath(t *testing.T) {
	var rec capture
	c := server(t, http.StatusOK, []map[string]any{
		{"jobName": "PUBLIC.CUSTOMERS", "columnName": "email", "deployedRuleName": "NotNull_email", "status": "deployed"},
		{"jobName": "PUBLIC.CUSTOMERS", "columnName": "name", "deployedRuleName": "NotNull_name", "status": "deployed"},
	}, &rec)
	out, err := deploy_dq_rule_template.NewTool(c).Handler(t.Context(), deploy_dq_rule_template.Input{
		RuleTemplateName: "Not Null Check",
		Targets: []deploy_dq_rule_template.Target{
			{JobName: "PUBLIC.CUSTOMERS", ColumnName: "email"},
			{JobName: "PUBLIC.CUSTOMERS", ColumnName: "name"},
		},
		Confirm: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != deploy_dq_rule_template.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if out.Deployed != 2 || out.Skipped != 0 || len(out.Outcomes) != 2 {
		t.Fatalf("unexpected tally: deployed=%d skipped=%d outcomes=%+v", out.Deployed, out.Skipped, out.Outcomes)
	}
	if out.Outcomes[0].DeployedRuleName != "NotNull_email" {
		t.Fatalf("unexpected outcome: %+v", out.Outcomes[0])
	}
	if rec.path != "/rest/dq/1.0/ruleTemplates/Not Null Check/deploy" {
		t.Fatalf("unexpected path: %s", rec.path)
	}
	if len(rec.targets) != 2 || rec.targets[0]["columnName"] != "email" {
		t.Fatalf("unexpected targets: %+v", rec.targets)
	}
}

func TestDeployDQRuleTemplate_PartialSuccess(t *testing.T) {
	c := server(t, http.StatusOK, []map[string]any{
		{"jobName": "PUBLIC.CUSTOMERS", "columnName": "email", "deployedRuleName": "NotNull_email", "status": "deployed"},
		{"jobName": "PUBLIC.CUSTOMERS", "columnName": "name", "status": "SKIPPED", "reason": "rule already exists"},
	}, nil)
	out, _ := deploy_dq_rule_template.NewTool(c).Handler(t.Context(), deploy_dq_rule_template.Input{
		RuleTemplateName: "Not Null Check",
		Targets: []deploy_dq_rule_template.Target{
			{JobName: "PUBLIC.CUSTOMERS", ColumnName: "email"},
			{JobName: "PUBLIC.CUSTOMERS", ColumnName: "name"},
		},
		Confirm: true,
	})
	if out.Status != deploy_dq_rule_template.StatusPartial {
		t.Fatalf("status = %q, want partial (%s)", out.Status, out.Message)
	}
	if out.Deployed != 1 || out.Skipped != 1 {
		t.Fatalf("unexpected tally: deployed=%d skipped=%d", out.Deployed, out.Skipped)
	}
	if out.Outcomes[1].Reason != "rule already exists" {
		t.Fatalf("expected skip reason surfaced, got %+v", out.Outcomes[1])
	}
}

func TestDeployDQRuleTemplate_MissingTargets(t *testing.T) {
	c := server(t, http.StatusOK, nil, nil)
	out, _ := deploy_dq_rule_template.NewTool(c).Handler(t.Context(), deploy_dq_rule_template.Input{RuleTemplateName: "t1"})
	if out.Status != deploy_dq_rule_template.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestDeployDQRuleTemplate_MissingJobName(t *testing.T) {
	c := server(t, http.StatusOK, nil, nil)
	out, _ := deploy_dq_rule_template.NewTool(c).Handler(t.Context(), deploy_dq_rule_template.Input{
		RuleTemplateName: "t1",
		Targets:          []deploy_dq_rule_template.Target{{ColumnName: "email"}},
	})
	if out.Status != deploy_dq_rule_template.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestDeployDQRuleTemplate_DownstreamErrorSurfaces(t *testing.T) {
	c := server(t, http.StatusNotFound, nil, nil)
	out, _ := deploy_dq_rule_template.NewTool(c).Handler(t.Context(), deploy_dq_rule_template.Input{
		RuleTemplateName: "t1",
		Targets:          []deploy_dq_rule_template.Target{{JobName: "DS"}},
		Confirm:          true,
	})
	if out.Status != deploy_dq_rule_template.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}

func TestDeployDQRuleTemplate_PreviewByDefault_DeploysNothing(t *testing.T) {
	var rec capture
	c := server(t, http.StatusOK, nil, &rec)
	out, err := deploy_dq_rule_template.NewTool(c).Handler(t.Context(), deploy_dq_rule_template.Input{
		RuleTemplateName: "t1",
		Targets:          []deploy_dq_rule_template.Target{{JobName: "PUBLIC.CUSTOMERS", ColumnName: "email"}},
		// Confirm omitted -> preview
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != deploy_dq_rule_template.StatusPreview {
		t.Fatalf("status = %q, want preview", out.Status)
	}
	if out.Preview == nil || out.Preview.Count != 1 {
		t.Fatalf("expected preview with 1 target, got %+v", out.Preview)
	}
	// The deploy endpoint must not have been called.
	if rec.path != "" {
		t.Fatalf("expected no deploy call in preview mode, but server was hit: %s", rec.path)
	}
}
