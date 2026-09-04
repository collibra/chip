package update_dq_rule_template_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/testutil"
	tools "github.com/collibra/chip/pkg/tools/update_dq_rule_template"
)

const templateName = "Row Count Range"

// server mocks the get + update endpoints. putBody captures what was PUT; it
// stays nil when the tool wrote nothing.
type server struct {
	putBody  map[string]any
	putCalls int
}

func newServer(t *testing.T, getCode int, stored any, putCode int, updateResult any) (*http.Client, *server) {
	t.Helper()
	state := &server{}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /rest/dq/1.0/ruleTemplates/{name}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(getCode)
		_ = json.NewEncoder(w).Encode(stored)
	})
	mux.HandleFunc("PUT /rest/dq/1.0/ruleTemplates/{name}", func(w http.ResponseWriter, r *http.Request) {
		state.putCalls++
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &state.putBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(putCode)
		_ = json.NewEncoder(w).Encode(updateResult)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv), state
}

func storedTemplate() map[string]any {
	return map[string]any{
		"id":                   "11111111-2222-3333-4444-555555555555",
		"ruleTemplateName":     templateName,
		"description":          "Stored description",
		"sql":                  "select * from @dataset where {{column}} is null",
		"dialect":              "snowflake",
		"dimensions":           []string{"Completeness"},
		"tolerance":            7,
		"isSystem":             false,
		"deployedRuleCount":    3,
		"businessRuleAssetIds": []string{"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"},
	}
}

func updateResult(deployments []map[string]any) map[string]any {
	return map[string]any{
		"ruleTemplate": storedTemplate(),
		"deployments":  deployments,
	}
}

func TestUpdateRequiresName(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, storedTemplate(), http.StatusOK, updateResult(nil))
	out, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: "  ", SQL: "select 1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestUpdateNotFound(t *testing.T) {
	client, state := newServer(t, http.StatusNotFound, map[string]any{"message": "nope"}, http.StatusOK, updateResult(nil))

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: templateName, SQL: "select 2", Confirm: true})
	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
	if !strings.Contains(out.Message, "No rule template named") {
		t.Errorf("message = %q, want a not-found explanation", out.Message)
	}
	if state.putCalls != 0 {
		t.Errorf("put was called %d times for a missing template, want 0", state.putCalls)
	}
}

func TestUpdateRefusesSystemTemplate(t *testing.T) {
	stored := storedTemplate()
	stored["isSystem"] = true
	client, state := newServer(t, http.StatusOK, stored, http.StatusOK, updateResult(nil))

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: templateName, SQL: "select 2", Confirm: true})
	if out.Status != tools.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
	if state.putCalls != 0 {
		t.Errorf("put was called %d times for an out-of-the-box template, want 0", state.putCalls)
	}
}

func TestUpdateWithNoChangesIsRejected(t *testing.T) {
	client, state := newServer(t, http.StatusOK, storedTemplate(), http.StatusOK, updateResult(nil))

	// Every supplied value equals the stored one, so nothing would change.
	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Name: templateName, Dialect: "snowflake", Confirm: true,
	})
	if out.Status != tools.StatusValidationError {
		t.Fatalf("status = %q, want validation_error (%s)", out.Status, out.Message)
	}
	if state.putCalls != 0 {
		t.Errorf("put was called %d times with no changes, want 0", state.putCalls)
	}
}

func TestUpdatePreviewShowsMergeAndBlastRadius(t *testing.T) {
	client, state := newServer(t, http.StatusOK, storedTemplate(), http.StatusOK, updateResult(nil))

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: templateName, SQL: "select 2"})
	if out.Status != tools.StatusPreview {
		t.Fatalf("status = %q, want preview (%s)", out.Status, out.Message)
	}
	if state.putCalls != 0 {
		t.Fatalf("put was called %d times without confirm, want 0", state.putCalls)
	}
	preview := out.Preview
	if preview == nil {
		t.Fatal("preview = nil, want the merged template")
	}
	if preview.DeployedRuleCount != 3 {
		t.Errorf("deployedRuleCount = %d, want 3", preview.DeployedRuleCount)
	}
	if len(preview.ChangedFields) != 1 || preview.ChangedFields[0] != "sql" {
		t.Errorf("changedFields = %v, want just sql", preview.ChangedFields)
	}
	// Untouched fields must be carried through, not blanked.
	if preview.Template.Description != "Stored description" || preview.Template.Dialect != "snowflake" {
		t.Errorf("merged template = %+v, want stored description and dialect preserved", preview.Template)
	}
	if preview.Template.Tolerance == nil || *preview.Template.Tolerance != 7 {
		t.Errorf("merged tolerance = %v, want the stored 7 carried through", preview.Template.Tolerance)
	}
}

// The API's update is a full-replacement PUT, so the merged payload must carry
// every stored field the caller did not change.
func TestUpdateSendsMergedPayload(t *testing.T) {
	client, state := newServer(t, http.StatusOK, storedTemplate(), http.StatusOK, updateResult(nil))

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: templateName, SQL: "select 2", Confirm: true})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if state.putCalls != 1 {
		t.Fatalf("put was called %d times, want 1", state.putCalls)
	}
	if got := state.putBody["sql"]; got != "select 2" {
		t.Errorf("put sql = %v, want the new value", got)
	}
	if got := state.putBody["description"]; got != "Stored description" {
		t.Errorf("put description = %v, want the stored value preserved", got)
	}
	if got := state.putBody["dialect"]; got != "snowflake" {
		t.Errorf("put dialect = %v, want the stored value preserved", got)
	}
	if got := state.putBody["tolerance"]; got != float64(7) {
		t.Errorf("put tolerance = %v, want the stored 7 preserved", got)
	}
	links, _ := state.putBody["businessRuleAssetIds"].([]any)
	if len(links) != 1 {
		t.Errorf("put businessRuleAssetIds = %v, want the stored link preserved", state.putBody["businessRuleAssetIds"])
	}
}

func TestUpdateReportsPartialCascade(t *testing.T) {
	deployments := []map[string]any{
		{"jobName": "PUBLIC.A", "status": "DEPLOYED", "deployedRuleName": "Row Count Range_amount"},
		{"jobName": "PUBLIC.B", "status": "SKIPPED", "reason": "no snowflake dialect for this job"},
	}
	client, _ := newServer(t, http.StatusOK, storedTemplate(), http.StatusOK, updateResult(deployments))

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: templateName, SQL: "select 2", Confirm: true})
	if out.Status != tools.StatusPartial {
		t.Fatalf("status = %q, want partial (%s)", out.Status, out.Message)
	}
	if out.Updated != 1 || out.Skipped != 1 {
		t.Errorf("updated/skipped = %d/%d, want 1/1", out.Updated, out.Skipped)
	}
	if len(out.Deployments) != 2 {
		t.Fatalf("deployments = %d, want 2", len(out.Deployments))
	}
	if out.Deployments[1].Reason == "" {
		t.Error("skipped deployment has no reason, want the API's explanation")
	}
}

func TestUpdateAllDeployedIsSuccess(t *testing.T) {
	deployments := []map[string]any{{"jobName": "PUBLIC.A", "status": "DEPLOYED"}}
	client, _ := newServer(t, http.StatusOK, storedTemplate(), http.StatusOK, updateResult(deployments))

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: templateName, SQL: "select 2", Confirm: true})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if out.Updated != 1 || out.Skipped != 0 {
		t.Errorf("updated/skipped = %d/%d, want 1/0", out.Updated, out.Skipped)
	}
}

func TestUpdateSurfacesAPIValidationFailure(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, storedTemplate(), http.StatusBadRequest, map[string]any{"message": "cannot translate sql"})

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: templateName, SQL: "select 2", Confirm: true})
	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
	if !strings.Contains(out.Message, "cannot translate sql") {
		t.Errorf("message = %q, want it to carry the API's validation detail", out.Message)
	}
}
