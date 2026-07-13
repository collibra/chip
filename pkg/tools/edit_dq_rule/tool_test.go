package edit_dq_rule_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/edit_dq_rule"
	"github.com/collibra/chip/pkg/tools/testutil"
)

type capture struct {
	newRuleName string
	body        clients.EditDQRuleRequest
}

func server(t *testing.T, code int, rec *capture) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /rest/dq/internal/v1/monitors/rules", func(w http.ResponseWriter, r *http.Request) {
		rec.newRuleName = r.URL.Query().Get("newRuleName")
		_ = json.NewDecoder(r.Body).Decode(&rec.body)
		if code != http.StatusOK {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		name := rec.body.MonitorName
		if rec.newRuleName != "" {
			name = rec.newRuleName
		}
		_ = json.NewEncoder(w).Encode(clients.CreateDQRuleResponse{JobName: rec.body.JobName, MonitorName: name})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func TestEditDQRule_HappyPath_Rename(t *testing.T) {
	var rec capture
	c := server(t, http.StatusOK, &rec)
	out, err := edit_dq_rule.NewTool(c).Handler(t.Context(), edit_dq_rule.Input{
		JobName: "DS", MonitorName: "Old", NewMonitorName: "New",
		MonitorType: "FREEFORM_SQL", MonitorValue: "SELECT 1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_dq_rule.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if rec.newRuleName != "New" {
		t.Fatalf("newRuleName query = %q, want New", rec.newRuleName)
	}
	if out.MonitorName != "New" {
		t.Fatalf("monitorName = %q, want New", out.MonitorName)
	}
}

func TestEditDQRule_InvalidMonitorType(t *testing.T) {
	var rec capture
	c := server(t, http.StatusOK, &rec)
	out, _ := edit_dq_rule.NewTool(c).Handler(t.Context(), edit_dq_rule.Input{
		JobName: "DS", MonitorName: "R", MonitorType: "REGEX", MonitorValue: "x",
	})
	if out.Status != edit_dq_rule.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestEditDQRule_DownstreamErrorSurfaces(t *testing.T) {
	var rec capture
	c := server(t, http.StatusNotFound, &rec)
	out, _ := edit_dq_rule.NewTool(c).Handler(t.Context(), edit_dq_rule.Input{
		JobName: "DS", MonitorName: "R", MonitorType: "FREEFORM_SQL", MonitorValue: "x",
	})
	if out.Status != edit_dq_rule.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}
