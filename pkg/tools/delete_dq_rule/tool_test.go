package delete_dq_rule_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/tools/delete_dq_rule"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func server(t *testing.T, code int, path *string) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("DELETE /rest/dq/internal/v1/jobs/{jobName}/monitors/rules/{monitorName}", func(w http.ResponseWriter, r *http.Request) {
		if path != nil {
			*path = r.URL.Path
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

func TestDeleteDQRule_HappyPath(t *testing.T) {
	var path string
	c := server(t, http.StatusNoContent, &path)
	out, err := delete_dq_rule.NewTool(c).Handler(t.Context(), delete_dq_rule.Input{JobName: "PUBLIC.DS", MonitorName: "R"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != delete_dq_rule.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if path != "/rest/dq/internal/v1/jobs/PUBLIC.DS/monitors/rules/R" {
		t.Fatalf("unexpected path: %s", path)
	}
}

func TestDeleteDQRule_MissingInput(t *testing.T) {
	c := server(t, http.StatusNoContent, nil)
	out, _ := delete_dq_rule.NewTool(c).Handler(t.Context(), delete_dq_rule.Input{MonitorName: "R"})
	if out.Status != delete_dq_rule.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestDeleteDQRule_NotFound(t *testing.T) {
	c := server(t, http.StatusNotFound, nil)
	out, _ := delete_dq_rule.NewTool(c).Handler(t.Context(), delete_dq_rule.Input{JobName: "DS", MonitorName: "R"})
	if out.Status != delete_dq_rule.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}
