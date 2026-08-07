package clients

import (
	"encoding/json"
	"testing"

	"github.com/collibra/chip/pkg/chip"
)

func TestBuildProfileMonitorsMapsKeysToFields(t *testing.T) {
	pm, unknown := BuildProfileMonitors([]string{"rowCount", "MIN", " uniqueness ", "descriptiveStatistics"})
	if len(unknown) != 0 {
		t.Fatalf("expected no unknown keys, got %v", unknown)
	}
	// Case-insensitive + trimmed keys map to the right fields.
	if !pm.RowCount || !pm.Min || !pm.Uniqueness || !pm.DescriptiveStatistics {
		t.Errorf("selected monitors not all ON: %+v", pm)
	}
	// Everything not selected is OFF.
	if pm.NullValues || pm.EmptyFields || pm.Mean || pm.Max || pm.ExecutionTime {
		t.Errorf("unselected monitors should be OFF: %+v", pm)
	}
}

func TestBuildProfileMonitorsReportsUnknown(t *testing.T) {
	_, unknown := BuildProfileMonitors([]string{"rowCount", "bogus"})
	if len(unknown) != 1 || unknown[0] != "bogus" {
		t.Fatalf("expected unknown=[bogus], got %v", unknown)
	}
}

func TestDefaultMonitorKeysMatchCatalog(t *testing.T) {
	defaults := map[string]bool{}
	for _, k := range DefaultMonitorKeys() {
		defaults[k] = true
	}
	for _, m := range DqMonitorCatalog() {
		if m.DefaultEnabled != defaults[m.Key] {
			t.Errorf("monitor %q: DefaultEnabled=%v but DefaultMonitorKeys membership=%v", m.Key, m.DefaultEnabled, defaults[m.Key])
		}
	}
}

func TestBuildSchedulingSettingsNeverIsNil(t *testing.T) {
	for _, repeat := range []string{"", "NEVER", "never"} {
		s, err := BuildSchedulingSettings(DqScheduleInput{Repeat: repeat})
		if err != nil || s != nil {
			t.Errorf("repeat=%q: expected (nil,nil), got (%+v,%v)", repeat, s, err)
		}
	}
}

func TestBuildSchedulingSettingsDailyDefaults(t *testing.T) {
	s, err := BuildSchedulingSettings(DqScheduleInput{Repeat: "DAILY"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.SchedulerMode != "DAILY" || s.Daily == nil || len(s.Daily.DaysOfWeek) != 7 {
		t.Errorf("expected DAILY with 7 days, got %+v", s)
	}
	if s.ScheduledRunTime != "00:00:00" || s.Daily.DailyOffset != "SCHEDULED" || !s.IsActive {
		t.Errorf("unexpected defaults: %+v / daily %+v", s, s.Daily)
	}
}

func TestBuildSchedulingSettingsWeekdays(t *testing.T) {
	s, err := BuildSchedulingSettings(DqScheduleInput{Repeat: "WEEKDAYS", RunTime: "08:00:00"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.SchedulerMode != "DAILY" || len(s.Daily.DaysOfWeek) != 5 {
		t.Errorf("expected Mon-Fri (5 days), got %+v", s.Daily)
	}
}

func TestBuildSchedulingSettingsWeeklyInvalidDay(t *testing.T) {
	if _, err := BuildSchedulingSettings(DqScheduleInput{Repeat: "WEEKLY", DaysOfWeek: []string{"FUNDAY"}}); err == nil {
		t.Errorf("expected error for invalid day FUNDAY")
	}
	if _, err := BuildSchedulingSettings(DqScheduleInput{Repeat: "WEEKLY"}); err == nil {
		t.Errorf("expected error for WEEKLY with no days")
	}
}

func TestBuildSchedulingSettingsHourly(t *testing.T) {
	s, err := BuildSchedulingSettings(DqScheduleInput{Repeat: "HOURLY", RunDateOffset: "TWO_HOURS"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.SchedulerMode != "HOURLY" || s.Hourly == nil || s.Hourly.HourlyOffset != "TWO_HOURS" {
		t.Errorf("expected HOURLY/TWO_HOURS, got %+v", s)
	}
	if _, err := BuildSchedulingSettings(DqScheduleInput{Repeat: "HOURLY", RunDateOffset: "SEVEN_DAYS"}); err == nil {
		t.Errorf("expected error: SEVEN_DAYS is not a valid hourly offset")
	}
}

func TestHasPermission(t *testing.T) {
	perms := []string{"DATA_QUALITY_JOB_CREATE", "DATA_QUALITY_JOB_RUN"}
	if !HasPermission(perms, "DATA_QUALITY_JOB_CREATE") {
		t.Error("expected create permission present")
	}
	if !HasPermission(perms, "data_quality_job_run") { // case-insensitive
		t.Error("expected run permission present (case-insensitive)")
	}
	if HasPermission(perms, "DATA_QUALITY_JOB_SCHEDULE") {
		t.Error("did not expect schedule permission")
	}
}

func TestDqJobDetailsPath(t *testing.T) {
	if got := DqJobDetailsPath("sales.orders"); got != "/data-quality/jobs?jobName=sales.orders" {
		t.Errorf("unexpected path: %q", got)
	}
	if p := DqJobDetailsPath("my job"); p != "/data-quality/jobs?jobName=my+job" {
		t.Errorf("expected query-escaped name, got %q", p)
	}
}

func TestBuildSchedulingSettingsMonthly(t *testing.T) {
	s, err := BuildSchedulingSettings(DqScheduleInput{Repeat: "MONTHLY", MonthlyMode: "LAST", RunDateOffset: "FIRST_OF_PRIOR_MONTH"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Monthly == nil || s.Monthly.MonthlyRepeat != "LAST" || s.Monthly.MonthlyOffset != "FIRST_OF_PRIOR_MONTH" {
		t.Errorf("expected MONTHLY LAST/FIRST_OF_PRIOR_MONTH, got %+v", s.Monthly)
	}
	if _, err := BuildSchedulingSettings(DqScheduleInput{Repeat: "MONTHLY", MonthlyMode: "DAY", DayOfMonth: 0}); err == nil {
		t.Errorf("expected error: DAY mode needs dayOfMonth 1-28")
	}
	if _, err := BuildSchedulingSettings(DqScheduleInput{Repeat: "MONTHLY", MonthlyMode: "DAY", DayOfMonth: 31}); err == nil {
		t.Errorf("expected error: dayOfMonth 31 is out of range")
	}
}

func TestNextAvailableDqJobName(t *testing.T) {
	base := "public.customers"
	cases := []struct {
		name     string
		existing []string
		want     string
	}{
		{"base free", nil, base},
		{"base free ignores unrelated", []string{"other.table", "public.customers_1"}, base}, // base itself absent -> base
		{"base taken, no suffixes", []string{base}, base + "_1"},
		{"base taken, _1 taken", []string{base, base + "_1"}, base + "_2"},
		{"smallest free fills gap", []string{base, base + "_1", base + "_3"}, base + "_2"},
		{"non-numeric suffix ignored", []string{base, base + "_abc"}, base + "_1"},
		{"prefix-substring not a match", []string{base, "public.customersX"}, base + "_1"},
	}
	for _, c := range cases {
		if got := NextAvailableDqJobName(base, c.existing); got != c.want {
			t.Errorf("%s: NextAvailableDqJobName(%q, %v) = %q, want %q", c.name, base, c.existing, got, c.want)
		}
	}
}

func TestIsCancellableDqRunState(t *testing.T) {
	cancellable := []string{"RUNNING", "SUBMITTED", "WAITING", "DISPATCHED", "SETUP", "SENDING",
		"running", " Running ", "dispatched"}
	for _, s := range cancellable {
		if !IsCancellableDqRunState(s) {
			t.Errorf("expected %q to be cancellable (nonterminal)", s)
		}
	}
	terminal := []string{"FINISHED", "COMPLETED", "FAILED", "CANCELLED", "ABORTED", "ERROR", "", "  ", "bogus"}
	for _, s := range terminal {
		if IsCancellableDqRunState(s) {
			t.Errorf("expected %q to be non-cancellable (terminal/unknown)", s)
		}
	}
}

func TestIsDeletableDqRunState(t *testing.T) {
	deletable := []string{"FINISHED", "CANCELLED", "FAILED", "UNKNOWN", "finished", " Failed ", "bogus"}
	for _, s := range deletable {
		if !IsDeletableDqRunState(s) {
			t.Errorf("expected %q to be deletable (not in progress)", s)
		}
	}
	inProgress := append([]string{"", "  ", "running", " Setup "}, DqCancellableRunStates...)
	for _, s := range inProgress {
		if IsDeletableDqRunState(s) {
			t.Errorf("expected %q to be non-deletable (in progress or unknowable)", s)
		}
	}
}

// The whole point of the UpdateDqJobRequest patch types: a field the caller did not touch must
// marshal away entirely. A zero value on the wire is an instruction to the server, not an absence.
func TestUpdateDqJobRequestOmitsUntouchedFields(t *testing.T) {
	body, err := json.Marshal(UpdateDqJobRequest{
		JobName:            "sales.orders",
		SchedulingSettings: &DqSchedulingSettings{SchedulerMode: "DAILY", ScheduledRunTime: "04:00:00", IsActive: true},
	})
	if err != nil {
		t.Fatalf("failed to marshal request: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("failed to parse marshalled request: %v", err)
	}
	for _, field := range []string{"sourceQuery", "runDate", "runDateEnd", "dataLocation", "jobSettings", "monitoringSettings", "notifications"} {
		if _, present := got[field]; present {
			t.Errorf("%q should be absent from a schedule-only patch, got %s", field, body)
		}
	}
	if _, present := got["schedulingSettings"]; !present {
		t.Errorf("schedulingSettings should be present, got %s", body)
	}
}

// dataLookBack and learningPhase are pointers because 0 is a MEANINGFUL value (no look back / no
// learning phase), so a plain int could not express "leave this one alone".
func TestAdaptiveMonitorSettingsPatchDistinguishesZeroFromUnset(t *testing.T) {
	body, err := json.Marshal(DqAdaptiveMonitorSettingsPatch{DataLookBack: chip.Ptr(0)})
	if err != nil {
		t.Fatalf("failed to marshal settings: %v", err)
	}
	if got, want := string(body), `{"dataLookBack":0}`; got != want {
		t.Errorf("marshalled = %s, want %s (an explicit 0 must survive, learningPhase must be omitted)", got, want)
	}
}

// numPartitions has the same problem: 0 means "let Spark decide".
func TestLoadOptionsPatchKeepsExplicitZeroPartitions(t *testing.T) {
	body, err := json.Marshal(DqLoadOptionsPatch{NumPartitions: chip.Ptr(0)})
	if err != nil {
		t.Fatalf("failed to marshal load options: %v", err)
	}
	if got, want := string(body), `{"numPartitions":0}`; got != want {
		t.Errorf("marshalled = %s, want %s", got, want)
	}
}

// The update path's monitor toggles are authoritative, so every one is always on the wire —
// otherwise an omitted toggle would read as "unchanged" and the selection could not turn one off.
func TestPatchAdaptiveMonitorsFromProfileSendsEveryToggle(t *testing.T) {
	profile, _ := BuildProfileMonitors([]string{"rowCount", "min"})
	body, err := json.Marshal(PatchAdaptiveMonitorsFromProfile(profile))
	if err != nil {
		t.Fatalf("failed to marshal monitors: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatalf("failed to parse marshalled monitors: %v", err)
	}
	for _, key := range MonitorKeys() {
		if _, present := got[key]; !present {
			t.Errorf("%q should always be sent so the selection is authoritative, got %s", key, body)
		}
	}
	if got["rowCount"] != true || got["min"] != true {
		t.Errorf("selected monitors should be true, got %s", body)
	}
	if got["nullValues"] != false {
		t.Errorf("unselected monitors should be false, got %s", body)
	}
}

func TestEnabledPublicMonitorKeysReadsJobsCurrentSelection(t *testing.T) {
	keys := EnabledPublicMonitorKeys(&DqPublicAdaptiveMonitors{RowCount: true, Uniqueness: true})
	if len(keys) != 2 || keys[0] != "rowCount" || keys[1] != "uniqueness" {
		t.Errorf("keys = %v, want [rowCount uniqueness] in catalog order", keys)
	}
	if EnabledPublicMonitorKeys(nil) != nil {
		t.Error("a job with no adaptive monitors should yield no keys")
	}
}
