package get_dq_job_run_profile_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/get_dq_job_run_profile"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const runID = "a1b2c3d4-e5f6-7890-abcd-ef1234567890"

// newServer mocks GET /rest/dq/1.0/jobRuns/{id}/profile and records the query it was
// called with. A nil handler means "must not be called".
func newServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *string) {
	t.Helper()
	var gotQuery string
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/dq/1.0/jobRuns/", func(w http.ResponseWriter, r *http.Request) {
		if handler == nil {
			t.Errorf("unexpected profile call: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		gotQuery = r.URL.RawQuery
		handler(w, r)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &gotQuery
}

func jsonHandler(code int, body any) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if body != nil {
			_ = json.NewEncoder(w).Encode(body)
		}
	}
}

// profilePage is a payload shaped like the public API's JobRunProfileResults.
func profilePage(total int, columns ...map[string]any) map[string]any {
	return map[string]any{
		"jobRunId": runID,
		"jobName":  "customer_orders",
		"runDate":  map[string]any{"kind": "TIMESTAMP", "value": "2024-10-22T00:00:00Z"},
		"offset":   0,
		"limit":    100,
		"total":    total,
		"results":  columns,
	}
}

func orderIDColumn() map[string]any {
	return map[string]any{
		"columnName":  "order_id",
		"definedType": "BIGINT",
		"valueCount":  150000,
		"nullCount":   0,
		"emptyCount":  0,
		"uniqueCount": 150000,
		"min":         "1000",
		"max":         "999999",
		"topShapes": []map[string]any{
			{"pattern": "######", "count": 148500, "percentage": 99.0},
		},
	}
}

func amountColumn() map[string]any {
	return map[string]any{
		"columnName":   "amount",
		"definedType":  "DECIMAL",
		"inferredType": "String, Double",
		"valueCount":   149880,
		"nullCount":    120,
		"emptyCount":   0,
		"uniqueCount":  4320,
		"min":          "0.99",
		"max":          "9999.99",
		"median":       "47.5",
		"q1":           "12.99",
		"q3":           "199.0",
		"topShapes":    nil,
	}
}

func TestMissingRunIDNeedsInput(t *testing.T) {
	srv, _ := newServer(t, nil)
	out, err := tools.NewTool(testutil.NewClient(srv)).Handler(t.Context(), tools.Input{RunID: "  "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("status = %q, want needs_input (%s)", out.Status, out.Message)
	}
	if out.Guidance == "" {
		t.Error("guidance is empty, want an explanation of what to supply")
	}
}

func TestProfileSuccess(t *testing.T) {
	srv, gotQuery := newServer(t, jsonHandler(http.StatusOK, profilePage(2, orderIDColumn(), amountColumn())))

	out, err := tools.NewTool(testutil.NewClient(srv)).Handler(t.Context(), tools.Input{RunID: runID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if *gotQuery != "includeTotal=true&limit=100" {
		t.Errorf("query = %q, want the default limit and includeTotal", *gotQuery)
	}

	profile := out.Profile
	if profile == nil {
		t.Fatal("profile = nil, want the returned page")
	}
	if profile.JobName != "customer_orders" || profile.RunDate != "2024-10-22T00:00:00Z" {
		t.Errorf("job/runDate = %q/%q, want customer_orders/2024-10-22T00:00:00Z", profile.JobName, profile.RunDate)
	}
	if profile.JobDetails == "" {
		t.Error("jobDetailsLink is empty, want a deep link when jobName is known")
	}
	if profile.Total == nil || *profile.Total != 2 {
		t.Errorf("total = %v, want 2", profile.Total)
	}
	if profile.HasMore {
		t.Error("hasMore = true, want false when the page covers every column")
	}
	if len(profile.Columns) != 2 {
		t.Fatalf("columns = %d, want 2", len(profile.Columns))
	}

	orderID := profile.Columns[0]
	if orderID.ColumnName != "order_id" || orderID.DefinedType != "BIGINT" {
		t.Errorf("order_id = %+v, want name order_id and type BIGINT", orderID)
	}
	if orderID.NullPercent != 0 {
		t.Errorf("order_id nullPercent = %v, want 0", orderID.NullPercent)
	}
	if len(orderID.TopShapes) != 1 || orderID.TopShapes[0].Pattern != "######" {
		t.Errorf("order_id topShapes = %+v, want the ###### shape", orderID.TopShapes)
	}

	amount := profile.Columns[1]
	if amount.InferredType != "String, Double" {
		t.Errorf("amount inferredType = %q, want the mixed-type list", amount.InferredType)
	}
	// 120 nulls out of 150000 observed rows.
	if amount.NullPercent != 0.08 {
		t.Errorf("amount nullPercent = %v, want 0.08", amount.NullPercent)
	}
	if amount.Median != "47.5" || amount.Q1 != "12.99" || amount.Q3 != "199.0" {
		t.Errorf("amount quartiles = %q/%q/%q, want 12.99/47.5/199.0", amount.Q1, amount.Median, amount.Q3)
	}
	if amount.TopShapes != nil {
		t.Errorf("amount topShapes = %+v, want nil when shape analysis did not run", amount.TopShapes)
	}
}

func TestProfilePaginationPassesLimitAndOffsetAndFlagsMore(t *testing.T) {
	page := profilePage(35, orderIDColumn())
	page["offset"] = 10
	page["limit"] = 1
	srv, gotQuery := newServer(t, jsonHandler(http.StatusOK, page))

	out, _ := tools.NewTool(testutil.NewClient(srv)).Handler(t.Context(), tools.Input{RunID: runID, Limit: 1, Offset: 10})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if *gotQuery != "includeTotal=true&limit=1&offset=10" {
		t.Errorf("query = %q, want limit=1 and offset=10", *gotQuery)
	}
	if !out.Profile.HasMore {
		t.Error("hasMore = false, want true when 11 of 35 columns have been seen")
	}
}

func TestProfileRejectsOutOfRangeInputs(t *testing.T) {
	srv, _ := newServer(t, nil)
	tool := tools.NewTool(testutil.NewClient(srv))

	for name, input := range map[string]tools.Input{
		"limit above the API cap": {RunID: runID, Limit: 501},
		"negative limit":          {RunID: runID, Limit: -1},
		"negative offset":         {RunID: runID, Offset: -1},
	} {
		out, _ := tool.Handler(t.Context(), input)
		if out.Status != tools.StatusNeedsInput {
			t.Errorf("%s: status = %q, want needs_input", name, out.Status)
		}
	}
}

func TestProfileNoResultsExplainsWhy(t *testing.T) {
	srv, _ := newServer(t, jsonHandler(http.StatusOK, profilePage(0)))

	out, _ := tools.NewTool(testutil.NewClient(srv)).Handler(t.Context(), tools.Input{RunID: runID})
	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
	if out.Guidance == "" {
		t.Fatal("guidance is empty, want an explanation of why there is no profile")
	}
	for _, want := range []string{"did not complete", "profiling is not configured", "dq_get_job_run"} {
		if !strings.Contains(out.Guidance, want) {
			t.Errorf("guidance = %q, want it to mention %q", out.Guidance, want)
		}
	}
}

func TestProfileEmptyPagePastEndPointsAtOffset(t *testing.T) {
	page := profilePage(35)
	page["offset"] = 999
	srv, _ := newServer(t, jsonHandler(http.StatusOK, page))

	out, _ := tools.NewTool(testutil.NewClient(srv)).Handler(t.Context(), tools.Input{RunID: runID, Offset: 999})
	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
	if !strings.Contains(out.Guidance, "smaller offset") {
		t.Errorf("guidance = %q, want it to suggest a smaller offset", out.Guidance)
	}
}

func TestProfileLookupErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		name     string
		code     int
		wantText string
	}{
		{"not found", http.StatusNotFound, "404"},
		{"unauthorized", http.StatusUnauthorized, "401"},
		{"forbidden", http.StatusForbidden, "403"},
		{"bad request", http.StatusBadRequest, "400"},
		{"server error", http.StatusInternalServerError, "500"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newServer(t, jsonHandler(tc.code, map[string]any{"message": "boom"}))
			out, err := tools.NewTool(testutil.NewClient(srv)).Handler(t.Context(), tools.Input{RunID: runID})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.Status != tools.StatusError {
				t.Fatalf("status = %q, want error", out.Status)
			}
			if !strings.Contains(out.Message, tc.wantText) {
				t.Errorf("message = %q, want it to report HTTP %s", out.Message, tc.wantText)
			}
			if out.Guidance == "" {
				t.Error("guidance is empty, want actionable next steps")
			}
		})
	}
}

func TestProfileTransportError(t *testing.T) {
	srv, _ := newServer(t, jsonHandler(http.StatusOK, nil))
	client := testutil.NewClient(srv)
	srv.Close()

	out, err := tools.NewTool(client).Handler(t.Context(), tools.Input{RunID: runID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
	if !strings.Contains(out.Guidance, "Retry") {
		t.Errorf("guidance = %q, want a retry suggestion", out.Guidance)
	}
}
