package update_dq_job_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/testutil"
	tools "github.com/collibra/chip/pkg/tools/update_dq_job"
)

// handlers configures the mocked job endpoints. A nil handler means "must not be called" — the mux
// fails the test if that endpoint is hit, which is how the tests assert that the PATCH is NOT
// reached without confirm=true.
type handlers struct {
	get    func(w http.ResponseWriter, r *http.Request) // GET   /rest/dq/1.0/jobs/{jobName}
	update func(w http.ResponseWriter, r *http.Request) // PATCH /rest/dq/1.0/jobs/{jobName}
}

func newServer(t *testing.T, h handlers) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/dq/1.0/jobs/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			if h.update == nil {
				t.Errorf("unexpected update call: %s %s", r.Method, r.URL)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			h.update(w, r)
			return
		}
		if h.get == nil {
			t.Errorf("unexpected get call: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		h.get(w, r)
	})
	return httptest.NewServer(mux)
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

// captureBody records the PATCH body so tests can assert exactly which fields were sent — the
// central property of a partial update.
func captureBody(t *testing.T, got *map[string]any) func(http.ResponseWriter, *http.Request) {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		if err := json.Unmarshal(raw, got); err != nil {
			t.Errorf("failed to parse request body %q: %v", raw, err)
		}
		w.WriteHeader(http.StatusOK)
	}
}

// dqJob is the canonical existing job — a PUSHDOWN job with an active daily schedule, the default
// monitor set, and notifications, so a partial update has real values to be diffed against.
func dqJob() map[string]any {
	return map[string]any{
		"jobName": "sales.orders",
		"jobType": "PUSHDOWN",
		"dataLocation": map[string]any{
			"edgeSiteName":       "edge-eu",
			"edgeConnectionName": "snowflake-prod",
			"dataSourceName":     "SALES_DB",
			"schemaName":         "sales",
			"tableName":          "orders",
		},
		"sourceQuery": "SELECT * FROM sales.orders",
		"runDate":     map[string]any{"kind": "DATE", "value": "2025-10-22"},
		"jobSettings": map[string]any{
			"dateFormat":       "DATE",
			"pushdownSettings": map[string]any{"connections": 10, "threads": 2},
		},
		"monitoringSettings": map[string]any{
			"adaptiveMonitors": map[string]any{
				"rowCount": true, "nullValues": true, "emptyFields": true, "uniqueness": true,
				"settings": map[string]any{"dataLookBack": 10, "learningPhase": 4},
			},
		},
		"schedulingSettings": map[string]any{
			"schedulerMode":    "DAILY",
			"scheduledRunTime": "00:00:00",
			"isActive":         true,
			"daily":            map[string]any{"dailyOffset": "SCHEDULED", "daysOfWeek": []string{"MONDAY"}},
		},
	}
}

func pullupJob() map[string]any {
	job := dqJob()
	job["jobType"] = "PULLUP"
	job["jobSettings"] = map[string]any{"dateFormat": "DATE"}
	return job
}

func run(t *testing.T, server *httptest.Server, in tools.Input) tools.Output {
	t.Helper()
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("handler returned a Go error (should surface via Output): %v", err)
	}
	return out
}

func TestMissingJobNameNeedsInput(t *testing.T) {
	server := newServer(t, handlers{})
	defer server.Close()

	out := run(t, server, tools.Input{ScheduleRepeat: "DAILY"})
	if out.Status != "needs_input" {
		t.Fatalf("status = %q, want needs_input (output: %+v)", out.Status, out)
	}
	if !strings.Contains(out.Guidance, "jobName") {
		t.Errorf("guidance should name jobName, got %q", out.Guidance)
	}
}

func TestInvalidJobNameNeedsInput(t *testing.T) {
	server := newServer(t, handlers{})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "-bad name!", ScheduleRepeat: "DAILY"})
	if out.Status != "needs_input" {
		t.Fatalf("status = %q, want needs_input (output: %+v)", out.Status, out)
	}
	if !strings.Contains(out.Message, "not a valid") {
		t.Errorf("message should explain the name is invalid, got %q", out.Message)
	}
}

// A bare jobName is not an update request — it must be rejected before it costs an API call.
func TestNoChangesRequestedNeedsInput(t *testing.T) {
	server := newServer(t, handlers{})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders"})
	if out.Status != "needs_input" {
		t.Fatalf("status = %q, want needs_input (output: %+v)", out.Status, out)
	}
	if !strings.Contains(out.Message, "No changes") {
		t.Errorf("message should say no changes were requested, got %q", out.Message)
	}
}

func TestUnknownMonitorNeedsInput(t *testing.T) {
	server := newServer(t, handlers{})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", Monitors: []string{"rowCount", "nonsense"}})
	if out.Status != "needs_input" {
		t.Fatalf("status = %q, want needs_input (output: %+v)", out.Status, out)
	}
	if !strings.Contains(out.Message, "nonsense") {
		t.Errorf("message should name the unknown monitor, got %q", out.Message)
	}
}

func TestDescriptiveStatisticsRequiresAcknowledgement(t *testing.T) {
	server := newServer(t, handlers{})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", Monitors: []string{"rowCount", "descriptiveStatistics"}})
	if out.Status != "needs_input" {
		t.Fatalf("status = %q, want needs_input (output: %+v)", out.Status, out)
	}
	if !strings.Contains(out.Guidance, "acknowledgeDescriptiveStatistics") {
		t.Errorf("guidance should name the acknowledgement flag, got %q", out.Guidance)
	}
}

func TestInvalidScheduleNeedsInput(t *testing.T) {
	server := newServer(t, handlers{})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", ScheduleRepeat: "FORTNIGHTLY"})
	if out.Status != "needs_input" {
		t.Fatalf("status = %q, want needs_input (output: %+v)", out.Status, out)
	}
}

func TestDateFormatContradictingRunDateNeedsInput(t *testing.T) {
	server := newServer(t, handlers{})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", RunDate: "2025-11-01", DateFormat: "TIMESTAMP"})
	if out.Status != "needs_input" {
		t.Fatalf("status = %q, want needs_input (output: %+v)", out.Status, out)
	}
	if !strings.Contains(out.Message, "TIMESTAMP") {
		t.Errorf("message should explain the format mismatch, got %q", out.Message)
	}
}

// The safety checkpoint: confirm=false looks the job up and returns a diff, but the nil update
// handler means the test fails if the PATCH is reached.
func TestPreviewDoesNotUpdate(t *testing.T) {
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, dqJob())})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", ScheduleRepeat: "DAILY", ScheduleRunTime: "04:00"})
	if out.Status != "confirm_required" {
		t.Fatalf("status = %q, want confirm_required (output: %+v)", out.Status, out)
	}
	if out.Request == nil {
		t.Fatal("preview should carry the exact request")
	}
	if len(out.Changes) == 0 {
		t.Fatal("preview should carry a before/after diff")
	}
	if !strings.Contains(out.Guidance, "confirm=true") {
		t.Errorf("guidance should tell the caller to re-call with confirm=true, got %q", out.Guidance)
	}
	if out.JobDetailsLink == "" {
		t.Error("preview should carry the job details link")
	}
}

// The diff must show the schedule the job has now against the one it would get.
func TestPreviewDiffShowsCurrentAndProposed(t *testing.T) {
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, dqJob())})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", ScheduleRepeat: "HOURLY"})
	change := findChange(t, out, "schedule")
	if !strings.Contains(change.Current, "DAILY") {
		t.Errorf("current schedule should be the job's DAILY schedule, got %q", change.Current)
	}
	if !strings.Contains(change.Proposed, "HOURLY") {
		t.Errorf("proposed schedule should be HOURLY, got %q", change.Proposed)
	}
}

func TestConfirmSendsPatch(t *testing.T) {
	var body map[string]any
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, dqJob()), update: captureBody(t, &body)})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", ScheduleRepeat: "DAILY", ScheduleRunTime: "04:00", Confirm: true})
	if out.Status != "updated" {
		t.Fatalf("status = %q, want updated (output: %+v)", out.Status, out)
	}
	schedule, ok := body["schedulingSettings"].(map[string]any)
	if !ok {
		t.Fatalf("body should carry schedulingSettings, got %v", body)
	}
	if schedule["scheduledRunTime"] != "04:00:00" {
		t.Errorf("scheduledRunTime = %v, want 04:00:00", schedule["scheduledRunTime"])
	}
	if schedule["isActive"] != true {
		t.Errorf("a new schedule should be active, got %v", schedule["isActive"])
	}
}

// The core patch-semantics guarantee: a schedule-only update must not mention anything else, or the
// server would overwrite settings the caller never asked to change.
func TestPatchBodyOmitsUntouchedFields(t *testing.T) {
	var body map[string]any
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, dqJob()), update: captureBody(t, &body)})
	defer server.Close()

	run(t, server, tools.Input{JobName: "sales.orders", ScheduleRepeat: "DAILY", Confirm: true})

	for _, field := range []string{"sourceQuery", "runDate", "runDateEnd", "dataLocation", "jobSettings", "monitoringSettings", "notifications"} {
		if _, present := body[field]; present {
			t.Errorf("%q must be absent from a schedule-only patch, body = %v", field, body)
		}
	}
}

// dataLookback and learningPhase are pointers precisely so that setting one does not send 0 for the
// other — 0 is a meaningful value ("no learning phase"), not an absence.
func TestPatchOmitsUnsetAdaptiveSetting(t *testing.T) {
	var body map[string]any
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, dqJob()), update: captureBody(t, &body)})
	defer server.Close()

	run(t, server, tools.Input{JobName: "sales.orders", Monitors: []string{"rowCount"}, LearningPhase: 6, Confirm: true})

	monitoring := body["monitoringSettings"].(map[string]any)
	adaptive := monitoring["adaptiveMonitors"].(map[string]any)
	settings, ok := adaptive["settings"].(map[string]any)
	if !ok {
		t.Fatalf("adaptiveMonitors should carry settings, got %v", adaptive)
	}
	if settings["learningPhase"] != float64(6) {
		t.Errorf("learningPhase = %v, want 6", settings["learningPhase"])
	}
	if _, present := settings["dataLookBack"]; present {
		t.Errorf("dataLookBack must be absent when it was not set, got %v", settings)
	}
}

// The monitor selection is authoritative: monitors the caller left out are turned off.
func TestMonitorsAreAuthoritative(t *testing.T) {
	var body map[string]any
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, dqJob()), update: captureBody(t, &body)})
	defer server.Close()

	run(t, server, tools.Input{JobName: "sales.orders", Monitors: []string{"rowCount", "min"}, Confirm: true})

	adaptive := body["monitoringSettings"].(map[string]any)["adaptiveMonitors"].(map[string]any)
	if adaptive["rowCount"] != true || adaptive["min"] != true {
		t.Errorf("selected monitors should be on, got %v", adaptive)
	}
	if adaptive["nullValues"] != false || adaptive["uniqueness"] != false {
		t.Errorf("monitors omitted from the selection should be turned off, got %v", adaptive)
	}
}

// Changing the adaptive tuning alone cannot be expressed, because the toggles that must accompany it
// are authoritative — sending them without a selection would silently disable every monitor.
func TestAdaptiveSettingsWithoutMonitorsNeedsInput(t *testing.T) {
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, dqJob())})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", DataLookback: 20})
	if out.Status != "needs_input" {
		t.Fatalf("status = %q, want needs_input (output: %+v)", out.Status, out)
	}
	if !strings.Contains(out.Message, "monitors") {
		t.Errorf("message should tell the caller to pass monitors too, got %q", out.Message)
	}
}

// dataLocation is a whole-object replace, so the four untouched fields must be carried over from the
// job rather than sent empty.
func TestDataLocationOverlaysCurrentValues(t *testing.T) {
	var body map[string]any
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, dqJob()), update: captureBody(t, &body)})
	defer server.Close()

	run(t, server, tools.Input{JobName: "sales.orders", TableName: "orders_v2", Confirm: true})

	location := body["dataLocation"].(map[string]any)
	if location["tableName"] != "orders_v2" {
		t.Errorf("tableName = %v, want orders_v2", location["tableName"])
	}
	for field, want := range map[string]string{
		"edgeSiteName": "edge-eu", "edgeConnectionName": "snowflake-prod",
		"dataSourceName": "SALES_DB", "schemaName": "sales",
	} {
		if location[field] != want {
			t.Errorf("%s = %v, want %v (untouched location fields must be carried over)", field, location[field], want)
		}
	}
}

// Retargeting the table without rewriting the SQL leaves the scan pointing at the old table, which
// is a silent misconfiguration worth warning about.
func TestDataLocationChangeWarnsAboutSourceQuery(t *testing.T) {
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, dqJob())})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", TableName: "orders_v2"})
	if !containsSubstring(out.Warnings, "sourceQuery") {
		t.Errorf("warnings should flag that sourceQuery still names the old table, got %v", out.Warnings)
	}
}

// Nothing can be nulled, so NEVER resends the current schedule with isActive=false.
func TestScheduleNeverDisablesExistingSchedule(t *testing.T) {
	var body map[string]any
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, dqJob()), update: captureBody(t, &body)})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", ScheduleRepeat: "NEVER", Confirm: true})
	if out.Status != "updated" {
		t.Fatalf("status = %q, want updated (output: %+v)", out.Status, out)
	}
	schedule := body["schedulingSettings"].(map[string]any)
	if schedule["isActive"] != false {
		t.Errorf("isActive = %v, want false", schedule["isActive"])
	}
	if schedule["schedulerMode"] != "DAILY" {
		t.Errorf("the existing mode should be preserved when disabling, got %v", schedule["schedulerMode"])
	}
}

// NEVER on a job that has no schedule is a no-op, not an error — and it must not send a patch.
func TestScheduleNeverWithoutScheduleIsNoop(t *testing.T) {
	job := dqJob()
	delete(job, "schedulingSettings")
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, job)})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", ScheduleRepeat: "NEVER", Confirm: true})
	if out.Status != "needs_input" {
		t.Fatalf("status = %q, want needs_input (output: %+v)", out.Status, out)
	}
	if !containsSubstring(out.Warnings, "nothing to switch off") {
		t.Errorf("warnings should say there was no schedule to disable, got %v", out.Warnings)
	}
}

// An update that would change nothing must not be sent — the nil update handler enforces it.
func TestNoOpUpdateShortCircuits(t *testing.T) {
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, dqJob())})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", SourceQuery: "SELECT * FROM sales.orders", Confirm: true})
	if out.Status != "needs_input" {
		t.Fatalf("status = %q, want needs_input (output: %+v)", out.Status, out)
	}
	if !strings.Contains(out.Message, "already has") {
		t.Errorf("message should say the job already has this configuration, got %q", out.Message)
	}
}

func TestNotificationsReplacementIsWarned(t *testing.T) {
	job := dqJob()
	job["notifications"] = map[string]any{
		"notificationOptions": []any{map[string]any{"notificationType": "JOB_FAILED", "enabled": true}},
		"channels":            []any{map[string]any{"channel": "EMAIL", "recipients": []string{"existing"}}},
	}
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, job)})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", Notify: []string{"jobCompleted"}})
	if out.Status != "confirm_required" {
		t.Fatalf("status = %q, want confirm_required (output: %+v)", out.Status, out)
	}
	if !containsSubstring(out.Warnings, "replaced wholesale") {
		t.Errorf("warnings should flag that notifications are replaced, got %v", out.Warnings)
	}
}

// A job's type is immutable, so type-specific settings aimed at the wrong type are rejected rather
// than sent for the server to puzzle over.
func TestPullupSettingsOnPushdownJobRejected(t *testing.T) {
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, dqJob())})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", SizingMaxExecutors: 4})
	if out.Status != "needs_input" {
		t.Fatalf("status = %q, want needs_input (output: %+v)", out.Status, out)
	}
	if !strings.Contains(out.Message, "PULLUP") {
		t.Errorf("message should explain the setting is PULLUP-only, got %q", out.Message)
	}
}

func TestPushdownSettingsOnPullupJobRejected(t *testing.T) {
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, pullupJob())})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", PushdownConnections: 12})
	if out.Status != "needs_input" {
		t.Fatalf("status = %q, want needs_input (output: %+v)", out.Status, out)
	}
	if !strings.Contains(out.Message, "PUSHDOWN") {
		t.Errorf("message should explain the setting is PUSHDOWN-only, got %q", out.Message)
	}
}

// Changing one compute knob must not restate the other, whose create-side default (threads 2)
// differs from what this job might have.
func TestPushdownComputeSendsOnlyTheTouchedKnob(t *testing.T) {
	var body map[string]any
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, dqJob()), update: captureBody(t, &body)})
	defer server.Close()

	run(t, server, tools.Input{JobName: "sales.orders", PushdownConnections: 20, Confirm: true})

	pushdown := body["jobSettings"].(map[string]any)["pushdownSettings"].(map[string]any)
	if pushdown["connections"] != float64(20) {
		t.Errorf("connections = %v, want 20", pushdown["connections"])
	}
	if _, present := pushdown["threads"]; present {
		t.Errorf("threads must be absent when it was not set, got %v", pushdown)
	}
}

func TestPushdownConnectionsOutOfRangeRejected(t *testing.T) {
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, dqJob())})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", PushdownConnections: 99})
	if out.Status != "needs_input" {
		t.Fatalf("status = %q, want needs_input (output: %+v)", out.Status, out)
	}
	if !strings.Contains(out.Message, "1 and 50") {
		t.Errorf("message should state the valid range, got %q", out.Message)
	}
}

// numPartitions is a pointer because 0 means "let Spark decide" rather than "unset", so an explicit
// reset has to be expressible.
func TestNumPartitionsResetSendsZero(t *testing.T) {
	var body map[string]any
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, pullupJob()), update: captureBody(t, &body)})
	defer server.Close()

	run(t, server, tools.Input{JobName: "sales.orders", SizingNumPartitions: -1, Confirm: true})

	loadOptions := body["jobSettings"].(map[string]any)["pullupSettings"].(map[string]any)["loadOptions"].(map[string]any)
	if loadOptions["numPartitions"] != float64(0) {
		t.Errorf("numPartitions = %v, want an explicit 0", loadOptions["numPartitions"])
	}
}

func TestManualSizingWarnsAboutDefaults(t *testing.T) {
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, pullupJob())})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", SizingMaxExecutors: 4})
	if out.Status != "confirm_required" {
		t.Fatalf("status = %q, want confirm_required (output: %+v)", out.Status, out)
	}
	if !containsSubstring(out.Warnings, "MANUAL Spark sizing") {
		t.Errorf("warnings should flag the switch to manual sizing, got %v", out.Warnings)
	}
}

func TestParallelJdbcManualRequiresPartitionNumber(t *testing.T) {
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, pullupJob())})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", ParallelJdbcPartitionColumn: "id"})
	if out.Status != "needs_input" {
		t.Fatalf("status = %q, want needs_input (output: %+v)", out.Status, out)
	}
	if !strings.Contains(out.Message, "parallelJdbcPartitionNumber") {
		t.Errorf("message should name the missing field, got %q", out.Message)
	}
}

// runDate's kind is discriminated by the value's format, not asked for separately.
func TestRunDateKindInferredFromValue(t *testing.T) {
	var body map[string]any
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, dqJob()), update: captureBody(t, &body)})
	defer server.Close()

	run(t, server, tools.Input{JobName: "sales.orders", RunDate: "2025-11-01", RunDateEnd: "2025-11-02T00:00:00Z", Confirm: true})

	if kind := body["runDate"].(map[string]any)["kind"]; kind != "DATE" {
		t.Errorf("runDate kind = %v, want DATE", kind)
	}
	if kind := body["runDateEnd"].(map[string]any)["kind"]; kind != "TIMESTAMP" {
		t.Errorf("runDateEnd kind = %v, want TIMESTAMP", kind)
	}
}

func TestJobNameIsPathEscaped(t *testing.T) {
	var gotPath string
	server := newServer(t, handlers{
		get: func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.EscapedPath()
			jsonHandler(http.StatusOK, dqJob())(w, r)
		},
	})
	defer server.Close()

	run(t, server, tools.Input{JobName: "sales.orders_v1", ScheduleRepeat: "DAILY"})
	if !strings.HasSuffix(gotPath, "/rest/dq/1.0/jobs/sales.orders_v1") {
		t.Errorf("path = %q, want it to end with the escaped job name", gotPath)
	}
}

func TestLookupErrorMapping(t *testing.T) {
	cases := []struct {
		code      int
		wantInMsg string
	}{
		{http.StatusNotFound, "404"},
		{http.StatusUnauthorized, "401"},
		{http.StatusForbidden, "403"},
		{http.StatusBadRequest, "400"},
		{http.StatusInternalServerError, "500"},
	}
	for _, tc := range cases {
		t.Run(tc.wantInMsg, func(t *testing.T) {
			server := newServer(t, handlers{get: jsonHandler(tc.code, map[string]any{"userMessage": "nope"})})
			defer server.Close()

			out := run(t, server, tools.Input{JobName: "sales.orders", ScheduleRepeat: "DAILY"})
			if out.Status != "error" {
				t.Fatalf("status = %q, want error (output: %+v)", out.Status, out)
			}
			if !strings.Contains(out.Message, tc.wantInMsg) {
				t.Errorf("message %q should mention HTTP %s", out.Message, tc.wantInMsg)
			}
			if out.Guidance == "" {
				t.Error("every error branch should carry actionable guidance")
			}
		})
	}
}

func TestUpdateErrorMapping(t *testing.T) {
	cases := []struct {
		code      int
		wantInMsg string
	}{
		{http.StatusBadRequest, "400"},
		{http.StatusUnauthorized, "401"},
		{http.StatusForbidden, "403"},
		{http.StatusNotFound, "404"},
		{http.StatusInternalServerError, "500"},
	}
	for _, tc := range cases {
		t.Run(tc.wantInMsg, func(t *testing.T) {
			server := newServer(t, handlers{
				get:    jsonHandler(http.StatusOK, dqJob()),
				update: jsonHandler(tc.code, map[string]any{"userMessage": "nope"}),
			})
			defer server.Close()

			out := run(t, server, tools.Input{JobName: "sales.orders", ScheduleRepeat: "DAILY", Confirm: true})
			if out.Status != "error" {
				t.Fatalf("status = %q, want error (output: %+v)", out.Status, out)
			}
			if !strings.Contains(out.Message, tc.wantInMsg) {
				t.Errorf("message %q should mention HTTP %s", out.Message, tc.wantInMsg)
			}
			if out.Guidance == "" {
				t.Error("every error branch should carry actionable guidance")
			}
		})
	}
}

// The 403 guidance has to name the permission an administrator would have to grant.
func TestUpdateForbiddenNamesEditPermission(t *testing.T) {
	server := newServer(t, handlers{
		get:    jsonHandler(http.StatusOK, dqJob()),
		update: jsonHandler(http.StatusForbidden, nil),
	})
	defer server.Close()

	out := run(t, server, tools.Input{JobName: "sales.orders", ScheduleRepeat: "DAILY", Confirm: true})
	if !strings.Contains(out.Guidance, "DATA_QUALITY_JOB_EDIT") {
		t.Errorf("guidance should name DATA_QUALITY_JOB_EDIT, got %q", out.Guidance)
	}
}

func TestTransportError(t *testing.T) {
	server := newServer(t, handlers{get: jsonHandler(http.StatusOK, dqJob())})
	server.Close() // no listener: the lookup fails at the transport layer

	out := run(t, server, tools.Input{JobName: "sales.orders", ScheduleRepeat: "DAILY", Confirm: true})
	if out.Status != "error" {
		t.Fatalf("status = %q, want error (output: %+v)", out.Status, out)
	}
	if out.Guidance == "" {
		t.Error("the transport-error branch should carry actionable guidance")
	}
}

func findChange(t *testing.T, out tools.Output, field string) tools.FieldChange {
	t.Helper()
	for _, c := range out.Changes {
		if c.Field == field {
			return c
		}
	}
	t.Fatalf("no change for field %q in %+v", field, out.Changes)
	return tools.FieldChange{}
}

func containsSubstring(haystack []string, needle string) bool {
	for _, s := range haystack {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
