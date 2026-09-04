package delete_dq_rule_template_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/delete_dq_rule_template"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const templateName = "Row Count Range"

// server mocks the get + delete endpoints. deleteQuery records the query the
// delete was called with; deleteCalls stays 0 when the tool deleted nothing.
type server struct {
	deleteQuery string
	deleteCalls int
}

func newServer(t *testing.T, getCode int, stored any, deleteCode int) (*http.Client, *server) {
	t.Helper()
	state := &server{}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /rest/dq/1.0/ruleTemplates/{name}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(getCode)
		_ = json.NewEncoder(w).Encode(stored)
	})
	mux.HandleFunc("DELETE /rest/dq/1.0/ruleTemplates/{name}", func(w http.ResponseWriter, r *http.Request) {
		state.deleteCalls++
		state.deleteQuery = r.URL.RawQuery
		w.WriteHeader(deleteCode)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv), state
}

func storedTemplate(deployedRuleCount int, isSystem bool) map[string]any {
	return map[string]any{
		"id":                "11111111-2222-3333-4444-555555555555",
		"ruleTemplateName":  templateName,
		"description":       "Row count within the learned range",
		"dialect":           "snowflake",
		"isSystem":          isSystem,
		"deployedRuleCount": deployedRuleCount,
	}
}

func TestDeleteRequiresName(t *testing.T) {
	client, state := newServer(t, http.StatusOK, storedTemplate(0, false), http.StatusNoContent)

	out, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: "   ", Confirm: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
	if state.deleteCalls != 0 {
		t.Errorf("delete was called %d times, want 0", state.deleteCalls)
	}
}

// The API's delete is idempotent (204 even for a missing template), so the tool
// checks existence itself to be able to report it.
func TestDeleteMissingTemplateIsReported(t *testing.T) {
	client, state := newServer(t, http.StatusNotFound, map[string]any{"message": "nope"}, http.StatusNoContent)

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: templateName, Confirm: true})
	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
	if !strings.Contains(out.Message, "No rule template named") {
		t.Errorf("message = %q, want a not-found explanation", out.Message)
	}
	if state.deleteCalls != 0 {
		t.Errorf("delete was called %d times for a missing template, want 0", state.deleteCalls)
	}
}

func TestDeleteRefusesSystemTemplate(t *testing.T) {
	client, state := newServer(t, http.StatusOK, storedTemplate(0, true), http.StatusNoContent)

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: templateName, Confirm: true})
	if out.Status != tools.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
	if state.deleteCalls != 0 {
		t.Errorf("delete was called %d times for an out-of-the-box template, want 0", state.deleteCalls)
	}
}

func TestDeleteWithoutCascadeRefusesLiveDeployments(t *testing.T) {
	client, state := newServer(t, http.StatusOK, storedTemplate(4, false), http.StatusNoContent)

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: templateName, Confirm: true})
	if out.Status != tools.StatusValidationError {
		t.Fatalf("status = %q, want validation_error (%s)", out.Status, out.Message)
	}
	if !strings.Contains(out.Message, "4 rule(s)") {
		t.Errorf("message = %q, want it to surface the count of affected rules", out.Message)
	}
	if !strings.Contains(out.Guidance, "cascade=true") {
		t.Errorf("guidance = %q, want it to point at cascade=true", out.Guidance)
	}
	if state.deleteCalls != 0 {
		t.Errorf("delete was called %d times, want 0", state.deleteCalls)
	}
}

func TestDeletePreviewWritesNothing(t *testing.T) {
	client, state := newServer(t, http.StatusOK, storedTemplate(2, false), http.StatusNoContent)

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: templateName, Cascade: true})
	if out.Status != tools.StatusPreview {
		t.Fatalf("status = %q, want preview (%s)", out.Status, out.Message)
	}
	if state.deleteCalls != 0 {
		t.Fatalf("delete was called %d times without confirm, want 0", state.deleteCalls)
	}
	if out.Preview == nil || out.Preview.DeployedRuleCount != 2 || !out.Preview.Cascade {
		t.Fatalf("preview = %+v, want the cascade plan with 2 deployed rules", out.Preview)
	}
	if !strings.Contains(out.Message, "2 rule(s)") {
		t.Errorf("message = %q, want it to state how many deployed rules would go", out.Message)
	}
}

func TestDeleteCascadeSendsDeleteDeployments(t *testing.T) {
	client, state := newServer(t, http.StatusOK, storedTemplate(2, false), http.StatusNoContent)

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: templateName, Cascade: true, Confirm: true})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if state.deleteCalls != 1 {
		t.Fatalf("delete was called %d times, want 1", state.deleteCalls)
	}
	if state.deleteQuery != "deleteDeployments=true" {
		t.Errorf("delete query = %q, want deleteDeployments=true", state.deleteQuery)
	}
	if out.Deleted == nil || out.Deleted.DeployedRuleCount != 2 {
		t.Errorf("deleted = %+v, want the plan that was carried out", out.Deleted)
	}
}

func TestDeleteWithoutDeploymentsOmitsCascadeFlag(t *testing.T) {
	client, state := newServer(t, http.StatusOK, storedTemplate(0, false), http.StatusNoContent)

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: templateName, Confirm: true})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if state.deleteQuery != "" {
		t.Errorf("delete query = %q, want no deleteDeployments flag when not cascading", state.deleteQuery)
	}
}

func TestDeleteSurfacesAPIRejection(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, storedTemplate(0, false), http.StatusBadRequest)

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: templateName, Confirm: true})
	if out.Status != tools.StatusValidationError && out.Status != tools.StatusError {
		t.Fatalf("status = %q, want the API rejection surfaced", out.Status)
	}
	if out.Guidance == "" {
		t.Error("guidance is empty, want actionable next steps")
	}
}
