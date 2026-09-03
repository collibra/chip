// Package update_dq_job implements the dq_update_job MCP tool — a PARTIAL update of an existing
// Collibra data-quality job definition. Supply the job name plus only the fields to change; anything
// omitted is left exactly as it is.
//
// The flow is a two-call converge on the public update:
//
//	Look up : GET   /rest/dq/1.0/jobs/{jobName} -> on failure, relay a meaningful error; on success,
//	          diff the job's current configuration against the requested changes (confirm_required).
//	Update  : PATCH /rest/dq/1.0/jobs/{jobName} — only once the caller re-calls with confirm=true.
//
// confirm=false (the default) is strictly READ-ONLY: it never reaches the PATCH. Permissions are
// enforced by the server; 400/401/403/404 and transport failures are surfaced as messages with
// actionable guidance rather than Go errors.
//
// WHAT MERGES AND WHAT REPLACES. The public update contract is not uniformly granular, and the tool
// papers over that using the job it just fetched (see clients.UpdateDqJobRequest):
//
//	jobSettings, monitoringSettings   merged per field by the server — only what's set is sent.
//	dataLocation                      whole-object replace; the tool OVERLAYS the caller's fields onto
//	                                  the job's current location so a single field can be changed.
//	schedulingSettings                whole-object replace; rebuilt from the schedule inputs.
//	notifications                     whole-object replace; rebuilt from the notify* inputs, so the
//	                                  job's existing notification config is REPLACED, not extended.
//
// Nothing can be unset: per the spec, null and omitted mean the same thing. A schedule is therefore
// switched OFF with scheduleRepeat=NEVER, which resends the current schedule with isActive=false.
//
// SCOPE. jobType is immutable and back-runs are not part of the update contract, so neither is
// exposed. The scan shape (columns, row filter, sampling, time slice) is changed by editing
// sourceQuery directly — unlike create_data_quality_job, this tool does not recompose the SQL, since
// doing so needs the connection dialect and live column list and would clobber hand-edited SQL. DQ
// rules are managed by create_data_quality_rule / deploy_data_quality_rule_template, not here.
package update_dq_job

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type Status string

const (
	StatusUpdated         Status = "updated"
	StatusConfirmRequired Status = "confirm_required"
	StatusNeedsInput      Status = "needs_input"
	StatusError           Status = "error"
)

// Input — jobName identifies the job; every other field is optional and OMITTING IT LEAVES THAT PART
// OF THE JOB UNCHANGED. Grouped the same way as create_data_quality_job's input so the two read alike.
type Input struct {
	JobName string `json:"jobName" jsonschema:"The name of the existing data-quality job to update."`

	// --- Scan SQL. The job's source query is replaced verbatim. Use ${rd}/${rdEnd} to keep (or
	// introduce) an incremental time slice. ---
	SourceQuery string `json:"sourceQuery,omitempty" jsonschema:"Replacement scan SQL for the job, used verbatim (e.g. SELECT \"id\", \"amount\" FROM \"sales\".\"orders\" WHERE \"txn_ts\" >= '${rd}' AND \"txn_ts\" < '${rdEnd}'). This is how you change which columns are profiled, the row filter, sampling, or the time slice — the tool does NOT rebuild the SQL for you. Omit to leave the query unchanged; review the current one from the confirm_required diff first."`

	// --- Run date window. ${rd}/${rdEnd} in sourceQuery are substituted from these per run. Format
	// determines the kind: 'yyyy-MM-dd' => DATE, RFC3339 => TIMESTAMP. ---
	RunDate    string `json:"runDate,omitempty" jsonschema:"Start of the run-date window — 'yyyy-MM-dd' (DATE) or RFC3339 'yyyy-MM-ddTHH:mm:ssZ' (TIMESTAMP). Substituted for ${rd} in the source query. Omit to leave unchanged."`
	RunDateEnd string `json:"runDateEnd,omitempty" jsonschema:"Exclusive end of the run-date window, same formats as runDate and must be later than it. Substituted for ${rdEnd}. Omit to leave unchanged."`
	DateFormat string `json:"dateFormat,omitempty" jsonschema:"DATE or TIMESTAMP — how the engine formats ${rd}/${rdEnd} when substituting them. Omit to leave unchanged; it is inferred from runDate's format when you set that instead."`

	// --- Data location (for a moved/renamed table). Provided fields are overlaid onto the job's
	// current location, so you can change just one. ---
	EdgeSiteName   string `json:"edgeSiteName,omitempty" jsonschema:"New edge site name, if the job should run on a different Edge site. Omit to keep the current one."`
	ConnectionName string `json:"connectionName,omitempty" jsonschema:"New edge connection name, if the job should read through a different connection. Omit to keep the current one."`
	DataSourceName string `json:"dataSourceName,omitempty" jsonschema:"New data source / database name. Omit to keep the current one."`
	SchemaName     string `json:"schemaName,omitempty" jsonschema:"New schema name — use when the table has moved to another schema. Omit to keep the current one."`
	TableName      string `json:"tableName,omitempty" jsonschema:"New table name — use when the table has been renamed. Omit to keep the current one. NOTE: retargeting the location does not rewrite sourceQuery, which names the schema/table itself — set sourceQuery too."`

	// --- Monitors. AUTHORITATIVE when provided: any monitor not listed is turned OFF. Omit the
	// field entirely to leave the job's monitor selection alone. ---
	Monitors []string `json:"monitors,omitempty" jsonschema:"Monitor keys the job should have enabled — AUTHORITATIVE, so anything omitted from this list is turned OFF. Valid keys: rowCount, nullValues, emptyFields, uniqueness, min, mean, max, executionTime, descriptiveStatistics. Omit the field entirely to leave the current selection unchanged (the confirm_required diff shows it). descriptiveStatistics UNMASKS sensitive data — only include it after explicit user confirmation."`

	// --- Advanced monitor settings (adaptive behavior). Each is sent only when set, so changing
	// one does not disturb the other. ---
	DataLookback  int `json:"dataLookback,omitempty" jsonschema:"Adaptive monitors: number of prior runs used as the baseline. Omit to leave unchanged."`
	LearningPhase int `json:"learningPhase,omitempty" jsonschema:"Adaptive monitors: number of runs before adaptive monitors begin alerting. Omit to leave unchanged."`

	// --- Schedule. Rebuilt wholesale from these fields; NEVER switches an existing schedule off. ---
	ScheduleRepeat      string   `json:"scheduleRepeat,omitempty" jsonschema:"NEVER (switch the existing schedule OFF) | HOURLY | DAILY | WEEKLY | WEEKDAYS | MONTHLY. WEEKLY needs scheduleDaysOfWeek; MONTHLY uses scheduleDayOfMonth or scheduleMonthlyMode. Omit to leave the schedule unchanged. Setting this REPLACES the whole schedule, so re-supply scheduleRunTime/runDateOffset if they should be kept."`
	ScheduleRunTime     string   `json:"scheduleRunTime,omitempty" jsonschema:"Scheduled run time, UTC HH:mm[:ss] (e.g. '04:00:00'). Defaults to 00:00:00 when a schedule is set. Only meaningful alongside scheduleRepeat."`
	ScheduleDaysOfWeek  []string `json:"scheduleDaysOfWeek,omitempty" jsonschema:"For WEEKLY: the days to run, e.g. ['MONDAY','THURSDAY'] (MONDAY..SUNDAY). Ignored for other modes (DAILY runs every day; WEEKDAYS runs Mon-Fri)."`
	ScheduleDayOfMonth  int      `json:"scheduleDayOfMonth,omitempty" jsonschema:"For MONTHLY with scheduleMonthlyMode=DAY (the default): day of month 1-28."`
	ScheduleMonthlyMode string   `json:"scheduleMonthlyMode,omitempty" jsonschema:"For MONTHLY: DAY (default — run on scheduleDayOfMonth) | FIRST (first day of month) | LAST (last day of month)."`
	RunDateOffset       string   `json:"runDateOffset,omitempty" jsonschema:"How far back the slice's run date (${rd}) is from the execution time. Default SCHEDULED. DAILY/WEEKLY/WEEKDAYS: SCHEDULED | ONE_DAY..SEVEN_DAYS. HOURLY: SCHEDULED | ONE_HOUR | TWO_HOURS. MONTHLY: SCHEDULED | FIRST_OF_CURRENT_MONTH | FIRST_OF_PRIOR_MONTH | LAST_OF_PRIOR_MONTH."`

	// --- Notifications. REPLACED wholesale when any notify* field is set. ---
	Notify                         []string          `json:"notify,omitempty" jsonschema:"Notification keys the job should have enabled — AUTHORITATIVE. Keys: jobFailed, rowsBelow, scoreBelow, runTimeAbove, jobCompleted, runsWithoutData, daysWithoutData. Setting any notify* field REPLACES the job's entire notification configuration (options, messages and recipients), so re-supply everything that should be kept — the confirm_required diff shows what is there now. Omit all notify* fields to leave notifications untouched."`
	NotifyRowsBelow                int               `json:"notifyRowsBelow,omitempty" jsonschema:"Threshold for rowsBelow — alert when row count <= this. Default 1."`
	NotifyScoreBelow               int               `json:"notifyScoreBelow,omitempty" jsonschema:"Threshold for scoreBelow — alert when score (0-100) <= this. Default 75."`
	NotifyRunTimeAboveMinutes      int               `json:"notifyRunTimeAboveMinutes,omitempty" jsonschema:"Threshold for runTimeAbove — alert when run time minutes > this. Default 60."`
	NotifyRunsWithoutData          int               `json:"notifyRunsWithoutData,omitempty" jsonschema:"Threshold for runsWithoutData — alert when runs without data >= this. Default 1."`
	NotifyDaysWithoutData          int               `json:"notifyDaysWithoutData,omitempty" jsonschema:"Threshold for daysWithoutData — alert when days without data >= this. Default 1."`
	NotifyMessage                  string            `json:"notifyMessage,omitempty" jsonschema:"Optional global message applied to the enabled notifications."`
	NotifyMessages                 map[string]string `json:"notifyMessages,omitempty" jsonschema:"Per-notification message overrides, keyed by notification key. A per-key message overrides notifyMessage for just that notification."`
	NotifyRecipients               []string          `json:"notifyRecipients,omitempty" jsonschema:"Recipients by username or email (the invoking user is always included). Each is validated against active Collibra accounts; unresolved ones are reported."`
	NotifyProceedWithoutUnresolved bool              `json:"notifyProceedWithoutUnresolved,omitempty" jsonschema:"If some notifyRecipients can't be resolved to an active account, set true to update anyway with the resolvable recipients. Default false: the tool returns needs_input listing the unresolved ones so you can fix or confirm."`

	// --- PUSHDOWN compute. Sent individually, so changing one leaves the other alone. ---
	PushdownConnections int `json:"pushdownConnections,omitempty" jsonschema:"PUSHDOWN compute: number of concurrent source connections (1-50). Omit to leave unchanged. Only valid on a PUSHDOWN job."`
	PushdownThreads     int `json:"pushdownThreads,omitempty" jsonschema:"PUSHDOWN compute: worker threads per connection (1-10). Omit to leave unchanged. Only valid on a PUSHDOWN job."`

	// --- PULLUP sizing. Setting ANY sizing* field sends a complete manual sizing block (wizard
	// default 1 for the fields left unset), switching the job off automatic sizing. ---
	SizingMaxExecutors     int    `json:"sizingMaxExecutors,omitempty" jsonschema:"PULLUP manual sizing: number of executors. Setting any sizing* field switches the job to MANUAL sizing, sending a complete sizing block with 1 for the fields you leave unset. Only valid on a PULLUP job."`
	SizingExecutorCores    int    `json:"sizingExecutorCores,omitempty" jsonschema:"PULLUP manual sizing: cores per executor. Default 1 when manual sizing is engaged."`
	SizingExecutorMemoryGb string `json:"sizingExecutorMemoryGb,omitempty" jsonschema:"PULLUP manual sizing: memory per executor in whole GB, as a string (e.g. '2'). Default '1' when manual sizing is engaged."`
	SizingDriverCores      int    `json:"sizingDriverCores,omitempty" jsonschema:"PULLUP manual sizing: driver cores. Default 1 when manual sizing is engaged."`
	SizingDriverMemoryGb   string `json:"sizingDriverMemoryGb,omitempty" jsonschema:"PULLUP manual sizing: driver memory in whole GB, as a string (e.g. '2'). Default '1' when manual sizing is engaged."`
	SizingMemoryOverheadGb string `json:"sizingMemoryOverheadGb,omitempty" jsonschema:"PULLUP manual sizing: memory overhead in whole GB, as a string (e.g. '1'). Default '1' when manual sizing is engaged."`
	SizingNumPartitions    int    `json:"sizingNumPartitions,omitempty" jsonschema:"PULLUP: number of Spark input partitions (load options). Omit to leave unchanged; pass -1 to reset it to 0 ('let Spark decide')."`

	ParallelJdbcMode            string `json:"parallelJdbcMode,omitempty" jsonschema:"PULLUP Parallel JDBC mode: AUTO | AUTO_COLUMN | MANUAL. AUTO needs nothing else; AUTO_COLUMN requires parallelJdbcPartitionNumber; MANUAL requires both parallelJdbcPartitionColumn and parallelJdbcPartitionNumber. Omit to infer from the other fields, or omit all three to leave Parallel JDBC unchanged."`
	ParallelJdbcPartitionColumn string `json:"parallelJdbcPartitionColumn,omitempty" jsonschema:"PULLUP Parallel JDBC: the column to partition on (MANUAL mode only)."`
	ParallelJdbcPartitionNumber int    `json:"parallelJdbcPartitionNumber,omitempty" jsonschema:"PULLUP Parallel JDBC: number of partitions. Required for AUTO_COLUMN and MANUAL modes."`

	SparkSqlProperties map[string]string `json:"sparkSqlProperties,omitempty" jsonschema:"PULLUP: Spark SQL key/value properties to set on the job. Replaces the job's existing property map. Only valid on a PULLUP job."`

	AcknowledgeDescriptiveStatistics bool `json:"acknowledgeDescriptiveStatistics,omitempty" jsonschema:"Required true when monitors includes descriptiveStatistics — confirms you accept that it UNMASKS sensitive values. Without it the tool refuses to proceed."`

	Confirm bool `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) is READ-ONLY: it returns a before/after diff of what WOULD change plus the exact request, and updates nothing — review it with the user. true applies the update."`
}

// FieldChange is one line of the before/after diff, so the user can see exactly what a partial
// update would alter before it happens.
type FieldChange struct {
	Field    string `json:"field" jsonschema:"The part of the job being changed (e.g. schedule, monitors, sourceQuery)."`
	Current  string `json:"current" jsonschema:"The job's current value, or '(none)' when it has none."`
	Proposed string `json:"proposed" jsonschema:"The value the update would set."`
}

type Output struct {
	Status         Status                      `json:"status" jsonschema:"updated | confirm_required | needs_input | error."`
	Message        string                      `json:"message" jsonschema:"Human-readable outcome and what to do next."`
	JobName        string                      `json:"jobName,omitempty" jsonschema:"The job that was updated (or, on confirm_required, the one to confirm)."`
	JobType        string                      `json:"jobType,omitempty" jsonschema:"The job's execution type (PUSHDOWN/PULLUP). It cannot be changed by this tool."`
	Changes        []FieldChange               `json:"changes,omitempty" jsonschema:"On confirm_required: the before/after diff of everything this update would change. Show it to the user before confirming."`
	Request        *clients.UpdateDqJobRequest `json:"request,omitempty" jsonschema:"The exact public-API payload (PATCH /rest/dq/1.0/jobs/{jobName}) that will be / was submitted. Only the fields being changed appear in it; everything else is left untouched server-side."`
	JobDetailsLink string                      `json:"jobDetailsLink,omitempty" jsonschema:"Job Details deep-link path (relative to the Collibra instance URL)."`
	Warnings       []string                    `json:"warnings,omitempty" jsonschema:"Non-fatal warnings to surface to the user (e.g. notifications being replaced wholesale, settings that don't apply to this job type)."`
	Guidance       string                      `json:"guidance,omitempty" jsonschema:"On confirm_required/needs_input/error, what to do next."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "dq_update_job",
		Title: "Update Data Quality Job",
		Description: "Changes the configuration of an EXISTING Collibra data-quality job, identified by its jobName. " +
			"This is a PARTIAL update: supply only the fields you want to change and everything else is left exactly " +
			"as it is. Use this instead of deleting and recreating a job — recreating destroys the job's run history, " +
			"results and learned monitor baselines.\n\n" +
			"WHAT YOU CAN CHANGE: the scan SQL (sourceQuery); the run-date window (runDate/runDateEnd/dateFormat); the " +
			"recurring schedule (scheduleRepeat/scheduleRunTime/scheduleDaysOfWeek/... — scheduleRepeat=NEVER switches " +
			"an existing schedule OFF); the monitor set (monitors) and adaptive baseline (dataLookback/learningPhase); " +
			"notifications (notify keys, thresholds, messages, notifyRecipients); PUSHDOWN compute " +
			"(pushdownConnections/pushdownThreads); PULLUP sizing (sizing*/parallelJdbc*/sparkSqlProperties); and the " +
			"data location (edgeSiteName/connectionName/dataSourceName/schemaName/tableName) when the table has moved " +
			"or been renamed.\n\n" +
			"MERGE BEHAVIOUR — worth telling the user before confirming. Most settings merge field by field, but three " +
			"are REPLACED wholesale: `monitors` is authoritative (any monitor you leave out is turned OFF), the schedule " +
			"is rebuilt from the schedule inputs, and setting ANY notify* field replaces the job's entire notification " +
			"configuration. Re-supply the parts that should be kept. Data-location fields are overlaid onto the current " +
			"location, so changing just tableName is fine. Nothing can be unset — the API treats omitted and null alike.\n\n" +
			"SAFETY CHECKPOINT: confirm=false (the default) is READ-ONLY — it looks the job up and returns a before/after " +
			"diff (`changes`) plus the exact request, and changes nothing. Review it with the user, then call again with " +
			"the same inputs and confirm=true to apply. If the job already has the requested values, the tool says so " +
			"instead of issuing a pointless update.\n\n" +
			"NOT CHANGEABLE HERE: a job's type (PUSHDOWN/PULLUP) is immutable, and back-runs are not part of the update " +
			"API. Which columns are profiled, the row filter, sampling and the time slice all live inside sourceQuery — " +
			"edit it directly (this tool does not rebuild the SQL for you; read the current query from the diff first). " +
			"DQ rules are managed with create_data_quality_rule and deploy_data_quality_rule_template. To remove a job " +
			"entirely use dq_delete_job.\n\n" +
			"Example user requests: \"Change the sales.orders data quality job to run daily at 4am\"; \"Add the " +
			"uniqueness monitor to my customers quality job\"; \"Stop the scheduled runs on public.transactions\"; " +
			"\"The orders table moved to the reporting schema — point the DQ job at it\"; \"Alert dana@example.com when " +
			"the orders DQ job fails.\"",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		// Writes only on confirm=true. Changing configuration is not destructive (no data or history is
		// removed — that is dq_delete_job). Idempotent: reapplying the same patch yields the same state.
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		jobName := strings.TrimSpace(input.JobName)
		if jobName == "" {
			return Output{
				Status:   StatusNeedsInput,
				Message:  "Provide the job to update.",
				Guidance: "Supply jobName — the name of the existing data-quality job to update.",
			}, nil
		}
		if !clients.IsValidDqJobName(jobName) {
			return Output{
				Status:   StatusNeedsInput,
				JobName:  jobName,
				Message:  fmt.Sprintf("%q is not a valid data-quality job name.", jobName),
				Guidance: "Job names contain only letters, digits, '.', '-' and '_', and cannot start with '-'. Check the name and retry.",
			}, nil
		}
		if !hasAnyChange(input) {
			return Output{
				Status:   StatusNeedsInput,
				JobName:  jobName,
				Message:  fmt.Sprintf("No changes were requested for job %q.", jobName),
				Guidance: "Supply at least one field to change: sourceQuery, runDate/runDateEnd/dateFormat, the schedule (scheduleRepeat/...), monitors, dataLookback/learningPhase, notifications (notify/...), PUSHDOWN compute (pushdownConnections/pushdownThreads), PULLUP sizing (sizing*/parallelJdbc*/sparkSqlProperties), or the data location (edgeSiteName/connectionName/dataSourceName/schemaName/tableName).",
			}, nil
		}

		// Reject unknown monitor keys before any API call.
		if len(input.Monitors) > 0 {
			if _, unknown := clients.BuildProfileMonitors(input.Monitors); len(unknown) > 0 {
				return Output{
					Status:   StatusNeedsInput,
					JobName:  jobName,
					Message:  fmt.Sprintf("Unknown monitor(s): %s.", strings.Join(unknown, ", ")),
					Guidance: "Use monitor keys from: " + strings.Join(clients.MonitorKeys(), ", ") + ".",
				}, nil
			}
		}

		// Descriptive statistics UNMASKS sensitive data — require explicit acknowledgement, as create does.
		if monitorsIncludeDescriptiveStats(input.Monitors) && !input.AcknowledgeDescriptiveStatistics {
			return Output{
				Status:   StatusNeedsInput,
				JobName:  jobName,
				Message:  "Enabling descriptive statistics may expose sensitive values if they are present in the columns included in the scan.",
				Guidance: "To proceed with descriptiveStatistics, set acknowledgeDescriptiveStatistics=true; otherwise remove it from monitors.",
			}, nil
		}

		if err := validateDateFormat(input); err != nil {
			return Output{Status: StatusNeedsInput, JobName: jobName, Message: err.Error(), Guidance: "Use DATE with 'yyyy-MM-dd' values, or TIMESTAMP with RFC3339 'yyyy-MM-ddTHH:mm:ssZ' values."}, nil
		}

		// Validate the schedule before any API call. NEVER is handled later (it needs the current job).
		schedule, err := buildSchedule(input)
		if err != nil {
			return Output{
				Status:   StatusNeedsInput,
				JobName:  jobName,
				Message:  err.Error(),
				Guidance: "Fix the schedule input: scheduleRepeat (NEVER/HOURLY/DAILY/WEEKLY/WEEKDAYS/MONTHLY), scheduleRunTime, scheduleDaysOfWeek (WEEKLY), scheduleDayOfMonth/scheduleMonthlyMode (MONTHLY), runDateOffset.",
			}, nil
		}

		notifications, notifyOut := buildNotifications(ctx, collibraClient, input, jobName)
		if notifyOut != nil {
			return *notifyOut, nil
		}

		job, code, err := clients.GetDqJob(ctx, collibraClient, jobName)
		if err != nil {
			return lookupError(code, err, jobName), nil
		}

		request, warnings, err := buildRequest(input, job, schedule, notifications)
		if err != nil {
			return Output{
				Status:   StatusNeedsInput,
				JobName:  jobName,
				JobType:  job.JobType,
				Message:  err.Error(),
				Guidance: "Fix the input and retry. Sizing/Parallel JDBC apply to PULLUP jobs and compute settings to PUSHDOWN jobs; a job's type cannot be changed.",
			}, nil
		}

		changes := diff(job, request)
		if len(changes) == 0 {
			return Output{
				Status:         StatusNeedsInput,
				JobName:        jobName,
				JobType:        job.JobType,
				JobDetailsLink: clients.DqJobDetailsPath(jobName),
				Warnings:       warnings,
				Message:        fmt.Sprintf("Job %q already has the requested configuration — nothing to update.", jobName),
				Guidance:       "No update was sent. Change one of the values, or leave the job as it is.",
			}, nil
		}

		if !input.Confirm {
			return Output{
				Status:         StatusConfirmRequired,
				JobName:        jobName,
				JobType:        job.JobType,
				Changes:        changes,
				Request:        request,
				JobDetailsLink: clients.DqJobDetailsPath(jobName),
				Warnings:       warnings,
				Message:        fmt.Sprintf("About to update %d setting(s) on job %q.", len(changes), jobName),
				Guidance:       "Nothing has been changed yet. Show the before/after diff to the user and get their approval, then re-call this tool with the same inputs and confirm=true. Anything not listed in `request` is left untouched.",
			}, nil
		}

		if code, err := clients.UpdateDqJob(ctx, collibraClient, jobName, *request); err != nil {
			return updateError(code, err, jobName), nil
		}
		return Output{
			Status:         StatusUpdated,
			JobName:        jobName,
			JobType:        job.JobType,
			Changes:        changes,
			Request:        request,
			JobDetailsLink: clients.DqJobDetailsPath(jobName),
			Warnings:       warnings,
			Message:        fmt.Sprintf("Job %q updated — %d setting(s) changed. Everything else was left unchanged.", jobName, len(changes)),
		}, nil
	}
}

// hasAnyChange reports whether the caller asked for anything at all beyond naming the job, so a
// bare jobName is rejected before it costs an API call.
func hasAnyChange(in Input) bool {
	return strings.TrimSpace(in.SourceQuery) != "" ||
		strings.TrimSpace(in.RunDate) != "" || strings.TrimSpace(in.RunDateEnd) != "" || strings.TrimSpace(in.DateFormat) != "" ||
		hasLocationChange(in) ||
		len(in.Monitors) > 0 || in.DataLookback > 0 || in.LearningPhase > 0 ||
		strings.TrimSpace(in.ScheduleRepeat) != "" ||
		hasNotifyFields(in) ||
		in.PushdownConnections > 0 || in.PushdownThreads > 0 ||
		hasSizingFields(in) || hasParallelJdbcFields(in) || len(in.SparkSqlProperties) > 0
}

func hasLocationChange(in Input) bool {
	return strings.TrimSpace(in.EdgeSiteName) != "" || strings.TrimSpace(in.ConnectionName) != "" ||
		strings.TrimSpace(in.DataSourceName) != "" || strings.TrimSpace(in.SchemaName) != "" ||
		strings.TrimSpace(in.TableName) != ""
}

func hasNotifyFields(in Input) bool {
	return len(in.Notify) > 0 || len(in.NotifyRecipients) > 0 || len(in.NotifyMessages) > 0 ||
		strings.TrimSpace(in.NotifyMessage) != "" ||
		in.NotifyRowsBelow > 0 || in.NotifyScoreBelow > 0 || in.NotifyRunTimeAboveMinutes > 0 ||
		in.NotifyRunsWithoutData > 0 || in.NotifyDaysWithoutData > 0
}

// hasSizingFields reports whether any manual Spark sizing field is set. sizingNumPartitions is a
// load option rather than part of the sizing block, so it is deliberately not included here.
func hasSizingFields(in Input) bool {
	return in.SizingMaxExecutors > 0 || in.SizingExecutorCores > 0 || strings.TrimSpace(in.SizingExecutorMemoryGb) != "" ||
		in.SizingDriverCores > 0 || strings.TrimSpace(in.SizingDriverMemoryGb) != "" || strings.TrimSpace(in.SizingMemoryOverheadGb) != "" ||
		in.SizingNumPartitions != 0
}

func hasParallelJdbcFields(in Input) bool {
	return strings.TrimSpace(in.ParallelJdbcMode) != "" || strings.TrimSpace(in.ParallelJdbcPartitionColumn) != "" ||
		in.ParallelJdbcPartitionNumber != 0
}

func monitorsIncludeDescriptiveStats(keys []string) bool {
	for _, k := range keys {
		if strings.EqualFold(strings.TrimSpace(k), "descriptiveStatistics") {
			return true
		}
	}
	return false
}

// validateDateFormat rejects a dateFormat that contradicts the run-date values it would apply to,
// which the server would otherwise reject with an opaque 400.
func validateDateFormat(in Input) error {
	format := strings.ToUpper(strings.TrimSpace(in.DateFormat))
	if format != "" && format != "DATE" && format != "TIMESTAMP" {
		return fmt.Errorf("dateFormat %q is invalid (use DATE or TIMESTAMP)", in.DateFormat)
	}
	for name, value := range map[string]string{"runDate": in.RunDate, "runDateEnd": in.RunDateEnd} {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		kind := runDateKind(value)
		if format != "" && format != kind {
			return fmt.Errorf("%s %q is a %s value but dateFormat is %s", name, value, kind, format)
		}
	}
	return nil
}

// runDateKind classifies a run-date value the way the public RunDateValue discriminator does: a
// date-time (with a 'T') is a TIMESTAMP, a bare date is a DATE.
func runDateKind(value string) string {
	if strings.Contains(strings.ToUpper(value), "T") {
		return "TIMESTAMP"
	}
	return "DATE"
}

// buildSchedule validates the schedule inputs up front. It returns nil both when no schedule change
// was asked for and for NEVER, which needs the fetched job to switch the existing schedule off.
func buildSchedule(in Input) (*clients.DqSchedulingSettings, error) {
	repeat := strings.ToUpper(strings.TrimSpace(in.ScheduleRepeat))
	if repeat == "" || repeat == "NEVER" {
		return nil, nil
	}
	return clients.BuildSchedulingSettings(clients.DqScheduleInput{
		Repeat:        in.ScheduleRepeat,
		RunTime:       in.ScheduleRunTime,
		DaysOfWeek:    in.ScheduleDaysOfWeek,
		DayOfMonth:    in.ScheduleDayOfMonth,
		MonthlyMode:   in.ScheduleMonthlyMode,
		RunDateOffset: in.RunDateOffset,
	})
}

// buildNotifications rebuilds the job's whole notification object from the notify* inputs, or returns
// (nil, nil) when the caller didn't touch notifications. A non-nil second return is a terminal Output.
// Mirrors create_data_quality_job: the invoking user is always a recipient, and the public channel
// carries platform USERNAMES rather than UUIDs.
func buildNotifications(ctx context.Context, collibraClient *http.Client, in Input, jobName string) (*clients.DqJobNotifications, *Output) {
	if !hasNotifyFields(in) {
		return nil, nil
	}
	keys := in.Notify
	if len(keys) == 0 {
		keys = clients.DefaultNotificationKeys()
	}
	notifyMessages := map[string]string{}
	for k, v := range in.NotifyMessages {
		notifyMessages[strings.ToLower(strings.TrimSpace(k))] = v
	}
	opts, unknown := clients.BuildNotificationOptions(keys, map[string]int{
		"rowsbelow":       in.NotifyRowsBelow,
		"scorebelow":      in.NotifyScoreBelow,
		"runtimeabove":    in.NotifyRunTimeAboveMinutes,
		"runswithoutdata": in.NotifyRunsWithoutData,
		"dayswithoutdata": in.NotifyDaysWithoutData,
	}, notifyMessages)
	if len(unknown) > 0 {
		return nil, &Output{
			Status:   StatusNeedsInput,
			JobName:  jobName,
			Message:  fmt.Sprintf("Unknown notification(s): %s.", strings.Join(unknown, ", ")),
			Guidance: "Use notification keys from: " + strings.Join(clients.NotificationKeys(), ", ") + ".",
		}
	}

	var recipients []string
	seenUser := map[string]bool{}
	addUser := func(username string) {
		username = strings.TrimSpace(username)
		if username != "" && !seenUser[username] {
			seenUser[username] = true
			recipients = append(recipients, username)
		}
	}
	if cu, err := clients.GetCurrentUser(ctx, collibraClient); err == nil && cu != nil {
		addUser(cu.UserName)
	}
	res, err := clients.ResolveNotificationRecipients(ctx, collibraClient, in.NotifyRecipients)
	if err != nil {
		return nil, &Output{
			Status:   StatusError,
			JobName:  jobName,
			Message:  fmt.Sprintf("Failed to resolve notification recipients: %v", err),
			Guidance: "Check the recipient usernames/emails and retry.",
		}
	}
	if len(res.Unresolved) > 0 && !in.NotifyProceedWithoutUnresolved {
		return nil, &Output{
			Status:   StatusNeedsInput,
			JobName:  jobName,
			Message:  fmt.Sprintf("These notification recipients have no active Collibra account: %s.", strings.Join(res.Unresolved, ", ")),
			Guidance: "Fix the username/email, or set notifyProceedWithoutUnresolved=true to update anyway with the valid recipients (the unresolved ones are dropped).",
		}
	}
	for _, username := range res.Usernames {
		addUser(username)
	}
	useIndividual := false
	for _, o := range opts {
		if o.Message != "" {
			useIndividual = true
			break
		}
	}
	return &clients.DqJobNotifications{
		NotificationOptions:   opts,
		GlobalMessage:         strings.TrimSpace(in.NotifyMessage),
		UseIndividualMessages: useIndividual,
		Channels:              []clients.DqNotificationChannel{{Channel: "EMAIL", Recipients: recipients}},
	}, nil
}

// buildRequest composes the PATCH body: only the parts the caller touched, with the fetched job
// supplying what the whole-object-replace fields need (the data-location overlay, and the current
// schedule when it is being switched off). Warnings collect the non-fatal caveats worth showing.
func buildRequest(in Input, job *clients.DqJobDefinition, schedule *clients.DqSchedulingSettings, notifications *clients.DqJobNotifications) (*clients.UpdateDqJobRequest, []string, error) {
	var warnings []string
	// The body's jobName identifies the job being updated, alongside the path parameter.
	request := &clients.UpdateDqJobRequest{
		JobName:       strings.TrimSpace(in.JobName),
		SourceQuery:   strings.TrimSpace(in.SourceQuery),
		Notifications: notifications,
	}
	if notifications != nil && job.Notifications != nil {
		warnings = append(warnings, "Notifications are replaced wholesale, not merged: the job's existing notification options, messages and recipients are discarded in favour of the ones in this request.")
	}

	if kind := strings.TrimSpace(in.RunDate); kind != "" {
		request.RunDate = &clients.DqPublicRunDate{Kind: runDateKind(kind), Value: kind}
	}
	if kind := strings.TrimSpace(in.RunDateEnd); kind != "" {
		request.RunDateEnd = &clients.DqPublicRunDate{Kind: runDateKind(kind), Value: kind}
	}

	if hasLocationChange(in) {
		// dataLocation is a whole-object replace requiring all five fields, so overlay onto the current
		// location. databaseProductName is readOnly server-side and deliberately not sent.
		location := clients.DqDataLocation{
			EdgeSiteName:       orCurrent(in.EdgeSiteName, job.DataLocation.EdgeSiteName),
			EdgeConnectionName: orCurrent(in.ConnectionName, job.DataLocation.EdgeConnectionName),
			DataSourceName:     orCurrent(in.DataSourceName, job.DataLocation.DataSourceName),
			SchemaName:         orCurrent(in.SchemaName, job.DataLocation.SchemaName),
			TableName:          orCurrent(in.TableName, job.DataLocation.TableName),
		}
		if missing := missingLocationFields(location); len(missing) > 0 {
			return nil, nil, fmt.Errorf("the job's current data location is missing %s, so it cannot be partially updated — supply %s explicitly", strings.Join(missing, ", "), strings.Join(missing, ", "))
		}
		request.DataLocation = &location
		if strings.TrimSpace(in.SourceQuery) == "" && (strings.TrimSpace(in.SchemaName) != "" || strings.TrimSpace(in.TableName) != "") {
			warnings = append(warnings, "The data location's schema/table changed but sourceQuery was not: the scan SQL still names the old schema/table. Set sourceQuery to match the new location.")
		}
	}

	// Schedule: an explicit NEVER switches the existing schedule off (nothing can be nulled), which
	// needs the job's current settings resent with isActive=false.
	if strings.EqualFold(strings.TrimSpace(in.ScheduleRepeat), "NEVER") {
		if job.SchedulingSettings == nil {
			warnings = append(warnings, "scheduleRepeat=NEVER was requested but the job has no schedule, so there was nothing to switch off.")
		} else if !job.SchedulingSettings.IsActive {
			warnings = append(warnings, "scheduleRepeat=NEVER was requested but the job's schedule is already inactive.")
		} else {
			disabled := *job.SchedulingSettings
			disabled.IsActive = false
			request.SchedulingSettings = &disabled
		}
	} else {
		request.SchedulingSettings = schedule
	}

	monitoring, err := buildMonitoringSettings(in)
	if err != nil {
		return nil, nil, err
	}
	request.MonitoringSettings = monitoring

	jobSettings, settingsWarnings, err := buildJobSettings(in, job.JobType)
	if err != nil {
		return nil, nil, err
	}
	request.JobSettings = jobSettings
	warnings = append(warnings, settingsWarnings...)

	return request, warnings, nil
}

func orCurrent(value, current string) string {
	if v := strings.TrimSpace(value); v != "" {
		return v
	}
	return current
}

func missingLocationFields(l clients.DqDataLocation) []string {
	var missing []string
	for name, value := range map[string]string{
		"edgeSiteName": l.EdgeSiteName, "connectionName": l.EdgeConnectionName,
		"dataSourceName": l.DataSourceName, "schemaName": l.SchemaName, "tableName": l.TableName,
	} {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, name)
		}
	}
	sortStrings(missing)
	return missing
}

// buildMonitoringSettings sends adaptiveMonitors only when the caller touched monitors or the
// adaptive tuning. The toggle set is authoritative (as on create), while dataLookback/learningPhase
// are sent individually so changing one leaves the other alone.
func buildMonitoringSettings(in Input) (*clients.DqMonitoringSettingsPatch, error) {
	if len(in.Monitors) == 0 && in.DataLookback == 0 && in.LearningPhase == 0 {
		return nil, nil
	}
	var monitors *clients.DqAdaptiveMonitorsPatch
	if len(in.Monitors) > 0 {
		profile, _ := clients.BuildProfileMonitors(in.Monitors) // unknown keys already rejected
		monitors = clients.PatchAdaptiveMonitorsFromProfile(profile)
	} else {
		// Only the adaptive tuning changed. The toggles are non-omitempty (authoritative on the wire),
		// so they must not be sent here — that would silently turn every monitor off.
		monitors = nil
	}
	settings := &clients.DqAdaptiveMonitorSettingsPatch{}
	if in.DataLookback > 0 {
		settings.DataLookBack = chip.Ptr(in.DataLookback)
	}
	if in.LearningPhase > 0 {
		settings.LearningPhase = chip.Ptr(in.LearningPhase)
	}
	if settings.DataLookBack == nil && settings.LearningPhase == nil {
		settings = nil
	}
	if monitors == nil && settings == nil {
		return nil, nil
	}
	if monitors == nil {
		// adaptiveMonitors is required to carry `settings`, but its toggles are authoritative — so a
		// settings-only change cannot be expressed without also restating the monitor selection.
		return nil, fmt.Errorf("dataLookback/learningPhase are part of the adaptive-monitor settings, which cannot be changed without also stating the monitor selection — pass monitors too (the confirm_required diff shows the current set)")
	}
	monitors.Settings = settings
	return &clients.DqMonitoringSettingsPatch{AdaptiveMonitors: monitors}, nil
}

// buildJobSettings maps the compute/sizing inputs onto jobSettings, rejecting the ones that don't
// apply to the job's (immutable) type. Only the touched leaves are sent.
func buildJobSettings(in Input, jobType string) (*clients.DqJobSettingsPatch, []string, error) {
	pushdownTouched := in.PushdownConnections > 0 || in.PushdownThreads > 0
	pullupTouched := hasSizingFields(in) || hasParallelJdbcFields(in) || len(in.SparkSqlProperties) > 0
	dateFormat := strings.ToUpper(strings.TrimSpace(in.DateFormat))

	isPushdown := strings.EqualFold(jobType, "PUSHDOWN")
	isPullup := strings.EqualFold(jobType, "PULLUP")
	if pushdownTouched && isPullup {
		return nil, nil, fmt.Errorf("pushdownConnections/pushdownThreads apply to PUSHDOWN jobs, but %q is a PULLUP job", jobType)
	}
	if pullupTouched && isPushdown {
		return nil, nil, fmt.Errorf("sizing*/parallelJdbc*/sparkSqlProperties apply to PULLUP jobs, but %q is a PUSHDOWN job", jobType)
	}

	if !pushdownTouched && !pullupTouched && dateFormat == "" {
		return nil, nil, nil
	}
	settings := &clients.DqJobSettingsPatch{DateFormat: dateFormat}

	if pushdownTouched {
		if in.PushdownConnections != 0 && (in.PushdownConnections < 1 || in.PushdownConnections > 50) {
			return nil, nil, fmt.Errorf("pushdownConnections must be between 1 and 50 (got %d)", in.PushdownConnections)
		}
		if in.PushdownThreads != 0 && (in.PushdownThreads < 1 || in.PushdownThreads > 10) {
			return nil, nil, fmt.Errorf("pushdownThreads must be between 1 and 10 (got %d)", in.PushdownThreads)
		}
		pushdown := &clients.DqPushdownSettingsPatch{}
		if in.PushdownConnections > 0 {
			pushdown.Connections = chip.Ptr(in.PushdownConnections)
		}
		if in.PushdownThreads > 0 {
			pushdown.Threads = chip.Ptr(in.PushdownThreads)
		}
		settings.PushdownSettings = pushdown
	}

	var warnings []string
	if pullupTouched {
		pullup := &clients.DqPullupSettingsPatch{SparkSqlProperties: in.SparkSqlProperties}
		parallelJdbc, err := buildParallelJdbc(in)
		if err != nil {
			return nil, nil, err
		}
		if in.SizingNumPartitions != 0 || parallelJdbc != nil {
			loadOptions := &clients.DqLoadOptionsPatch{ParallelJdbcOptions: parallelJdbc}
			switch {
			case in.SizingNumPartitions < 0:
				loadOptions.NumPartitions = chip.Ptr(0) // explicit reset to "let Spark decide"
			case in.SizingNumPartitions > 0:
				loadOptions.NumPartitions = chip.Ptr(in.SizingNumPartitions)
			}
			pullup.LoadOptions = loadOptions
		}
		sizing, err := buildSparkJobSizing(in)
		if err != nil {
			return nil, nil, err
		}
		pullup.SparkJobSizing = sizing
		if sizing != nil {
			warnings = append(warnings, "Setting any sizing* field switches the job to MANUAL Spark sizing and sends a complete sizing block, using 1 for the fields you left unset — supply them all to avoid shrinking the job's resources.")
		}
		settings.PullupSettings = pullup
	}
	return settings, warnings, nil
}

// buildSparkJobSizing returns nil unless a manual sizing field was set, in which case a complete
// manual block is sent (the API has no per-field sizing patch) using the wizard default of 1 for the
// unset fields. Memory GB strings are parsed to integers (SparkMemoryGB).
func buildSparkJobSizing(in Input) (*clients.DqPublicSparkJobSizing, error) {
	if !hasManualSizing(in) {
		return nil, nil
	}
	execMem, err := gbToInt(in.SizingExecutorMemoryGb, 1)
	if err != nil {
		return nil, fmt.Errorf("sizingExecutorMemoryGb %w", err)
	}
	driverMem, err := gbToInt(in.SizingDriverMemoryGb, 1)
	if err != nil {
		return nil, fmt.Errorf("sizingDriverMemoryGb %w", err)
	}
	overhead, err := gbToInt(in.SizingMemoryOverheadGb, 1)
	if err != nil {
		return nil, fmt.Errorf("sizingMemoryOverheadGb %w", err)
	}
	return &clients.DqPublicSparkJobSizing{
		NumExecutors:     orDefaultInt(in.SizingMaxExecutors, 1),
		NumExecutorCores: orDefaultInt(in.SizingExecutorCores, 1),
		ExecutorMemoryGb: execMem,
		DriverCores:      orDefaultInt(in.SizingDriverCores, 1),
		DriverMemoryGb:   driverMem,
		MemoryOverheadGb: overhead,
	}, nil
}

// hasManualSizing reports whether a manual Spark sizing field was set. Unlike hasSizingFields it
// excludes sizingNumPartitions, which is a load option rather than part of the sizing block.
func hasManualSizing(in Input) bool {
	return in.SizingMaxExecutors > 0 || in.SizingExecutorCores > 0 || strings.TrimSpace(in.SizingExecutorMemoryGb) != "" ||
		in.SizingDriverCores > 0 || strings.TrimSpace(in.SizingDriverMemoryGb) != "" || strings.TrimSpace(in.SizingMemoryOverheadGb) != ""
}

// gbToInt parses a GB string to a positive integer (the public SparkMemoryGB is an integer;
// fractional values are not supported). An empty string uses def.
func gbToInt(s string, def int) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return def, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("must be an integer number of GB, got %q", s)
	}
	if v < 1 {
		return 0, fmt.Errorf("must be >= 1 GB, got %q", s)
	}
	return v, nil
}

// buildParallelJdbc maps the Parallel JDBC inputs onto the ParallelJdbcOptions contract, inferring
// the mode when omitted and enforcing the wizard's rules: AUTO needs nothing; AUTO_COLUMN requires a
// partition count (and no column); MANUAL requires both. Returns (nil, nil) when not configured.
func buildParallelJdbc(in Input) (*clients.DqParallelJdbcOptions, error) {
	mode := strings.ToUpper(strings.TrimSpace(in.ParallelJdbcMode))
	col := strings.TrimSpace(in.ParallelJdbcPartitionColumn)
	num := in.ParallelJdbcPartitionNumber
	if mode == "" {
		switch {
		case col == "" && num == 0:
			return nil, nil // not configured
		case col != "":
			mode = clients.ParallelJdbcManual
		default:
			mode = clients.ParallelJdbcAutoColumn
		}
	}
	switch mode {
	case clients.ParallelJdbcAuto:
		return &clients.DqParallelJdbcOptions{Mode: clients.ParallelJdbcAuto}, nil
	case clients.ParallelJdbcAutoColumn:
		if col != "" {
			return nil, fmt.Errorf("parallelJdbcMode AUTO_COLUMN auto-selects the partition column — omit parallelJdbcPartitionColumn, or use MANUAL to choose one")
		}
		if num <= 0 {
			return nil, fmt.Errorf("parallelJdbcMode AUTO_COLUMN requires parallelJdbcPartitionNumber (auto-calculate is off once a partition count is set)")
		}
		return &clients.DqParallelJdbcOptions{Mode: clients.ParallelJdbcAutoColumn, PartitionNumber: num}, nil
	case clients.ParallelJdbcManual:
		if col == "" {
			return nil, fmt.Errorf("parallelJdbcMode MANUAL requires parallelJdbcPartitionColumn")
		}
		if num <= 0 {
			return nil, fmt.Errorf("parallelJdbcMode MANUAL requires a manual parallelJdbcPartitionNumber (auto-calculate is not allowed with a specific partition column)")
		}
		return &clients.DqParallelJdbcOptions{Mode: clients.ParallelJdbcManual, PartitionColumn: col, PartitionNumber: num}, nil
	default:
		return nil, fmt.Errorf("parallelJdbcMode %q is invalid (use AUTO, AUTO_COLUMN, or MANUAL)", mode)
	}
}

func orDefaultInt(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

// diff describes what the request would actually change, comparing each part of the composed body
// against the job as it stands. A part whose proposed value already matches is left out, so an empty
// result means the update would be a no-op.
func diff(job *clients.DqJobDefinition, request *clients.UpdateDqJobRequest) []FieldChange {
	var changes []FieldChange
	add := func(field, current, proposed string) {
		if current == proposed {
			return
		}
		changes = append(changes, FieldChange{Field: field, Current: orNone(current), Proposed: proposed})
	}

	if request.SourceQuery != "" {
		add("sourceQuery", job.SourceQuery, request.SourceQuery)
	}
	if request.RunDate != nil {
		add("runDate", describeRunDate(job.RunDate), describeRunDate(request.RunDate))
	}
	if request.RunDateEnd != nil {
		add("runDateEnd", describeRunDate(job.RunDateEnd), describeRunDate(request.RunDateEnd))
	}
	if request.DataLocation != nil {
		add("dataLocation", describeLocation(&job.DataLocation), describeLocation(request.DataLocation))
	}
	if request.SchedulingSettings != nil {
		add("schedule", describeSchedule(job.SchedulingSettings), describeSchedule(request.SchedulingSettings))
	}
	if request.MonitoringSettings != nil && request.MonitoringSettings.AdaptiveMonitors != nil {
		am := request.MonitoringSettings.AdaptiveMonitors
		var current *clients.DqPublicAdaptiveMonitors
		if job.MonitoringSettings != nil {
			current = job.MonitoringSettings.AdaptiveMonitors
		}
		add("monitors", strings.Join(clients.EnabledPublicMonitorKeys(current), ", "), strings.Join(patchMonitorKeys(am), ", "))
		if am.Settings != nil {
			var currentSettings *clients.DqPublicAdaptiveMonitorSettings
			if current != nil {
				currentSettings = current.Settings
			}
			if am.Settings.DataLookBack != nil {
				add("dataLookback", describeSetting(currentSettings, true), strconv.Itoa(*am.Settings.DataLookBack))
			}
			if am.Settings.LearningPhase != nil {
				add("learningPhase", describeSetting(currentSettings, false), strconv.Itoa(*am.Settings.LearningPhase))
			}
		}
	}
	if request.Notifications != nil {
		add("notifications", describeNotifications(job.Notifications), describeNotifications(request.Notifications))
	}
	if request.JobSettings != nil {
		changes = append(changes, jobSettingsChanges(job, request.JobSettings)...)
	}
	return changes
}

// jobSettingsChanges diffs the compute/sizing block. The nested patch leaves are compared one by one
// so a change to a single knob doesn't read as a wholesale settings replacement.
func jobSettingsChanges(job *clients.DqJobDefinition, proposed *clients.DqJobSettingsPatch) []FieldChange {
	var changes []FieldChange
	current := job.JobSettings
	add := func(field, cur, prop string) {
		if cur == prop {
			return
		}
		changes = append(changes, FieldChange{Field: field, Current: orNone(cur), Proposed: prop})
	}

	if proposed.DateFormat != "" {
		cur := ""
		if current != nil {
			cur = current.DateFormat
		}
		add("dateFormat", cur, proposed.DateFormat)
	}
	if p := proposed.PushdownSettings; p != nil {
		var cur *clients.DqPublicPushdownSettings
		if current != nil {
			cur = current.PushdownSettings
		}
		curConnections, curThreads := "", ""
		if cur != nil {
			curConnections, curThreads = strconv.Itoa(cur.Connections), strconv.Itoa(cur.Threads)
		}
		if p.Connections != nil {
			add("pushdownConnections", curConnections, strconv.Itoa(*p.Connections))
		}
		if p.Threads != nil {
			add("pushdownThreads", curThreads, strconv.Itoa(*p.Threads))
		}
	}
	if p := proposed.PullupSettings; p != nil {
		var cur *clients.DqPublicPullupSettings
		if current != nil {
			cur = current.PullupSettings
		}
		if p.LoadOptions != nil {
			if p.LoadOptions.NumPartitions != nil {
				curPartitions := ""
				if cur != nil && cur.LoadOptions != nil {
					curPartitions = strconv.Itoa(cur.LoadOptions.NumPartitions)
				}
				add("sizingNumPartitions", curPartitions, strconv.Itoa(*p.LoadOptions.NumPartitions))
			}
			if p.LoadOptions.ParallelJdbcOptions != nil {
				var curPj *clients.DqParallelJdbcOptions
				if cur != nil && cur.LoadOptions != nil {
					curPj = cur.LoadOptions.ParallelJdbcOptions
				}
				add("parallelJdbc", describeParallelJdbc(curPj), describeParallelJdbc(p.LoadOptions.ParallelJdbcOptions))
			}
		}
		if p.SparkJobSizing != nil {
			var curSizing *clients.DqPublicSparkJobSizing
			if cur != nil {
				curSizing = cur.SparkJobSizing
			}
			add("sparkJobSizing", describeSizing(curSizing), describeSizing(p.SparkJobSizing))
		}
		if len(p.SparkSqlProperties) > 0 {
			curProps := map[string]string{}
			if cur != nil {
				curProps = cur.SparkSqlProperties
			}
			add("sparkSqlProperties", describeProperties(curProps), describeProperties(p.SparkSqlProperties))
		}
	}
	return changes
}

func describeRunDate(rd *clients.DqPublicRunDate) string {
	if rd == nil {
		return ""
	}
	return fmt.Sprintf("%s (%s)", rd.Value, rd.Kind)
}

func describeLocation(l *clients.DqDataLocation) string {
	if l == nil {
		return ""
	}
	return fmt.Sprintf("site=%s connection=%s dataSource=%s schema=%s table=%s",
		l.EdgeSiteName, l.EdgeConnectionName, l.DataSourceName, l.SchemaName, l.TableName)
}

// describeSchedule renders a schedule for the diff, including whether it is active — the difference
// between an active and an inactive schedule is exactly what scheduleRepeat=NEVER changes.
func describeSchedule(s *clients.DqSchedulingSettings) string {
	if s == nil {
		return ""
	}
	state := "active"
	if !s.IsActive {
		state = "inactive"
	}
	desc := fmt.Sprintf("%s at %s UTC (%s)", s.SchedulerMode, s.ScheduledRunTime, state)
	switch {
	case s.Hourly != nil:
		desc += fmt.Sprintf(", offset %s", s.Hourly.HourlyOffset)
	case s.Daily != nil:
		desc += fmt.Sprintf(", days %s, offset %s", strings.Join(s.Daily.DaysOfWeek, "/"), s.Daily.DailyOffset)
	case s.Monthly != nil:
		desc += fmt.Sprintf(", repeat %s", s.Monthly.MonthlyRepeat)
		if s.Monthly.DayNumber > 0 {
			desc += fmt.Sprintf(" day %d", s.Monthly.DayNumber)
		}
		desc += fmt.Sprintf(", offset %s", s.Monthly.MonthlyOffset)
	}
	return desc
}

func describeNotifications(n *clients.DqJobNotifications) string {
	if n == nil {
		return ""
	}
	var enabled []string
	for _, o := range n.NotificationOptions {
		if o.Enabled {
			if o.Quantity > 0 {
				enabled = append(enabled, fmt.Sprintf("%s(%d)", o.NotificationType, o.Quantity))
			} else {
				enabled = append(enabled, o.NotificationType)
			}
		}
	}
	var recipients []string
	for _, c := range n.Channels {
		recipients = append(recipients, c.Recipients...)
	}
	return fmt.Sprintf("alerts [%s] to [%s]", strings.Join(enabled, ", "), strings.Join(recipients, ", "))
}

func describeParallelJdbc(pj *clients.DqParallelJdbcOptions) string {
	if pj == nil {
		return ""
	}
	desc := pj.Mode
	if pj.PartitionColumn != "" {
		desc += " on " + pj.PartitionColumn
	}
	if pj.PartitionNumber > 0 {
		desc += fmt.Sprintf(" x%d", pj.PartitionNumber)
	}
	return desc
}

func describeSizing(s *clients.DqPublicSparkJobSizing) string {
	if s == nil {
		return "automatic"
	}
	return fmt.Sprintf("manual: executors=%d executorCores=%d executorMemoryGb=%d driverCores=%d driverMemoryGb=%d memoryOverheadGb=%d",
		s.NumExecutors, s.NumExecutorCores, s.ExecutorMemoryGb, s.DriverCores, s.DriverMemoryGb, s.MemoryOverheadGb)
}

func describeProperties(props map[string]string) string {
	if len(props) == 0 {
		return ""
	}
	pairs := make([]string, 0, len(props))
	for k, v := range props {
		pairs = append(pairs, k+"="+v)
	}
	sortStrings(pairs)
	return strings.Join(pairs, " ")
}

// describeSetting renders the job's current dataLookBack (lookback=true) or learningPhase.
func describeSetting(s *clients.DqPublicAdaptiveMonitorSettings, lookback bool) string {
	if s == nil {
		return ""
	}
	if lookback {
		return strconv.Itoa(s.DataLookBack)
	}
	return strconv.Itoa(s.LearningPhase)
}

// patchMonitorKeys lists the monitor keys the patch would leave enabled, in catalog order.
func patchMonitorKeys(am *clients.DqAdaptiveMonitorsPatch) []string {
	if am == nil {
		return nil
	}
	return clients.EnabledPublicMonitorKeys(&clients.DqPublicAdaptiveMonitors{
		DescriptiveStatistics: am.DescriptiveStatistics,
		EmptyFields:           am.EmptyFields,
		ExecutionTime:         am.ExecutionTime,
		Max:                   am.Max,
		Mean:                  am.Mean,
		Min:                   am.Min,
		NullValues:            am.NullValues,
		RowCount:              am.RowCount,
		Uniqueness:            am.Uniqueness,
	})
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(none)"
	}
	return s
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

func lookupError(code int, err error, jobName string) Output {
	out := Output{Status: StatusError, JobName: jobName}
	switch code {
	case http.StatusNotFound:
		out.Message = fmt.Sprintf("No data-quality job named %q was found (HTTP 404).", jobName)
		out.Guidance = "Verify the job name — it may be misspelled, or the job may have been deleted. Only existing jobs can be updated; use create_data_quality_job to make a new one."
	case http.StatusUnauthorized:
		out.Message = "Not authenticated to the data-quality API (HTTP 401)."
		out.Guidance = "Your Collibra session/token is missing or expired — re-authenticate and retry."
	case http.StatusForbidden:
		out.Message = fmt.Sprintf("You do not have permission to view job %q (HTTP 403).", jobName)
		out.Guidance = "You need the Data Quality Job > View permission on the connection. Ask an administrator for the Data Quality Editor/Manager role, then retry."
	case http.StatusBadRequest:
		out.Message = fmt.Sprintf("The data-quality API rejected the lookup of job %q (HTTP 400): %v", jobName, err)
		out.Guidance = "Check that the job name is correct and well-formed, then retry."
	case 0:
		out.Message = fmt.Sprintf("Failed to look up job %q: %v", jobName, err)
		out.Guidance = "A network/transport error occurred contacting the data-quality API. Nothing was changed. Retry."
	default:
		out.Message = fmt.Sprintf("Failed to look up job %q (HTTP %d): %v", jobName, code, err)
		out.Guidance = "This is likely a server-side error. Nothing was changed. Retry shortly; if it persists, contact your Collibra administrator."
	}
	return out
}

func updateError(code int, err error, jobName string) Output {
	out := Output{Status: StatusError, JobName: jobName}
	switch code {
	case http.StatusBadRequest:
		out.Message = fmt.Sprintf("The data-quality API rejected the update of job %q (HTTP 400): %v", jobName, err)
		out.Guidance = "The request was invalid — check the message for the offending field. Common causes: a runDate/runDateEnd window that isn't increasing, a dateFormat that doesn't match the run-date values, sizing or compute settings that don't apply to this job's type, or a data location that no longer exists."
	case http.StatusUnauthorized:
		out.Message = "Not authenticated to the data-quality API (HTTP 401)."
		out.Guidance = "Your Collibra session/token is missing or expired — re-authenticate and retry. The job was not changed."
	case http.StatusForbidden:
		out.Message = fmt.Sprintf("You do not have permission to update job %q (HTTP 403).", jobName)
		out.Guidance = "You need the Data Quality Job > Edit permission (DATA_QUALITY_JOB_EDIT), or Resource Manage All. Ask an administrator for the Data Quality Editor/Manager role, then retry."
	case http.StatusNotFound:
		out.Message = fmt.Sprintf("No data-quality job named %q was found (HTTP 404).", jobName)
		out.Guidance = "The job may have been deleted between the lookup and the update. Verify the job name."
	case 0:
		out.Message = fmt.Sprintf("Update failed: %v", err)
		out.Guidance = "A network/transport error occurred contacting the data-quality API. The update may or may not have been applied — re-run this tool with confirm=false to read the job back before retrying."
	default:
		out.Message = fmt.Sprintf("The data-quality service failed to update job %q (HTTP %d): %v", jobName, code, err)
		out.Guidance = "This is likely a server-side error. Retry shortly; if it persists, contact your Collibra administrator."
	}
	return out
}
