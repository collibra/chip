package find_dq_rules_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/find_dq_rules"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func server(t *testing.T, code int, results []clients.DQMonitorSummary, total int64, captured *[]clients.DQMonitorFilter) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/dq/internal/v1/monitoring/monitors/dashboard", func(w http.ResponseWriter, r *http.Request) {
		if captured != nil {
			var body struct {
				Filters []clients.DQMonitorFilter `json:"filters"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			*captured = body.Filters
		}
		if code != http.StatusOK {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results, "total": total, "offset": 0, "limit": 25})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func TestFindDQRules_HappyPath_ColumnDuplicateCheck(t *testing.T) {
	var filters []clients.DQMonitorFilter
	c := server(t, http.StatusOK, []clients.DQMonitorSummary{
		{MonitorName: "email_not_null", JobName: "PUBLIC.USERS", ColumnName: "email", MonitorType: "SQLF", MonitorStatus: "PASSING"},
	}, 1, &filters)

	out, err := find_dq_rules.NewTool(c).Handler(t.Context(), find_dq_rules.Input{JobName: "PUBLIC.USERS", ColumnName: "email"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != find_dq_rules.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if len(out.Rules) != 1 || out.Rules[0].MonitorName != "email_not_null" {
		t.Fatalf("unexpected rules: %+v", out.Rules)
	}
	// Two EQUALS filters (JOB_NAME + COLUMN_NAME) should be sent.
	if len(filters) != 2 {
		t.Fatalf("expected 2 filters, got %+v", filters)
	}
	for _, f := range filters {
		if f.Operator != "EQUALS" {
			t.Fatalf("expected EQUALS operator, got %+v", f)
		}
	}
}

func TestFindDQRules_InvalidLimit(t *testing.T) {
	c := server(t, http.StatusOK, nil, 0, nil)
	out, _ := find_dq_rules.NewTool(c).Handler(t.Context(), find_dq_rules.Input{Limit: 1000})
	if out.Status != find_dq_rules.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestFindDQRules_DownstreamErrorSurfaces(t *testing.T) {
	c := server(t, http.StatusBadRequest, nil, 0, nil)
	out, _ := find_dq_rules.NewTool(c).Handler(t.Context(), find_dq_rules.Input{JobName: "DS"})
	if out.Status != find_dq_rules.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}
