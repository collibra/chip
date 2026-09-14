package list_dq_rule_template_deployments_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/list_dq_rule_template_deployments"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const templateName = "Row Count Range"

func newServer(t *testing.T, code int, body any) *http.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/dq/1.0/ruleTemplates/{name}/deployments", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if body != nil {
			_ = json.NewEncoder(w).Encode(body)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

// deploymentsPayload mirrors RuleTemplateDeploymentPaginated. The endpoint takes
// no paging parameters, so a response always carries the complete set.
func deploymentsPayload(n int) map[string]any {
	results := make([]map[string]any, 0, n)
	for i := range n {
		results = append(results, map[string]any{
			"jobName":          fmt.Sprintf("PUBLIC.TABLE_%d", i),
			"deployedRuleName": fmt.Sprintf("Row_Count_Range_col_%d", i),
			"columnName":       fmt.Sprintf("col_%d", i),
			"lastRunStatus":    "passing",
			"creator":          map[string]any{"userId": "u1", "userName": "steward"},
		})
	}
	return map[string]any{"results": results, "total": n, "offset": 0, "limit": n}
}

func call(t *testing.T, client *http.Client, in tools.Input) tools.Output {
	t.Helper()
	out, err := tools.NewTool(client).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return out
}

func TestMissingTemplateNameNeedsInput(t *testing.T) {
	out := call(t, newServer(t, http.StatusOK, deploymentsPayload(1)), tools.Input{TemplateName: "  "})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("status = %q, want needs_input", out.Status)
	}
	if out.Guidance == "" {
		t.Error("guidance is empty, want a pointer at list_data_quality_rule_templates")
	}
}

func TestDefaultsToTwentyFivePerPageAndReportsTotal(t *testing.T) {
	// AC: default to 25 and always tell the user the full count.
	out := call(t, newServer(t, http.StatusOK, deploymentsPayload(60)), tools.Input{TemplateName: templateName})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success", out.Status)
	}
	if got := len(out.Deployments.Results); got != 25 {
		t.Errorf("results = %d, want the default page of 25", got)
	}
	if out.Deployments.TotalDeployments != 60 {
		t.Errorf("totalDeployments = %d, want 60", out.Deployments.TotalDeployments)
	}
	if !strings.Contains(out.Message, "60") {
		t.Errorf("message = %q, want the full total in it", out.Message)
	}
	if !out.Deployments.HasMore {
		t.Error("hasMore = false, want true with 60 deployments on a page of 25")
	}
	if out.Deployments.TotalPages != 3 {
		t.Errorf("totalPages = %d, want 3", out.Deployments.TotalPages)
	}
}

func TestPromptsTheUserWhenMoreRemain(t *testing.T) {
	// AC: the user should be prompted about seeing the whole list or narrowing,
	// rather than the agent silently fetching everything.
	out := call(t, newServer(t, http.StatusOK, deploymentsPayload(60)), tools.Input{TemplateName: templateName})
	if out.Guidance == "" {
		t.Fatal("guidance is empty, want a prompt about the remaining deployments")
	}
	for _, want := range []string{"35", "Ask the user"} {
		if !strings.Contains(out.Guidance, want) {
			t.Errorf("guidance = %q, want it to contain %q", out.Guidance, want)
		}
	}
}

func TestPagingWalksTheSet(t *testing.T) {
	client := newServer(t, http.StatusOK, deploymentsPayload(60))
	out := call(t, client, tools.Input{TemplateName: templateName, Page: 3, PageSize: 25})
	if got := len(out.Deployments.Results); got != 10 {
		t.Errorf("results = %d, want the 10 remaining on page 3", got)
	}
	if out.Deployments.HasMore {
		t.Error("hasMore = true on the last page, want false")
	}
	if first := out.Deployments.Results[0].JobName; first != "PUBLIC.TABLE_50" {
		t.Errorf("first result = %q, want PUBLIC.TABLE_50", first)
	}
}

func TestPagePastTheEndIsSuccessNotError(t *testing.T) {
	out := call(t, newServer(t, http.StatusOK, deploymentsPayload(5)), tools.Input{TemplateName: templateName, Page: 9})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success", out.Status)
	}
	if len(out.Deployments.Results) != 0 {
		t.Errorf("results = %d, want empty past the end", len(out.Deployments.Results))
	}
	if !strings.Contains(out.Guidance, "between 1 and 1") {
		t.Errorf("guidance = %q, want it to name the valid page range", out.Guidance)
	}
}

func TestNoDeploymentsIsEmptySuccessNotError(t *testing.T) {
	// AC: a template with no deployments returns an empty list with a clear
	// indication, not an error.
	out := call(t, newServer(t, http.StatusOK, deploymentsPayload(0)), tools.Input{TemplateName: templateName})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success for a template with no deployments", out.Status)
	}
	if out.Deployments.Results == nil {
		t.Error("results is nil, want an empty array so the model sees an empty list")
	}
	if len(out.Deployments.Results) != 0 || out.Deployments.TotalDeployments != 0 {
		t.Errorf("results/total = %d/%d, want 0/0", len(out.Deployments.Results), out.Deployments.TotalDeployments)
	}
	if !strings.Contains(out.Message, "no deployed rules") {
		t.Errorf("message = %q, want it to say plainly that nothing is deployed", out.Message)
	}
}

func TestMapsFields(t *testing.T) {
	out := call(t, newServer(t, http.StatusOK, deploymentsPayload(1)), tools.Input{TemplateName: templateName})
	got := out.Deployments.Results[0]
	if got.JobName != "PUBLIC.TABLE_0" || got.DeployedRuleName != "Row_Count_Range_col_0" || got.ColumnName != "col_0" {
		t.Errorf("identity fields = %+v, want job/rule/column populated", got)
	}
	if got.LastRunStatus != "passing" {
		t.Errorf("lastRunStatus = %q, want passing", got.LastRunStatus)
	}
	if got.Creator != "steward" {
		t.Errorf("creator = %q, want the username flattened out of the UserReference", got.Creator)
	}
}

func TestNeverRunDeploymentOmitsRunFields(t *testing.T) {
	// lastRun and lastRunStatus are nullable: a freshly deployed rule has neither.
	payload := map[string]any{
		"results": []map[string]any{{
			"jobName": "PUBLIC.T", "deployedRuleName": "R", "lastRun": nil, "lastRunStatus": nil,
		}},
		"total": 1, "offset": 0, "limit": 1,
	}
	out := call(t, newServer(t, http.StatusOK, payload), tools.Input{TemplateName: templateName})
	got := out.Deployments.Results[0]
	if got.LastRun != "" || got.LastRunStatus != "" {
		t.Errorf("lastRun/lastRunStatus = %q/%q, want both empty for a rule that never ran", got.LastRun, got.LastRunStatus)
	}
}

func TestUnknownTemplateIsADescriptiveError(t *testing.T) {
	out := call(t, newServer(t, http.StatusNotFound, map[string]any{"message": "not found"}), tools.Input{TemplateName: "Nope"})
	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
	if !strings.Contains(out.Message, "Nope") {
		t.Errorf("message = %q, want the template name echoed back", out.Message)
	}
	if !strings.Contains(out.Guidance, "exactly") {
		t.Errorf("guidance = %q, want it to explain the name must match exactly", out.Guidance)
	}
}

func TestForbiddenNamesTheDeployPermission(t *testing.T) {
	// The read requires DEPLOY_TEMPLATES, which is a surprising scope for a list
	// call, so the guidance has to name it.
	out := call(t, newServer(t, http.StatusForbidden, map[string]any{"message": "denied"}), tools.Input{TemplateName: templateName})
	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
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
		{"bad request", http.StatusBadRequest},
		{"unprocessable", http.StatusUnprocessableEntity},
		{"server error", http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := call(t, newServer(t, tc.code, map[string]any{"message": "boom"}), tools.Input{TemplateName: templateName})
			if out.Status != tools.StatusError {
				t.Fatalf("status = %q, want error", out.Status)
			}
			if out.Message == "" || out.Guidance == "" {
				t.Errorf("message/guidance = %q/%q, want both populated", out.Message, out.Guidance)
			}
		})
	}
}

func TestRejectsOutOfRangePageSize(t *testing.T) {
	out := call(t, newServer(t, http.StatusOK, deploymentsPayload(1)), tools.Input{TemplateName: templateName, PageSize: 5000})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("status = %q, want needs_input", out.Status)
	}
	if !strings.Contains(out.Message, "200") {
		t.Errorf("message = %q, want it to state the maximum", out.Message)
	}
}

func TestTruncatesLongDownstreamError(t *testing.T) {
	leak := strings.Repeat("customer-row-data ", 60)
	out := call(t, newServer(t, http.StatusInternalServerError, map[string]any{"message": leak}), tools.Input{TemplateName: templateName})
	if len(out.Message) > 400 {
		t.Errorf("message length = %d, want the downstream body truncated", len(out.Message))
	}
	if !strings.Contains(out.Message, "truncated") {
		t.Errorf("message = %q, want the truncation marked", out.Message)
	}
}
