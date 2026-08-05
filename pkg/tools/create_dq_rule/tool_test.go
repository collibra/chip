package create_dq_rule_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/create_dq_rule"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// server boots an httptest server that captures the create-monitor request and
// echoes back a MonitorResponse. createCode overrides the success status.
func server(t *testing.T, createCode int, captured *clients.CreateDQRuleRequest) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/dq/internal/v1/monitoring/monitor", func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(captured); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		code := createCode
		if code == 0 {
			code = http.StatusOK
		}
		if code != http.StatusOK {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(clients.CreateDQRuleResponse{
			JobName:     captured.JobName,
			MonitorName: captured.MonitorName,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func TestCreateDQRule_HappyPath_DefaultsActive(t *testing.T) {
	var got clients.CreateDQRuleRequest
	c := server(t, http.StatusOK, &got)

	out, err := create_dq_rule.NewTool(c).Handler(t.Context(), create_dq_rule.Input{
		JobName:      "PUBLIC.SAMPLE_DATASET",
		MonitorName:  "Name_Not_Null",
		MonitorType:  "FREEFORM_SQL",
		MonitorValue: "SELECT * FROM @PUBLIC.SAMPLE_DATASET WHERE NAME IS NULL",
		Confirm:      true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != create_dq_rule.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if out.MonitorName != "Name_Not_Null" || out.JobName != "PUBLIC.SAMPLE_DATASET" {
		t.Fatalf("unexpected output: %+v", out)
	}
	if got.IsActive != 1 {
		t.Fatalf("isActive = %d, want 1 (default active)", got.IsActive)
	}
	if got.IsSuppressed {
		t.Fatalf("isSuppressed = true, want false (default)")
	}
}

func TestCreateDQRule_InactiveMapsToZero(t *testing.T) {
	var got clients.CreateDQRuleRequest
	c := server(t, http.StatusOK, &got)

	inactive := false
	_, err := create_dq_rule.NewTool(c).Handler(t.Context(), create_dq_rule.Input{
		JobName:      "DS",
		MonitorName:  "R",
		MonitorType:  "SIMPLE_SQL",
		MonitorValue: "NAME IS NOT NULL",
		ColumnName:   "NAME",
		Active:       &inactive,
		Confirm:      true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.IsActive != 0 {
		t.Fatalf("isActive = %d, want 0", got.IsActive)
	}
}

func TestCreateDQRule_InvalidMonitorType(t *testing.T) {
	var got clients.CreateDQRuleRequest
	c := server(t, http.StatusOK, &got)

	out, _ := create_dq_rule.NewTool(c).Handler(t.Context(), create_dq_rule.Input{
		JobName:      "DS",
		MonitorName:  "R",
		MonitorType:  "REGEX",
		MonitorValue: "x",
	})
	if out.Status != create_dq_rule.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestCreateDQRule_MissingRequiredFields(t *testing.T) {
	var got clients.CreateDQRuleRequest
	c := server(t, http.StatusOK, &got)

	out, _ := create_dq_rule.NewTool(c).Handler(t.Context(), create_dq_rule.Input{
		MonitorName:  "R",
		MonitorType:  "FREEFORM_SQL",
		MonitorValue: "x",
	})
	if out.Status != create_dq_rule.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestCreateDQRule_DownstreamErrorSurfaces(t *testing.T) {
	var got clients.CreateDQRuleRequest
	c := server(t, http.StatusUnprocessableEntity, &got)

	out, _ := create_dq_rule.NewTool(c).Handler(t.Context(), create_dq_rule.Input{
		JobName:      "DS",
		MonitorName:  "R",
		MonitorType:  "FREEFORM_SQL",
		MonitorValue: "x",
		Confirm:      true,
	})
	if out.Status != create_dq_rule.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}

func TestCreateDQRule_PreviewByDefault_CreatesNothing(t *testing.T) {
	var got clients.CreateDQRuleRequest
	c := server(t, http.StatusOK, &got)

	out, err := create_dq_rule.NewTool(c).Handler(t.Context(), create_dq_rule.Input{
		JobName:      "PUBLIC.DS",
		MonitorName:  "Name_Not_Null",
		MonitorType:  "FREEFORM_SQL",
		MonitorValue: "SELECT * FROM @PUBLIC.DS WHERE NAME IS NULL",
		// Confirm omitted -> preview
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != create_dq_rule.StatusPreview {
		t.Fatalf("status = %q, want preview", out.Status)
	}
	if out.Preview == nil || out.Preview.MonitorValue == "" {
		t.Fatalf("expected preview with SQL, got %+v", out.Preview)
	}
	// The DQ endpoint must not have been called (nothing written).
	if got.MonitorName != "" {
		t.Fatalf("expected no create request in preview mode, but server was called: %+v", got)
	}
}

func TestCreateDQRule_InvalidMonitorName(t *testing.T) {
	var got clients.CreateDQRuleRequest
	c := server(t, http.StatusOK, &got)

	out, _ := create_dq_rule.NewTool(c).Handler(t.Context(), create_dq_rule.Input{
		JobName:      "DS",
		MonitorName:  "bad name!", // space and '!' are not allowed
		MonitorType:  "FREEFORM_SQL",
		MonitorValue: "SELECT 1",
		Confirm:      true,
	})
	if out.Status != create_dq_rule.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
	if got.MonitorName != "" {
		t.Fatalf("expected no request for an invalid name")
	}
}

func TestCreateDQRule_SimpleSQLRequiresColumn(t *testing.T) {
	var got clients.CreateDQRuleRequest
	c := server(t, http.StatusOK, &got)

	out, _ := create_dq_rule.NewTool(c).Handler(t.Context(), create_dq_rule.Input{
		JobName:      "DS",
		MonitorName:  "R",
		MonitorType:  "SIMPLE_SQL",
		MonitorValue: "NAME IS NOT NULL",
		// columnName omitted
		Confirm: true,
	})
	if out.Status != create_dq_rule.StatusValidationError {
		t.Fatalf("status = %q, want validation_error (SIMPLE_SQL needs columnName)", out.Status)
	}
}
