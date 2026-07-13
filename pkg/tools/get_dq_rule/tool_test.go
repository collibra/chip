package get_dq_rule_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/get_dq_rule"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func server(t *testing.T, code int, rule clients.DQRule) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/dq/internal/v1/jobs/{jobName}/monitors/rules/{monitorName}", func(w http.ResponseWriter, r *http.Request) {
		if code != http.StatusOK {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(rule)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func TestGetDQRule_HappyPath(t *testing.T) {
	c := server(t, http.StatusOK, clients.DQRule{
		JobName: "PUBLIC.SAMPLE_DATASET", MonitorName: "Name_Not_Null",
		MonitorType: "FREEFORM_SQL", MonitorValue: "SELECT 1", IsActive: 1, IsSuppressed: false,
	})
	out, err := get_dq_rule.NewTool(c).Handler(t.Context(), get_dq_rule.Input{
		JobName: "PUBLIC.SAMPLE_DATASET", MonitorName: "Name_Not_Null",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != get_dq_rule.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if out.Rule == nil || out.Rule.MonitorName != "Name_Not_Null" || !out.Rule.Active {
		t.Fatalf("unexpected rule: %+v", out.Rule)
	}
}

func TestGetDQRule_MissingInput(t *testing.T) {
	c := server(t, http.StatusOK, clients.DQRule{})
	out, _ := get_dq_rule.NewTool(c).Handler(t.Context(), get_dq_rule.Input{JobName: "DS"})
	if out.Status != get_dq_rule.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestGetDQRule_NotFound(t *testing.T) {
	c := server(t, http.StatusNotFound, clients.DQRule{})
	out, _ := get_dq_rule.NewTool(c).Handler(t.Context(), get_dq_rule.Input{JobName: "DS", MonitorName: "R"})
	if out.Status != get_dq_rule.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}
