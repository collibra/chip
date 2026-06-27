package create_dq_job_test

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/create_dq_job"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// muxWithPerms returns the base connections mux plus the GraphQL permissions endpoint, reporting the
// given permission identifiers (e.g. DATA_QUALITY_JOB_CREATE) on the CONNECTION resource — the scope
// the DQ UI checks. Returned under `resource` to exercise the resource-aware preflight.
func muxWithPerms(perms ...string) *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	mux.HandleFunc("/graphql", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		quoted := make([]string, 0, len(perms))
		for _, p := range perms {
			quoted = append(quoted, strconv.Quote(p))
		}
		_, _ = w.Write([]byte(`{"data":{"api":{"currentUser":{"global":[],"resource":[` + strings.Join(quoted, ",") + `]}}}}`))
	})
	return mux
}

// connectionsHandler mocks the INTERNAL connections list create_dq_job uses to resolve the connection
// name + databaseProductName (and jobType from capabilities) for the public create request.
func connectionsHandler() http.Handler {
	return testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{
			"results": []map[string]any{{
				"connectionId":        "conn-1",
				"connectionName":      "POSTGRES-SOURCE",
				"capabilityTypes":     []string{"PULLUP"},
				"databaseProductName": "POSTGRES",
				"edgeSiteId":          "site-1",
				"edgeSiteName":        "EDGE-1",
			}},
		}
	})
}

// createdJobHandler mocks the PUBLIC create POST /rest/dq/1.0/jobs, echoing a job name + queued run id
// (JobDefinitionCreateResponse).
func createdJobHandler() http.Handler {
	return testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		if r.Method != http.MethodPost {
			return http.StatusMethodNotAllowed, map[string]any{}
		}
		return http.StatusCreated, map[string]any{"jobName": "sales.transactions", "jobType": "PULLUP", "jobRunId": "run-1"}
	})
}

func baseInput() tools.Input {
	return tools.Input{
		EdgeSiteName:       "EDGE-1",
		EdgeConnectionName: "POSTGRES-SOURCE",
		DataSourceName:     "postgres",
		SchemaName:         "sales",
		TableName:          "transactions",
		JobType:            "PULLUP",
	}
}

func TestPreviewDoesNotCreate(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	mux.HandleFunc("/rest/dq/1.0/jobs", func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("create endpoint must NOT be called during preview")
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusPreview {
		t.Fatalf("expected preview, got %q (%s)", out.Status, out.Message)
	}
	if out.Request == nil {
		t.Fatalf("expected a preview request")
	}
	// The public body uses dataLocation with NAMES (no connectionId/edgeSiteId).
	dl := out.Request.DataLocation
	if dl.EdgeConnectionName != "POSTGRES-SOURCE" || dl.EdgeSiteName != "EDGE-1" || dl.DatabaseProductName != "POSTGRES" {
		t.Errorf("dataLocation not resolved into request: %+v", dl)
	}
	if out.Request.JobType != "PULLUP" || out.Request.JobName != "sales.transactions" {
		t.Errorf("unexpected request: jobType=%s jobName=%s", out.Request.JobType, out.Request.JobName)
	}
	// PULLUP defaults to automatic sizing: pullupSettings present, sparkJobSizing omitted (nil).
	if out.Request.JobSettings == nil || out.Request.JobSettings.PullupSettings == nil {
		t.Fatalf("PULLUP request missing jobSettings.pullupSettings: %+v", out.Request.JobSettings)
	}
	if out.Request.JobSettings.PullupSettings.SparkJobSizing != nil {
		t.Errorf("expected automatic sizing (nil sparkJobSizing) by default, got %+v", out.Request.JobSettings.PullupSettings.SparkJobSizing)
	}
	if out.JobID != "" {
		t.Errorf("expected no jobId on preview, got %q", out.JobID)
	}
	// No selected columns / filter / sample -> a plain whole-table scan.
	q := out.Request.SourceQuery
	if !strings.HasPrefix(q, "SELECT * FROM") || strings.Contains(q, "RANDOM()") || strings.Contains(q, "${rd}") {
		t.Errorf("expected a plain SELECT * scan, got %q", q)
	}
	if len(out.UnsupportedOptions) == 0 {
		t.Errorf("preview should disclose still-unsupported wizard options")
	}
}

func TestRowFilterInlinedInQuery(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.FilterColumn = "amount"
	in.FilterOperator = ">"
	in.FilterValue = "100"
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The filter must be INLINED into the source query WHERE (value single-quoted, per the wizard).
	if !strings.Contains(out.Request.SourceQuery, `"amount" > '100'`) {
		t.Errorf("expected filter inlined into sourceQuery WHERE, got %q", out.Request.SourceQuery)
	}
}

func TestRowFilterAndSliceCombineInQuery(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.FilterColumn = "is_active"
	in.FilterOperator = "="
	in.FilterValue = "true"
	in.TimeSliceColumn = "txn_ts"
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	q := out.Request.SourceQuery
	// Filter AND slice predicates must both be present, ANDed together (filter value single-quoted).
	if !strings.Contains(q, `"is_active" = 'true'`) {
		t.Errorf("filter predicate missing from query: %q", q)
	}
	if !strings.Contains(q, `"txn_ts" >= '${rd}'`) || !strings.Contains(q, `"txn_ts" < '${rdEnd}'`) {
		t.Errorf("slice predicate missing from query: %q", q)
	}
	if !strings.Contains(q, " AND ") {
		t.Errorf("expected predicates ANDed together: %q", q)
	}
}

func TestRowFilterRequiresOperator(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.FilterColumn = "amount" // operator omitted -> incomplete filter
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input when filterColumn set without operator, got %q (%s)", out.Status, out.Message)
	}
}

func TestMonitorsDefaultWhenOmitted(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	am := out.Request.MonitoringSettings.AdaptiveMonitors
	// Defaults: rowCount/nullValues/emptyFields/uniqueness ON; everything else OFF.
	if !am.RowCount || !am.NullValues || !am.EmptyFields || !am.Uniqueness {
		t.Errorf("expected the four default monitors ON, got %+v", am)
	}
	if am.Min || am.Mean || am.Max || am.ExecutionTime || am.DescriptiveStatistics {
		t.Errorf("expected non-default monitors OFF, got %+v", am)
	}
}

func TestMonitorsCustomSelection(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.Monitors = []string{"rowCount", "min"} // authoritative -> only these ON
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	am := out.Request.MonitoringSettings.AdaptiveMonitors
	if !am.RowCount || !am.Min {
		t.Errorf("expected rowCount+min ON, got %+v", am)
	}
	if am.NullValues || am.EmptyFields || am.Uniqueness || am.Mean || am.Max {
		t.Errorf("expected unselected default monitors turned OFF, got %+v", am)
	}
}

func TestUnknownMonitorNeedsInput(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.Monitors = []string{"rowCount", "bogusMonitor"}
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for unknown monitor, got %q (%s)", out.Status, out.Message)
	}
}

func TestDescriptiveStatisticsWarnsInPreview(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.Monitors = []string{"rowCount", "descriptiveStatistics"}
	in.AcknowledgeDescriptiveStatistics = true // required by the hard gate
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusPreview {
		t.Fatalf("expected preview, got %q", out.Status)
	}
	if !out.Request.MonitoringSettings.AdaptiveMonitors.DescriptiveStatistics {
		t.Errorf("expected descriptiveStatistics ON in request")
	}
	if !strings.Contains(out.Message, "UNMASKS") {
		t.Errorf("expected a sensitive-data warning in the preview message, got %q", out.Message)
	}
}

func TestDescriptiveStatisticsRequiresAcknowledgement(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.Monitors = []string{"rowCount", "descriptiveStatistics"} // no acknowledgement
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input (hard gate) for descriptiveStatistics without acknowledgement, got %q (%s)", out.Status, out.Message)
	}
	if !strings.Contains(out.Message, "sensitive") {
		t.Errorf("expected the sensitive-data warning, got %q", out.Message)
	}
}

func TestSuccessReturnsTableAssetLink(t *testing.T) {
	mux := muxWithPerms("DATA_QUALITY_JOB_CREATE", "DATA_QUALITY_JOB_SCHEDULE", "DATA_QUALITY_JOB_RUN")
	mux.Handle("/rest/dq/1.0/jobs", createdJobHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.Confirm = true
	in.TableAssetID = "019f062a-18dc-7489-a373-c928e59f1fc4"
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusCreated {
		t.Fatalf("expected created, got %q (%s)", out.Status, out.Message)
	}
	if out.TableAssetLink != "/asset/019f062a-18dc-7489-a373-c928e59f1fc4" {
		t.Errorf("expected catalog asset link, got %q", out.TableAssetLink)
	}
}

func TestTimeSliceGeneratesRdQuery(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.TimeSliceColumn = "txn_ts"
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	q := out.Request.SourceQuery
	// The slice predicate must be in the WHERE for the scheduler to substitute it per run.
	if !strings.Contains(q, `"txn_ts" >= '${rd}'`) || !strings.Contains(q, `"txn_ts" < '${rdEnd}'`) {
		t.Errorf("expected ${rd}/${rdEnd} predicate on the slice column, got query %q", q)
	}
	// An initial run-date window must be set so the immediate queued run has concrete ${rd}/${rdEnd}.
	if out.Request.RunDate == nil || out.Request.RunDateEnd == nil {
		t.Fatalf("expected runDate+runDateEnd window for the initial run, got runDate=%+v runDateEnd=%+v", out.Request.RunDate, out.Request.RunDateEnd)
	}
	if out.Request.RunDate.Kind != "DATE" || out.Request.RunDateEnd.Kind != "DATE" {
		t.Errorf("expected DATE-kind run dates for a DAYS slice, got %+v / %+v", out.Request.RunDate, out.Request.RunDateEnd)
	}
	// dateFormat must match the run-date kind.
	if out.Request.JobSettings.DateFormat != "DATE" {
		t.Errorf("expected jobSettings.dateFormat=DATE, got %q", out.Request.JobSettings.DateFormat)
	}
}

func TestScheduleDailyPopulatesSubObject(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.ScheduleRepeat = "DAILY"
	in.ScheduleRunTime = "01:30"
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := out.Request.SchedulingSettings
	if s == nil || s.SchedulerMode != "DAILY" || s.Daily == nil {
		t.Fatalf("expected a DAILY schedule with a daily sub-object, got %+v", s)
	}
	if len(s.Daily.DaysOfWeek) != 7 {
		t.Errorf("expected all 7 days for DAILY, got %v", s.Daily.DaysOfWeek)
	}
	if s.ScheduledRunTime != "01:30:00" {
		t.Errorf("expected HH:mm normalized to HH:mm:ss, got %q", s.ScheduledRunTime)
	}
	if s.Daily.DailyOffset != "SCHEDULED" {
		t.Errorf("expected default offset SCHEDULED, got %q", s.Daily.DailyOffset)
	}
}

func TestScheduleWeeklyWithDays(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.ScheduleRepeat = "WEEKLY"
	in.ScheduleDaysOfWeek = []string{"monday", "THURSDAY"}
	in.RunDateOffset = "ONE_DAY"
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := out.Request.SchedulingSettings
	if s == nil || s.SchedulerMode != "DAILY" || s.Daily == nil {
		t.Fatalf("expected WEEKLY mapped to DAILY+daily, got %+v", s)
	}
	if len(s.Daily.DaysOfWeek) != 2 || s.Daily.DaysOfWeek[0] != "MONDAY" || s.Daily.DaysOfWeek[1] != "THURSDAY" {
		t.Errorf("expected [MONDAY THURSDAY], got %v", s.Daily.DaysOfWeek)
	}
	if s.Daily.DailyOffset != "ONE_DAY" {
		t.Errorf("expected offset ONE_DAY, got %q", s.Daily.DailyOffset)
	}
}

func TestScheduleWeeklyRequiresDays(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.ScheduleRepeat = "WEEKLY" // no days
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for WEEKLY without days, got %q (%s)", out.Status, out.Message)
	}
}

func TestScheduleMonthlyByDay(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.ScheduleRepeat = "MONTHLY"
	in.ScheduleDayOfMonth = 15
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := out.Request.SchedulingSettings
	if s == nil || s.SchedulerMode != "MONTHLY" || s.Monthly == nil {
		t.Fatalf("expected a MONTHLY schedule with a monthly sub-object, got %+v", s)
	}
	if s.Monthly.MonthlyRepeat != "DAY" || s.Monthly.DayNumber != 15 {
		t.Errorf("expected DAY/15, got %+v", s.Monthly)
	}
}

func TestScheduleInvalidOffsetForMode(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.ScheduleRepeat = "DAILY"
	in.RunDateOffset = "ONE_HOUR" // hourly offset on a daily schedule
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for invalid offset, got %q (%s)", out.Status, out.Message)
	}
}

func TestRowSampling(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.SampleSize = 5000
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Sampling must be APPLIED in the source query (Postgres RANDOM()/LIMIT form).
	q := out.Request.SourceQuery
	if !strings.Contains(q, "RANDOM() <") || !strings.Contains(q, "LIMIT 5000") {
		t.Errorf("expected dialect sampling in sourceQuery, got %q", q)
	}
}

func TestNoSamplingByDefault(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out.Request.SourceQuery, "RANDOM()") {
		t.Errorf("expected no sampling by default (scan all rows), got %q", out.Request.SourceQuery)
	}
}

func TestNoAdaptiveSettingsByDefault(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No adaptive settings unless asked (server applies its defaults).
	if out.Request.MonitoringSettings.AdaptiveMonitors.Settings != nil {
		t.Errorf("expected no adaptive monitor settings by default, got %+v", out.Request.MonitoringSettings.AdaptiveMonitors.Settings)
	}
}

func TestAdaptiveMonitorSettings(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.DataLookback = 30 // only lookback set -> learningPhase defaults to 4
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := out.Request.MonitoringSettings.AdaptiveMonitors.Settings
	if s == nil || s.DataLookBack != 30 || s.LearningPhase != 4 {
		t.Fatalf("expected settings lookback=30 learning=4 (defaulted), got %+v", s)
	}
}

// usersMux returns a mux with the DQ connections handler plus mocked core-api user endpoints:
// users/current resolves to admin, and "jane" resolves by name; everything else is unknown.
func usersMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	mux.HandleFunc("/rest/2.0/users/current", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"u-current","userName":"admin"}`))
	})
	mux.HandleFunc("/rest/2.0/users", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("name") == "jane" {
			_, _ = w.Write([]byte(`{"results":[{"id":"u-jane","userName":"jane"}]}`))
			return
		}
		_, _ = w.Write([]byte(`{"results":[]}`))
	})
	return mux
}

func TestNotificationsConfigured(t *testing.T) {
	server := httptest.NewServer(usersMux())
	defer server.Close()

	in := baseInput()
	in.Notify = []string{"jobFailed", "scoreBelow"}
	in.NotifyRecipients = []string{"jane"}
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	n := out.Request.Notifications
	if n == nil || len(n.NotificationOptions) != 2 {
		t.Fatalf("expected 2 notification options, got %+v", n)
	}
	var foundScore bool
	for _, o := range n.NotificationOptions {
		if o.NotificationType == "SCORE_LESS_THAN_LIMIT" {
			foundScore = true
			if o.Quantity != 75 {
				t.Errorf("expected scoreBelow default quantity 75, got %d", o.Quantity)
			}
		}
	}
	if !foundScore {
		t.Errorf("scoreBelow alert missing: %+v", n.NotificationOptions)
	}
	// Recipients are usernames in a single EMAIL channel: invoking user (admin) + jane.
	if len(n.Channels) != 1 || n.Channels[0].Channel != "EMAIL" {
		t.Fatalf("expected one EMAIL channel, got %+v", n.Channels)
	}
	got := strings.Join(n.Channels[0].Recipients, ",")
	if got != "admin,jane" {
		t.Errorf("expected recipients [admin jane], got %v", n.Channels[0].Recipients)
	}
}

func TestNotificationUnresolvedRecipient(t *testing.T) {
	server := httptest.NewServer(usersMux())
	defer server.Close()

	in := baseInput()
	in.Notify = []string{"jobFailed"}
	in.NotifyRecipients = []string{"ghost"} // not found
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for unresolved recipient, got %q (%s)", out.Status, out.Message)
	}

	// With the proceed flag, it creates anyway with only the resolvable recipient (invoking user).
	in.NotifyProceedWithoutUnresolved = true
	out, err = tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusPreview {
		t.Fatalf("expected preview when proceeding without unresolved, got %q (%s)", out.Status, out.Message)
	}
	n := out.Request.Notifications
	if n == nil || len(n.Channels) != 1 || len(n.Channels[0].Recipients) != 1 || n.Channels[0].Recipients[0] != "admin" {
		t.Errorf("expected only the invoking user (admin) as recipient, got %+v", n)
	}
}

func TestUnknownNotificationNeedsInput(t *testing.T) {
	server := httptest.NewServer(usersMux())
	defer server.Close()

	in := baseInput()
	in.Notify = []string{"bogusAlert"}
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for unknown notification, got %q (%s)", out.Status, out.Message)
	}
}

func TestNoNotificationsByDefault(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Request.Notifications != nil {
		t.Errorf("expected no notifications by default, got %+v", out.Request.Notifications)
	}
}

func TestNoScheduleByDefault(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Request.SchedulingSettings != nil {
		t.Errorf("expected no schedule by default, got %+v", out.Request.SchedulingSettings)
	}
	// No time slice -> plain whole-table query, no ${rd}.
	if strings.Contains(out.Request.SourceQuery, "${rd}") {
		t.Errorf("did not expect ${rd} without a timeSliceColumn, got %q", out.Request.SourceQuery)
	}
}

func TestSelectedColumnsInQuery(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.SelectedColumns = []string{"txn_ts", " ", "amount"} // blank entry must be dropped
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Selected columns scope the SELECT list of the source query (blank dropped, Postgres-escaped).
	if !strings.Contains(out.Request.SourceQuery, `SELECT "txn_ts", "amount" FROM`) {
		t.Errorf("expected selected columns in the source query SELECT list, got %q", out.Request.SourceQuery)
	}
}

func TestConfirmCreatesAndReturnsRunID(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", connectionsHandler())
	mux.Handle("/rest/dq/1.0/jobs", createdJobHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.Confirm = true
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusCreated {
		t.Fatalf("expected created, got %q (%s)", out.Status, out.Message)
	}
	if out.JobID != "run-1" {
		t.Errorf("expected jobRunId run-1, got %q", out.JobID)
	}
	if out.JobName != "sales.transactions" {
		t.Errorf("expected server-echoed job name, got %q", out.JobName)
	}
}

func TestMissingFieldsNeedsInput(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{EdgeSiteName: "EDGE-1", JobType: "PULLUP"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input, got %q (%s)", out.Status, out.Message)
	}
}

func hasWarning(warnings []string, substr string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

// ---- WS2: Pullup sizing & Parallel JDBC ----

func TestPullupAutoSizingByDefault(t *testing.T) {
	server := httptest.NewServer(muxWithPerms("DATA_QUALITY_JOB_CREATE"))
	defer server.Close()

	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Automatic sizing = sparkJobSizing omitted (the public API auto-sizes when it is absent).
	if out.Request.JobSettings.PullupSettings.SparkJobSizing != nil {
		t.Errorf("expected automatic sizing (nil sparkJobSizing) by default, got %+v", out.Request.JobSettings.PullupSettings.SparkJobSizing)
	}
}

func TestPullupManualSizing(t *testing.T) {
	server := httptest.NewServer(muxWithPerms("DATA_QUALITY_JOB_CREATE"))
	defer server.Close()

	in := baseInput()
	in.SizingMaxExecutors = 4
	in.SizingExecutorMemoryGb = "2"
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s := out.Request.JobSettings.PullupSettings.SparkJobSizing
	if s == nil {
		t.Fatalf("expected manual sizing object, got nil")
	}
	if s.NumExecutors != 4 || s.ExecutorMemoryGb != 2 {
		t.Errorf("manual sizing values not applied: %+v", s)
	}
	// Unset manual fields fall back to wizard defaults (1 GB / 1).
	if s.NumExecutorCores != 1 || s.DriverMemoryGb != 1 || s.MemoryOverheadGb != 1 {
		t.Errorf("expected unset manual fields to default to 1, got %+v", s)
	}
}

func TestParallelJdbcManualRequiresPartitionCount(t *testing.T) {
	server := httptest.NewServer(muxWithPerms("DATA_QUALITY_JOB_CREATE"))
	defer server.Close()

	in := baseInput()
	in.ParallelJdbcPartitionColumn = "id" // specific column implies MANUAL; a count is required
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input when a partition column is set without a count, got %q (%s)", out.Status, out.Message)
	}
}

func TestParallelJdbcManualValid(t *testing.T) {
	server := httptest.NewServer(muxWithPerms("DATA_QUALITY_JOB_CREATE"))
	defer server.Close()

	in := baseInput()
	in.ParallelJdbcPartitionColumn = "id"
	in.ParallelJdbcPartitionNumber = 8
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	pj := out.Request.JobSettings.PullupSettings.LoadOptions.ParallelJdbcOptions
	if pj == nil || pj.Mode != "MANUAL" || pj.PartitionColumn != "id" || pj.PartitionNumber != 8 {
		t.Errorf("expected MANUAL parallel JDBC id/8, got %+v", pj)
	}
}

func TestParallelJdbcAutoColumnRequiresCount(t *testing.T) {
	server := httptest.NewServer(muxWithPerms("DATA_QUALITY_JOB_CREATE"))
	defer server.Close()

	in := baseInput()
	in.ParallelJdbcMode = "AUTO_COLUMN" // count required, no column
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for AUTO_COLUMN without a partition count, got %q (%s)", out.Status, out.Message)
	}
}

// ---- WS3: Pushdown compute settings ----

func TestPushdownComputeSettings(t *testing.T) {
	server := httptest.NewServer(muxWithPerms("DATA_QUALITY_JOB_CREATE"))
	defer server.Close()

	in := baseInput()
	in.JobType = "PUSHDOWN"
	in.PushdownConnections = 20
	in.PushdownThreads = 5
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ps := out.Request.JobSettings.PushdownSettings
	if ps == nil || ps.Connections != 20 || ps.Threads != 5 {
		t.Errorf("expected pushdown compute 20/5, got %+v", ps)
	}
}

func TestPushdownComputeOutOfRange(t *testing.T) {
	server := httptest.NewServer(muxWithPerms("DATA_QUALITY_JOB_CREATE"))
	defer server.Close()

	in := baseInput()
	in.JobType = "PUSHDOWN"
	in.PushdownConnections = 100 // > 50
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for connections out of range, got %q (%s)", out.Status, out.Message)
	}
}

func TestSizingRejectedOnPushdown(t *testing.T) {
	server := httptest.NewServer(muxWithPerms("DATA_QUALITY_JOB_CREATE"))
	defer server.Close()

	in := baseInput()
	in.JobType = "PUSHDOWN"
	in.SizingMaxExecutors = 2 // sizing is PULLUP-only
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for sizing on a PUSHDOWN job, got %q (%s)", out.Status, out.Message)
	}
}

func TestComputeRejectedOnPullup(t *testing.T) {
	server := httptest.NewServer(muxWithPerms("DATA_QUALITY_JOB_CREATE"))
	defer server.Close()

	in := baseInput()
	in.PushdownConnections = 10 // compute is PUSHDOWN-only
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for compute on a PULLUP job, got %q (%s)", out.Status, out.Message)
	}
}

// ---- WS4: per-type notification messages ----

func TestPerTypeNotificationMessage(t *testing.T) {
	server := httptest.NewServer(usersMux())
	defer server.Close()

	in := baseInput()
	in.Notify = []string{"jobFailed", "scoreBelow"}
	in.NotifyMessages = map[string]string{"jobFailed": "page me"}
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	n := out.Request.Notifications
	if n == nil || !n.UseIndividualMessages {
		t.Fatalf("expected useIndividualMessages=true, got %+v", n)
	}
	var msg string
	for _, o := range n.NotificationOptions {
		if o.NotificationType == "JOB_FAILED" {
			msg = o.Message
		}
	}
	if msg != "page me" {
		t.Errorf("expected jobFailed per-type message 'page me', got %q", msg)
	}
}

// ---- WS6: permission preflight ----

func TestCreatePermissionDenied(t *testing.T) {
	server := httptest.NewServer(muxWithPerms("DATA_QUALITY_JOB_RUN")) // run but not create
	defer server.Close()

	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusError {
		t.Fatalf("expected error when lacking create permission, got %q (%s)", out.Status, out.Message)
	}
	if !strings.Contains(out.Message, "DATA_QUALITY_JOB_CREATE") {
		t.Errorf("expected the missing permission named, got %q", out.Message)
	}
}

func TestSchedulePermissionDropsSchedule(t *testing.T) {
	server := httptest.NewServer(muxWithPerms("DATA_QUALITY_JOB_CREATE", "DATA_QUALITY_JOB_RUN")) // no schedule
	defer server.Close()

	in := baseInput()
	in.ScheduleRepeat = "DAILY"
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Request.SchedulingSettings != nil {
		t.Errorf("expected schedule dropped without Schedule permission, got %+v", out.Request.SchedulingSettings)
	}
	if !hasWarning(out.Warnings, "Schedule") {
		t.Errorf("expected a Schedule-permission warning, got %v", out.Warnings)
	}
}

func TestRunPermissionWarns(t *testing.T) {
	server := httptest.NewServer(muxWithPerms("DATA_QUALITY_JOB_CREATE", "DATA_QUALITY_JOB_SCHEDULE")) // no run
	defer server.Close()

	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasWarning(out.Warnings, "Run permission") {
		t.Errorf("expected a Run-permission warning, got %v", out.Warnings)
	}
}

// ---- WS1: job name handling ----

func TestServerGeneratedJobName(t *testing.T) {
	mux := muxWithPerms("DATA_QUALITY_JOB_CREATE")
	mux.HandleFunc("/rest/dq/internal/v1/jobs/name", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"jobName":"sales.transactions_2"}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), baseInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.JobName != "sales.transactions_2" {
		t.Errorf("expected server-generated unique name, got %q", out.JobName)
	}
}

func TestInvalidJobNameRejected(t *testing.T) {
	mux := muxWithPerms("DATA_QUALITY_JOB_CREATE")
	mux.HandleFunc("/rest/dq/internal/v1/jobs/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/validJobName") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte("false"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.JobName = "bad name!"
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for an invalid job name, got %q (%s)", out.Status, out.Message)
	}
	if out.AffectedStep == "" {
		t.Errorf("expected an affectedStep for the name error")
	}
}

// ---- WS7: success deep link ----

func TestSuccessReturnsJobDetailsLink(t *testing.T) {
	mux := muxWithPerms("DATA_QUALITY_JOB_CREATE", "DATA_QUALITY_JOB_SCHEDULE", "DATA_QUALITY_JOB_RUN")
	mux.Handle("/rest/dq/1.0/jobs", createdJobHandler())
	server := httptest.NewServer(mux)
	defer server.Close()

	in := baseInput()
	in.Confirm = true
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusCreated {
		t.Fatalf("expected created, got %q (%s)", out.Status, out.Message)
	}
	if out.JobDetailsLink != "/data-quality/jobs?jobName=sales.transactions" {
		t.Errorf("expected job details deep link, got %q", out.JobDetailsLink)
	}
}
