package create_dq_rule_template_test

import (
	"net/http"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/create_dq_rule_template"
)

func ptrInt(v int) *int { return &v }

func TestCreateSendsTolerance(t *testing.T) {
	client, state := newServer(t, http.StatusCreated, createdTemplate(), nil)

	input := validInput()
	input.Tolerance = ptrInt(5)
	input.Confirm = true
	out, err := tools.NewTool(client).Handler(t.Context(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if got := state.createdBody["tolerance"]; got != float64(5) {
		t.Errorf("posted tolerance = %v, want 5", got)
	}
}

// A tolerance of 0 is meaningful — one failing record fails the rule — so an
// explicit 0 must reach the service rather than being dropped as an empty value.
func TestCreateSendsExplicitZeroTolerance(t *testing.T) {
	client, state := newServer(t, http.StatusCreated, createdTemplate(), nil)

	input := validInput()
	input.Tolerance = ptrInt(0)
	input.Confirm = true
	if _, err := tools.NewTool(client).Handler(t.Context(), input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, present := state.createdBody["tolerance"]
	if !present {
		t.Fatalf("tolerance missing from the posted body %v, want an explicit 0", state.createdBody)
	}
	if got != float64(0) {
		t.Errorf("posted tolerance = %v, want 0", got)
	}
}

func TestCreateOmitsToleranceWhenNotSupplied(t *testing.T) {
	client, state := newServer(t, http.StatusCreated, createdTemplate(), nil)

	input := validInput()
	input.Confirm = true
	if _, err := tools.NewTool(client).Handler(t.Context(), input); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, present := state.createdBody["tolerance"]; present {
		t.Errorf("tolerance was posted as %v, want it omitted so the service default applies", state.createdBody["tolerance"])
	}
}

func TestCreateRejectsNegativeTolerance(t *testing.T) {
	client, state := newServer(t, http.StatusCreated, createdTemplate(), nil)

	input := validInput()
	input.Tolerance = ptrInt(-1)
	input.Confirm = true
	out, _ := tools.NewTool(client).Handler(t.Context(), input)
	if out.Status != tools.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
	if !strings.Contains(out.Message, "tolerance") {
		t.Errorf("message = %q, want it to name tolerance", out.Message)
	}
	if state.createCalls != 0 {
		t.Errorf("create was called %d times with a negative tolerance, want 0", state.createCalls)
	}
}

// The preview must echo every field that would be written.
func TestCreatePreviewEchoesTolerance(t *testing.T) {
	client, _ := newServer(t, http.StatusCreated, createdTemplate(), nil)

	input := validInput()
	input.Tolerance = ptrInt(5)
	out, _ := tools.NewTool(client).Handler(t.Context(), input)
	if out.Status != tools.StatusPreview {
		t.Fatalf("status = %q, want preview", out.Status)
	}
	if out.Preview == nil || out.Preview.Tolerance == nil || *out.Preview.Tolerance != 5 {
		t.Fatalf("preview tolerance = %v, want 5 echoed back", out.Preview)
	}
}
