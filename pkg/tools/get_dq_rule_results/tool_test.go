package get_dq_rule_results_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/get_dq_rule_results"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func server(t *testing.T, code int, resp clients.DQRuleResults, sortOrder *string) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/dq/internal/v1/monitoring/rules/{jobName}/{ruleName}", func(w http.ResponseWriter, r *http.Request) {
		if sortOrder != nil {
			*sortOrder = r.URL.Query().Get("sortOrder")
		}
		if code != http.StatusOK {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func TestGetDQRuleResults_HappyPath_DefaultsToDesc(t *testing.T) {
	var sortOrder string
	c := server(t, http.StatusOK, clients.DQRuleResults{
		Dataset: "DS", RuleName: "R", RuleType: "SQLF", Total: 2,
		Results: []clients.DQRuleResultEntry{{RunDate: 1, Score: 100, PassFail: true, RuleStatus: "PASSING"}},
	}, &sortOrder)
	out, err := get_dq_rule_results.NewTool(c).Handler(t.Context(), get_dq_rule_results.Input{JobName: "DS", RuleName: "R"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != get_dq_rule_results.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if sortOrder != "DESC" {
		t.Fatalf("sortOrder = %q, want DESC (default)", sortOrder)
	}
	if len(out.Results) != 1 || out.Total != 2 {
		t.Fatalf("unexpected results: %+v total=%d", out.Results, out.Total)
	}
}

func TestGetDQRuleResults_InvalidSortOrder(t *testing.T) {
	c := server(t, http.StatusOK, clients.DQRuleResults{}, nil)
	out, _ := get_dq_rule_results.NewTool(c).Handler(t.Context(), get_dq_rule_results.Input{JobName: "DS", RuleName: "R", SortOrder: "SIDEWAYS"})
	if out.Status != get_dq_rule_results.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestGetDQRuleResults_MissingInput(t *testing.T) {
	c := server(t, http.StatusOK, clients.DQRuleResults{}, nil)
	out, _ := get_dq_rule_results.NewTool(c).Handler(t.Context(), get_dq_rule_results.Input{JobName: "DS"})
	if out.Status != get_dq_rule_results.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}
