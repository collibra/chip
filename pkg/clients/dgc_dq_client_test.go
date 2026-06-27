package clients

import "testing"

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
