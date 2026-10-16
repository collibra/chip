package update_dq_job_run_monitors_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/testutil"
	tools "github.com/collibra/chip/pkg/tools/update_dq_job_run_monitors"
)

const runID = "9c1d1e3a-2b4f-4a6c-8d90-111213141516"

// fakeRun mocks GET and PUT /rest/dq/1.0/jobRuns/{id}/monitors. Each GET serves
// the next entry of reads (the last one repeats); putCode is the PUT's status.
type fakeRun struct {
	t       *testing.T
	reads   []response
	getN    int
	putCode int
	putBody string
	puts    []putRequest
}

type response struct {
	code int
	body any
}

type putRequest struct {
	Operations []struct {
		MonitorName string `json:"monitorName"`
		Operation   string `json:"operation"`
	} `json:"operations"`
}

func newFakeRun(t *testing.T, reads ...response) *fakeRun {
	return &fakeRun{t: t, reads: reads, putCode: http.StatusNoContent}
}

func (f *fakeRun) client() *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/dq/1.0/jobRuns/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/dq/1.0/jobRuns/"+runID+"/monitors" {
			f.t.Errorf("path = %q, want the run's monitors", r.URL.Path)
		}
		switch r.Method {
		case http.MethodGet:
			f.serveGet(w)
		case http.MethodPut:
			f.servePut(w, r)
		default:
			f.t.Errorf("unexpected method %s", r.Method)
		}
	})
	srv := httptest.NewServer(mux)
	f.t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func (f *fakeRun) serveGet(w http.ResponseWriter) {
	if len(f.reads) == 0 {
		f.t.Error("unexpected monitors read")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	resp := f.reads[min(f.getN, len(f.reads)-1)]
	f.getN++
	writeJSON(w, resp.code, resp.body)
}

func (f *fakeRun) servePut(w http.ResponseWriter, r *http.Request) {
	var req putRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		f.t.Errorf("decoding PUT body: %v", err)
	}
	f.puts = append(f.puts, req)
	if f.putBody != "" {
		w.WriteHeader(f.putCode)
		_, _ = w.Write([]byte(f.putBody))
		return
	}
	w.WriteHeader(f.putCode)
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if body != nil {
		_ = json.NewEncoder(w).Encode(body)
	}
}

// runMonitors is shaped like the public JobRunMonitorsResult. states overrides
// the default state per monitor name.
func runMonitors(states map[string]string) response {
	state := func(name, def string) string {
		if s, ok := states[name]; ok {
			return s
		}
		return def
	}
	return response{code: http.StatusOK, body: map[string]any{
		"adaptiveMonitors": []map[string]any{
			{"monitorName": "ROW_COUNT", "monitorType": "ROW_COUNT", "state": state("ROW_COUNT", "BREAKING")},
			{"monitorName": "amount__NULL", "monitorType": "NULL", "primaryColumn": "amount", "state": state("amount__NULL", "PASSING")},
		},
		"customMonitors": []map[string]any{
			{"monitorName": "amount_is_positive", "state": state("amount_is_positive", "BREAKING"), "score": 40},
		},
	}}
}

func suppress(names ...string) []tools.Operation {
	ops := make([]tools.Operation, 0, len(names))
	for _, n := range names {
		ops = append(ops, tools.Operation{MonitorName: n, Action: "SUPPRESS"})
	}
	return ops
}

func call(t *testing.T, client *http.Client, input tools.Input) tools.Output {
	t.Helper()
	out, err := tools.NewTool(client).Handler(t.Context(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return out
}

func TestValidationRejectsBeforeAnyCall(t *testing.T) {
	tooMany := make([]tools.Operation, 501)
	for i := range tooMany {
		tooMany[i] = tools.Operation{MonitorName: fmt.Sprintf("m%d", i), Action: "SUPPRESS"}
	}
	for _, tc := range []struct {
		name     string
		input    tools.Input
		wantText string
	}{
		{"missing run id", tools.Input{RunID: "  ", Operations: suppress("ROW_COUNT")}, "run_id"},
		{"no operations", tools.Input{RunID: runID}, "at least one"},
		{"too many operations", tools.Input{RunID: runID, Operations: tooMany}, "at most 500"},
		{"missing monitor name", tools.Input{RunID: runID, Operations: []tools.Operation{{Action: "SUPPRESS"}}}, "operations[0].monitor_name is required"},
		{"malformed monitor name", tools.Input{RunID: runID, Operations: suppress("bad;name")}, "not a valid monitor name"},
		{"overlong monitor name", tools.Input{RunID: runID, Operations: suppress(strings.Repeat("a", 256))}, "at most 255"},
		{"missing action", tools.Input{RunID: runID, Operations: []tools.Operation{{MonitorName: "ROW_COUNT"}}}, "action is required"},
		{"pass is out of scope", tools.Input{RunID: runID, Operations: []tools.Operation{{MonitorName: "ROW_COUNT", Action: "PASS"}}}, `"PASS" is not supported`},
		{"unknown action", tools.Input{RunID: runID, Operations: []tools.Operation{{MonitorName: "ROW_COUNT", Action: "MUTE"}}}, `Use "SUPPRESS"`},
		{"duplicate monitor", tools.Input{RunID: runID, Operations: suppress("ROW_COUNT", "amount__NULL", "ROW_COUNT")}, "operations[2] repeats"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeRun(t)
			out := call(t, f.client(), tc.input)
			if out.Status != tools.StatusNeedsInput {
				t.Fatalf("status = %q, want needs_input (%s)", out.Status, out.Message)
			}
			if !strings.Contains(out.Message, tc.wantText) {
				t.Errorf("message = %q, want it to contain %q", out.Message, tc.wantText)
			}
			if out.Guidance == "" {
				t.Error("guidance is empty, want how to correct the call")
			}
			if f.getN != 0 || len(f.puts) != 0 {
				t.Errorf("made %d read(s) and %d write(s), want none", f.getN, len(f.puts))
			}
		})
	}
}

func TestPreviewWritesNothing(t *testing.T) {
	f := newFakeRun(t, runMonitors(nil))
	out := call(t, f.client(), tools.Input{RunID: runID, Operations: suppress("ROW_COUNT", "amount_is_positive")})

	if out.Status != tools.StatusPreview {
		t.Fatalf("status = %q, want preview (%s)", out.Status, out.Message)
	}
	if len(f.puts) != 0 {
		t.Fatalf("made %d write(s) in a preview, want none", len(f.puts))
	}
	if len(out.Operations) != 2 {
		t.Fatalf("operations = %d, want 2", len(out.Operations))
	}
	want := []tools.OperationResult{
		{MonitorName: "ROW_COUNT", MonitorKind: "adaptive", Action: "SUPPRESS", StateBefore: "BREAKING"},
		{MonitorName: "amount_is_positive", MonitorKind: "custom", Action: "SUPPRESS", StateBefore: "BREAKING"},
	}
	for i, w := range want {
		if out.Operations[i] != w {
			t.Errorf("operations[%d] = %+v, want %+v", i, out.Operations[i], w)
		}
	}
	if !strings.Contains(out.Guidance, "confirm=true") {
		t.Errorf("guidance = %q, want it to explain how to confirm", out.Guidance)
	}
}

func TestActionIsCaseInsensitive(t *testing.T) {
	f := newFakeRun(t, runMonitors(nil))
	out := call(t, f.client(), tools.Input{RunID: runID, Operations: []tools.Operation{{MonitorName: " ROW_COUNT ", Action: " suppress "}}})
	if out.Status != tools.StatusPreview {
		t.Fatalf("status = %q, want preview (%s)", out.Status, out.Message)
	}
	if got := out.Operations[0]; got.MonitorName != "ROW_COUNT" || got.Action != "SUPPRESS" {
		t.Errorf("operation = %+v, want the trimmed name and SUPPRESS", got)
	}
}

func TestUnknownMonitorsReportedByNameAndNothingWritten(t *testing.T) {
	f := newFakeRun(t, runMonitors(nil))
	out := call(t, f.client(), tools.Input{RunID: runID, Operations: suppress("ROW_COUNT", "row_count", "nope"), Confirm: true})

	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error (%s)", out.Status, out.Message)
	}
	if len(f.puts) != 0 {
		t.Fatalf("made %d write(s), want none when a monitor is unknown", len(f.puts))
	}
	if strings.Join(out.NotFound, ",") != "row_count,nope" {
		t.Errorf("notFound = %v, want [row_count nope] (names are case-sensitive)", out.NotFound)
	}
	for _, want := range []string{"ROW_COUNT", "amount__NULL", "amount_is_positive"} {
		if !strings.Contains(out.Guidance, want) {
			t.Errorf("guidance = %q, want it to list the run's monitor %q", out.Guidance, want)
		}
	}
}

func TestRunWithNoMonitorsExplainsWhy(t *testing.T) {
	f := newFakeRun(t, response{code: http.StatusOK, body: map[string]any{"adaptiveMonitors": []any{}, "customMonitors": []any{}}})
	out := call(t, f.client(), tools.Input{RunID: runID, Operations: suppress("ROW_COUNT")})
	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
	if !strings.Contains(out.Guidance, "no monitor results") {
		t.Errorf("guidance = %q, want it to say the run has no monitors", out.Guidance)
	}
}

func TestRunNotFound(t *testing.T) {
	f := newFakeRun(t, response{code: http.StatusNotFound, body: map[string]any{"message": "Job Run not found"}})
	out := call(t, f.client(), tools.Input{RunID: runID, Operations: suppress("ROW_COUNT"), Confirm: true})
	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
	if !strings.Contains(out.Message, "No job run with id") {
		t.Errorf("message = %q, want it to say the run was not found", out.Message)
	}
	if len(f.puts) != 0 {
		t.Errorf("made %d write(s), want none", len(f.puts))
	}
}

func TestConfirmAppliesAllOperationsInOneRequest(t *testing.T) {
	f := newFakeRun(t, runMonitors(nil), runMonitors(map[string]string{"ROW_COUNT": "SUPPRESSED", "amount_is_positive": "SUPPRESSED"}))
	out := call(t, f.client(), tools.Input{RunID: runID, Operations: suppress("ROW_COUNT", "amount_is_positive"), Confirm: true})

	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if len(f.puts) != 1 {
		t.Fatalf("made %d write(s), want exactly one bulk request", len(f.puts))
	}
	sent := f.puts[0].Operations
	if len(sent) != 2 || sent[0].MonitorName != "ROW_COUNT" || sent[1].MonitorName != "amount_is_positive" {
		t.Fatalf("sent operations = %+v, want ROW_COUNT and amount_is_positive in order", sent)
	}
	for _, op := range sent {
		if op.Operation != "SUPPRESS" {
			t.Errorf("sent operation = %q, want SUPPRESS", op.Operation)
		}
	}
	for _, r := range out.Operations {
		if !r.Applied || !r.Verified || r.StateBefore != "BREAKING" || r.StateAfter != "SUPPRESSED" {
			t.Errorf("result = %+v, want applied+verified, BREAKING -> SUPPRESSED", r)
		}
	}
	if out.Guidance != "" {
		t.Errorf("guidance = %q, want none when every monitor verified", out.Guidance)
	}
}

func TestAdaptiveSuppressionFlagCountsAsSuppressed(t *testing.T) {
	after := runMonitors(nil)
	after.body.(map[string]any)["adaptiveMonitors"].([]map[string]any)[0]["isSuppressed"] = true
	f := newFakeRun(t, runMonitors(nil), after)
	out := call(t, f.client(), tools.Input{RunID: runID, Operations: suppress("ROW_COUNT"), Confirm: true})
	if r := out.Operations[0]; !r.Verified || r.StateAfter != "SUPPRESSED" {
		t.Errorf("result = %+v, want isSuppressed to verify as SUPPRESSED", r)
	}
}

func TestReadBackMismatchNamesUnverifiedMonitors(t *testing.T) {
	f := newFakeRun(t, runMonitors(nil), runMonitors(map[string]string{"ROW_COUNT": "SUPPRESSED"}))
	out := call(t, f.client(), tools.Input{RunID: runID, Operations: suppress("ROW_COUNT", "amount_is_positive"), Confirm: true})

	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if !out.Operations[0].Verified {
		t.Errorf("ROW_COUNT = %+v, want verified", out.Operations[0])
	}
	if r := out.Operations[1]; !r.Applied || r.Verified || r.StateAfter != "BREAKING" {
		t.Errorf("amount_is_positive = %+v, want applied but not verified, still BREAKING", r)
	}
	if !strings.Contains(out.Guidance, "amount_is_positive") || strings.Contains(out.Guidance, "ROW_COUNT") {
		t.Errorf("guidance = %q, want it to name only the unverified monitor", out.Guidance)
	}
}

func TestReadBackFailureStillReportsSuccess(t *testing.T) {
	f := newFakeRun(t, runMonitors(nil), response{code: http.StatusInternalServerError, body: map[string]any{"message": "boom"}})
	out := call(t, f.client(), tools.Input{RunID: runID, Operations: suppress("ROW_COUNT"), Confirm: true})

	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success — the write committed (%s)", out.Status, out.Message)
	}
	if r := out.Operations[0]; !r.Applied || r.Verified || r.StateAfter != "" {
		t.Errorf("result = %+v, want applied, unverified, no stateAfter", r)
	}
	if !strings.Contains(out.Guidance, "does not affect the update") {
		t.Errorf("guidance = %q, want it to say the update still stands", out.Guidance)
	}
}

func TestUpdateErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		name      string
		code      int
		wantText  string
		unchanged bool
	}{
		{"not found", http.StatusNotFound, "404", true},
		{"unauthorized", http.StatusUnauthorized, "401", true},
		{"forbidden", http.StatusForbidden, "403", true},
		{"bad request", http.StatusBadRequest, "400", true},
		{"unprocessable", http.StatusUnprocessableEntity, "422", true},
		{"server error", http.StatusInternalServerError, "500", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFakeRun(t, runMonitors(nil))
			f.putCode = tc.code
			f.putBody = `{"message":"boom"}`
			out := call(t, f.client(), tools.Input{RunID: runID, Operations: suppress("ROW_COUNT"), Confirm: true})

			if out.Status != tools.StatusError {
				t.Fatalf("status = %q, want error", out.Status)
			}
			if !strings.Contains(out.Message, tc.wantText) {
				t.Errorf("message = %q, want it to mention %s", out.Message, tc.wantText)
			}
			if got := strings.Contains(out.Message, "Nothing was changed"); got != tc.unchanged {
				t.Errorf("message = %q, 'Nothing was changed' present = %v, want %v", out.Message, got, tc.unchanged)
			}
			if out.Guidance == "" {
				t.Error("guidance is empty")
			}
		})
	}
}

func TestUpdateErrorTextIsBounded(t *testing.T) {
	f := newFakeRun(t, runMonitors(nil))
	f.putCode = http.StatusBadRequest
	f.putBody = strings.Repeat("x", 5000)
	out := call(t, f.client(), tools.Input{RunID: runID, Operations: suppress("ROW_COUNT"), Confirm: true})
	if len(out.Message) > 600 {
		t.Errorf("message is %d characters, want the downstream text truncated", len(out.Message))
	}
	if !strings.Contains(out.Message, "(truncated)") {
		t.Errorf("message = %q, want a truncation marker", out.Message)
	}
}
