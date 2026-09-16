package update_dq_rule_template_test

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/update_dq_rule_template"
)

func ptrInt(v int) *int { return &v }

func TestUpdateChangesTolerance(t *testing.T) {
	client, state := newServer(t, http.StatusOK, storedTemplate(), http.StatusOK, updateResult(nil))

	out, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Name: templateName, Tolerance: ptrInt(9), Confirm: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if got := state.putBody["tolerance"]; got != float64(9) {
		t.Errorf("put tolerance = %v, want 9", got)
	}
	// Everything else must survive the full-replacement PUT.
	if got := state.putBody["sql"]; got != storedTemplate()["sql"] {
		t.Errorf("put sql = %v, want the stored SQL preserved", got)
	}
}

func TestUpdateToleranceIsReportedAsChanged(t *testing.T) {
	client, _ := newServer(t, http.StatusOK, storedTemplate(), http.StatusOK, updateResult(nil))

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: templateName, Tolerance: ptrInt(9)})
	if out.Status != tools.StatusPreview {
		t.Fatalf("status = %q, want preview (%s)", out.Status, out.Message)
	}
	if !slices.Contains(out.Preview.ChangedFields, "tolerance") {
		t.Errorf("changedFields = %v, want it to include tolerance", out.Preview.ChangedFields)
	}
	if out.Preview.Template.Tolerance == nil || *out.Preview.Template.Tolerance != 9 {
		t.Errorf("preview tolerance = %v, want 9", out.Preview.Template.Tolerance)
	}
}

// The stored tolerance is 7; supplying the same value changes nothing.
func TestUpdateSameToleranceIsNoChange(t *testing.T) {
	client, state := newServer(t, http.StatusOK, storedTemplate(), http.StatusOK, updateResult(nil))

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Name: templateName, Tolerance: ptrInt(7), Confirm: true,
	})
	if out.Status != tools.StatusValidationError {
		t.Fatalf("status = %q, want validation_error (%s)", out.Status, out.Message)
	}
	if state.putCalls != 0 {
		t.Errorf("put was called %d times with no effective change, want 0", state.putCalls)
	}
	if !strings.Contains(out.Guidance, "tolerance") {
		t.Errorf("guidance = %q, want tolerance listed among the changeable fields", out.Guidance)
	}
}

func TestUpdateRejectsNegativeTolerance(t *testing.T) {
	client, state := newServer(t, http.StatusOK, storedTemplate(), http.StatusOK, updateResult(nil))

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Name: templateName, Tolerance: ptrInt(-1), Confirm: true,
	})
	if out.Status != tools.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
	if !strings.Contains(out.Message, "tolerance") {
		t.Errorf("message = %q, want it to name tolerance", out.Message)
	}
	if state.putCalls != 0 {
		t.Errorf("put was called %d times with a negative tolerance, want 0", state.putCalls)
	}
}

// Setting tolerance to 0 on a template stored with 7 is a real change, not an
// omission.
func TestUpdateCanSetToleranceToZero(t *testing.T) {
	client, state := newServer(t, http.StatusOK, storedTemplate(), http.StatusOK, updateResult(nil))

	out, _ := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Name: templateName, Tolerance: ptrInt(0), Confirm: true,
	})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	got, present := state.putBody["tolerance"]
	if !present || got != float64(0) {
		t.Errorf("put tolerance = %v (present=%v), want an explicit 0", got, present)
	}
}
