package detach_dq_rule_from_template_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/detach_dq_rule_from_template"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	templateName = "Null Check"
	jobName      = "PUBLIC.CUSTOMERS"
	ruleName     = "Null_Check_email"
)

// state records what the mock DQ service was asked to do.
type state struct {
	detachCalls int
	detachBody  map[string]any
}

// opts configures the mock: which deployments the template reports, what the
// detach answers, and what reading the rule back returns.
type opts struct {
	deployments []map[string]any
	detachCode  int
	ruleCode    int
	rule        map[string]any
}

func newServer(t *testing.T, o opts) (*http.Client, *state) {
	t.Helper()
	st := &state{}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /rest/dq/1.0/ruleTemplates/{name}/deployments", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		results := o.deployments
		if results == nil {
			results = []map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": results, "total": len(results), "offset": 0, "limit": len(results),
		})
	})
	mux.HandleFunc("POST /rest/dq/1.0/ruleTemplates/{name}/detach", func(w http.ResponseWriter, r *http.Request) {
		st.detachCalls++
		_ = json.NewDecoder(r.Body).Decode(&st.detachBody)
		code := o.detachCode
		if code == 0 {
			code = http.StatusNoContent
		}
		w.WriteHeader(code)
	})
	mux.HandleFunc("GET /rest/dq/internal/v1/jobs/{job}/monitors/rules/{rule}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		code := o.ruleCode
		if code == 0 {
			code = http.StatusOK
		}
		w.WriteHeader(code)
		if o.rule != nil {
			_ = json.NewEncoder(w).Encode(o.rule)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv), st
}

func linkedDeployment() []map[string]any {
	return []map[string]any{{"jobName": jobName, "deployedRuleName": ruleName, "columnName": "email"}}
}

func standaloneRule() map[string]any {
	return map[string]any{
		"jobName": jobName, "monitorName": ruleName, "monitorType": "SIMPLE_SQL",
		"monitorValue": "email is not null", "columnName": "email", "tolerance": 0,
		"isActive": 1, "isSuppressed": false,
	}
}

func call(t *testing.T, client *http.Client, in tools.Input) tools.Output {
	t.Helper()
	out, err := tools.NewTool(client).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return out
}

func TestMissingInputsNamesAllThreeParts(t *testing.T) {
	client, st := newServer(t, opts{deployments: linkedDeployment()})
	out := call(t, client, tools.Input{TemplateName: templateName})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("status = %q, want needs_input", out.Status)
	}
	for _, want := range []string{"job_name", "deployed_rule_name"} {
		if !strings.Contains(out.Message, want) {
			t.Errorf("message = %q, want it to name the missing %s", out.Message, want)
		}
	}
	if st.detachCalls != 0 {
		t.Error("detach was called despite invalid input")
	}
}

func TestPreviewByDefaultWritesNothing(t *testing.T) {
	// §5.1: confirm=false must write nothing.
	client, st := newServer(t, opts{deployments: linkedDeployment()})
	out := call(t, client, tools.Input{TemplateName: templateName, JobName: jobName, DeployedRuleName: ruleName})
	if out.Status != tools.StatusPreview {
		t.Fatalf("status = %q, want preview", out.Status)
	}
	if st.detachCalls != 0 {
		t.Errorf("detach called %d time(s) on a preview, want 0", st.detachCalls)
	}
	if out.Detachment.Detached {
		t.Error("detached = true in a preview")
	}
	if !strings.Contains(out.Guidance, "confirm=true") {
		t.Errorf("guidance = %q, want it to say how to confirm", out.Guidance)
	}
}

func TestPreviewEchoesEveryFieldThatWouldBeWritten(t *testing.T) {
	// §5.2: the preview must echo every field that will be written.
	client, _ := newServer(t, opts{deployments: linkedDeployment()})
	out := call(t, client, tools.Input{TemplateName: templateName, JobName: jobName, DeployedRuleName: ruleName})
	d := out.Detachment
	if d.TemplateName != templateName || d.JobName != jobName || d.DeployedRuleName != ruleName {
		t.Errorf("preview = %+v, want all three identity fields echoed", d)
	}
}

func TestPreviewRefusesWhenRuleIsNotDeployedFromTemplate(t *testing.T) {
	// Pre-empts the API's opaque 400 so the user never approves a no-op.
	client, st := newServer(t, opts{deployments: []map[string]any{
		{"jobName": jobName, "deployedRuleName": "Some_Other_Rule"},
	}})
	out := call(t, client, tools.Input{TemplateName: templateName, JobName: jobName, DeployedRuleName: ruleName})
	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
	if st.detachCalls != 0 {
		t.Error("detach was called for a rule that is not linked")
	}
	if !strings.Contains(out.Guidance, "already standalone") {
		t.Errorf("guidance = %q, want it to raise the already-standalone possibility", out.Guidance)
	}
}

func TestConfirmDetachesAndSendsTheRightPayload(t *testing.T) {
	client, st := newServer(t, opts{deployments: linkedDeployment(), rule: standaloneRule()})
	out := call(t, client, tools.Input{
		TemplateName: templateName, JobName: jobName, DeployedRuleName: ruleName, Confirm: true,
	})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success (message: %s)", out.Status, out.Message)
	}
	if st.detachCalls != 1 {
		t.Fatalf("detach called %d time(s), want exactly 1", st.detachCalls)
	}
	if st.detachBody["jobName"] != jobName || st.detachBody["deployedRuleName"] != ruleName {
		t.Errorf("detach body = %v, want jobName and deployedRuleName", st.detachBody)
	}
	if !out.Detachment.Detached {
		t.Error("detached = false after a confirmed detach")
	}
}

func TestConfirmReadsBackTheStandaloneDefinition(t *testing.T) {
	// AC: on success return the rule's current standalone definition.
	client, _ := newServer(t, opts{deployments: linkedDeployment(), rule: standaloneRule()})
	out := call(t, client, tools.Input{
		TemplateName: templateName, JobName: jobName, DeployedRuleName: ruleName, Confirm: true,
	})
	rule := out.Detachment.Rule
	if rule == nil {
		t.Fatal("rule is nil, want the standalone definition read back")
	}
	if rule.RuleSQL != "email is not null" {
		t.Errorf("ruleSql = %q, want the definition preserved verbatim", rule.RuleSQL)
	}
	if rule.StillLinked {
		t.Error("stillLinked = true, want false once the template link is cleared")
	}
}

func TestConfirmReportsASurvivingTemplateLink(t *testing.T) {
	// If the API reports the rule still linked, say so rather than claiming a
	// clean detach.
	stale := standaloneRule()
	stale["templateId"] = "11111111-2222-3333-4444-555555555555"
	client, _ := newServer(t, opts{deployments: linkedDeployment(), rule: stale})
	out := call(t, client, tools.Input{
		TemplateName: templateName, JobName: jobName, DeployedRuleName: ruleName, Confirm: true,
	})
	if !out.Detachment.Rule.StillLinked {
		t.Error("stillLinked = false, want true when templateId is still set")
	}
	if !strings.Contains(out.Guidance, "still reports a source template") {
		t.Errorf("guidance = %q, want it to flag the surviving link", out.Guidance)
	}
}

func TestReadBackFailureDoesNotReportTheDetachAsFailed(t *testing.T) {
	// The write has already committed; a failed re-read must not look like a
	// failed detach.
	client, st := newServer(t, opts{deployments: linkedDeployment(), ruleCode: http.StatusInternalServerError})
	out := call(t, client, tools.Input{
		TemplateName: templateName, JobName: jobName, DeployedRuleName: ruleName, Confirm: true,
	})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success - the detach itself succeeded", out.Status)
	}
	if st.detachCalls != 1 {
		t.Errorf("detach calls = %d, want 1", st.detachCalls)
	}
	if out.Detachment.Rule != nil {
		t.Error("rule is populated despite the read-back failing")
	}
	if !strings.Contains(out.Guidance, "get_data_quality_rule") {
		t.Errorf("guidance = %q, want it to point at the read-back tool", out.Guidance)
	}
}

func TestAlreadyStandaloneIsADescriptiveError(t *testing.T) {
	// AC: an already-standalone rule must be a clear error, not a silent no-op.
	// The API answers 400 and cannot distinguish this from "belongs to another
	// template", so the guidance must raise both.
	client, _ := newServer(t, opts{deployments: linkedDeployment(), detachCode: http.StatusBadRequest})
	out := call(t, client, tools.Input{
		TemplateName: templateName, JobName: jobName, DeployedRuleName: ruleName, Confirm: true,
	})
	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
	if !strings.Contains(out.Message, "not linked to template") {
		t.Errorf("message = %q, want it to state the rule is not linked", out.Message)
	}
	for _, want := range []string{"already standalone", "different template"} {
		if !strings.Contains(out.Guidance, want) {
			t.Errorf("guidance = %q, want it to raise %q", out.Guidance, want)
		}
	}
}

func TestUnknownRuleOrTemplateIsADescriptiveError(t *testing.T) {
	client, _ := newServer(t, opts{deployments: linkedDeployment(), detachCode: http.StatusNotFound})
	out := call(t, client, tools.Input{
		TemplateName: templateName, JobName: jobName, DeployedRuleName: ruleName, Confirm: true,
	})
	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
	if !strings.Contains(out.Message, templateName) || !strings.Contains(out.Message, ruleName) {
		t.Errorf("message = %q, want both names echoed since the API cannot say which is unknown", out.Message)
	}
}

func TestForbiddenNamesTheDeployPermission(t *testing.T) {
	client, _ := newServer(t, opts{deployments: linkedDeployment(), detachCode: http.StatusForbidden})
	out := call(t, client, tools.Input{
		TemplateName: templateName, JobName: jobName, DeployedRuleName: ruleName, Confirm: true,
	})
	if !strings.Contains(out.Guidance, "Deploy Templates") {
		t.Errorf("guidance = %q, want it to name the Deploy Templates permission", out.Guidance)
	}
}

func TestDownstreamErrorsAreMapped(t *testing.T) {
	for _, tc := range []struct {
		name string
		code int
	}{
		{"unauthorized", http.StatusUnauthorized},
		{"unprocessable", http.StatusUnprocessableEntity},
		{"server error", http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, _ := newServer(t, opts{deployments: linkedDeployment(), detachCode: tc.code})
			out := call(t, client, tools.Input{
				TemplateName: templateName, JobName: jobName, DeployedRuleName: ruleName, Confirm: true,
			})
			if out.Status != tools.StatusError {
				t.Fatalf("status = %q, want error", out.Status)
			}
			if out.Message == "" || out.Guidance == "" {
				t.Errorf("message/guidance = %q/%q, want both populated", out.Message, out.Guidance)
			}
		})
	}
}
