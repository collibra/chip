// Package create_dq_job implements the create_dq_job MCP tool — it creates a
// Collibra data-quality job for a table and (per the wizard) queues a run.
//
// IT POSTS TO THE PUBLIC DQ API: POST /rest/dq/1.0/jobs (clients.CreateDqJob /
// clients.CreateDqJobRequest, contract dq/udq-app-client/oas/dq-v1-public-oas-spec.yaml).
// The public create accepts the full job definition — sourceQuery, runDate window, monitors,
// schedule, back-runs, notifications, and pullup/pushdown settings — so the internal BFF
// endpoint is no longer needed. (Discovery in prepare_create_dq_job still uses internal
// endpoints; the public DQ API exposes no connection/edge metadata browse.)
//
// HOW SCHEDULING + TIME-SLICE + ${rd} FIT TOGETHER:
// A scheduled job re-runs on a cadence; each run scans one date-column slice. The slice lives
// in the job SQL's WHERE as `<col> >= '${rd}' AND <col> < '${rdEnd}'`. The public API has NO
// structured time-slice object — instead the engine substitutes ${rd}/${rdEnd} from runDate/
// runDateEnd (formatted per jobSettings.dateFormat), and the scheduler adjusts the run date per
// run from the schedule's *Offset enums. So this tool writes the ${rd} predicate into sourceQuery
// whenever a timeSliceColumn is set and seeds runDate/runDateEnd to the most recent slice; without
// a timeSliceColumn every scheduled run rescans the whole table.
//
// It is built around a confirm checkpoint: confirm=false (default) returns a PREVIEW of
// the exact request without creating; confirm=true submits.
package create_dq_job

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

type Status string

const (
	StatusPreview    Status = "preview"
	StatusCreated    Status = "created"
	StatusNeedsInput Status = "needs_input"
	StatusError      Status = "error"
)

// Input — the five dataLocation fields are required (easiest via prepare_create_dq_job's
// `resolved` block). Time-slice / schedule / back-run fields are optional and structured.
type Input struct {
	EdgeSiteName       string `json:"edgeSiteName" jsonschema:"Edge site name. From prepare_create_dq_job resolved.edgeSiteName."`
	EdgeConnectionName string `json:"edgeConnectionName" jsonschema:"Edge connection name, e.g. 'POSTGRES-SOURCE'. The tool resolves its databaseProductName + job type from the connection for the create request."`
	DataSourceName     string `json:"dataSourceName" jsonschema:"Data source / database, e.g. 'postgres'."`
	SchemaName         string `json:"schemaName" jsonschema:"Schema, e.g. 'sales'."`
	TableName          string `json:"tableName" jsonschema:"Table, e.g. 'transactions'."`

	JobType string `json:"jobType,omitempty" jsonschema:"PUSHDOWN or PULLUP. If omitted, resolved from the connection's capabilities."`
	JobName string `json:"jobName,omitempty" jsonschema:"Job name to assign — offer the user the chance to set this. Defaults to '<schema>.<table>'."`

	// --- Column selection. Omit to monitor ALL columns (the default). Provide a subset to
	// scope profiling/monitoring to just those columns (each sent with selected=true). ---
	SelectedColumns []string `json:"selectedColumns,omitempty" jsonschema:"Columns to monitor. Omit for ALL columns (default). Provide a subset — exact names from prepare_create_dq_job's columns — to profile only those. Does not change the source query (still SELECT *); only scopes what is profiled."`

	// --- Monitors. Omit to use the default set; provide an authoritative set of monitor keys
	// to enable (anything not listed is turned off). descriptiveStatistics unmasks sensitive
	// data — only include it after explicit user confirmation. ---
	Monitors []string `json:"monitors,omitempty" jsonschema:"Monitor keys to enable — authoritative (anything omitted is turned OFF). Omit to use the defaults (rowCount, nullValues, emptyFields, uniqueness). Valid keys (see prepare_create_dq_job's monitors): rowCount, nullValues, emptyFields, uniqueness, min, mean, max, executionTime, descriptiveStatistics. descriptiveStatistics UNMASKS sensitive data — only include it after explicit user confirmation."`

	// --- Advanced monitor settings (adaptive behavior). Omit both to use the wizard defaults
	// (lookback 10, learning phase 4). Setting either sends a structured `settings` object. ---
	DataLookback  int `json:"dataLookback,omitempty" jsonschema:"Adaptive monitors: number of prior runs used as the baseline. Default 10 when omitted. Set with learningPhase to override the adaptive 'Advanced monitor settings'."`
	LearningPhase int `json:"learningPhase,omitempty" jsonschema:"Adaptive monitors: number of runs before adaptive monitors begin alerting. Default 4 when omitted."`

	// --- Row filter (the wizard's single-column predicate). Scopes the job to rows matching
	// "filterColumn filterOperator filterValue" (e.g. amount > 100). Omit to scan all rows.
	// This is ONE predicate — not compound AND/OR or free-form SQL (mirrors the DQ wizard). ---
	FilterColumn   string `json:"filterColumn,omitempty" jsonschema:"Column for a single-predicate row filter (the wizard's row filter). Use a name from prepare_create_dq_job's columns. Set filterColumn + filterOperator (+ filterValue) to scope the job to matching rows; omit to scan all rows."`
	FilterOperator string `json:"filterOperator,omitempty" jsonschema:"Comparison operator for the row filter: = != <> > >= < <= (the wizard's set) or LIKE; the value is treated as a single literal. For IS NULL / IS NOT NULL leave filterValue empty. Required when filterColumn is set."`
	FilterValue    string `json:"filterValue,omitempty" jsonschema:"Right-hand value for the row filter, passed BARE — do NOT add quotes. The tool wraps it in single quotes for you (matching the DQ wizard), e.g. pass US (not 'US'), 100, 2024-01-01. Leave empty for valueless operators (IS NULL / IS NOT NULL). Applied by writing it into the job's source-query WHERE."`

	// --- Row sampling. Omit to scan all matching rows; a positive size enables sampling. The
	// engine turns this into a dialect-correct RANDOM()/LIMIT query at dispatch. ---
	SampleSize int `json:"sampleSize,omitempty" jsonschema:"Row sampling: number of rows to sample. Omit (or 0) to scan ALL matching rows (default). A positive value enables sampling at that size (the engine generates a RANDOM()/LIMIT query for it)."`

	// --- Time slice (incremental scanning). Pick a DATE COLUMN; when set, the tool writes a
	// "<col> >= '${rd}' AND <col> < '${rdEnd}'" predicate into the job SQL's WHERE. The scheduler
	// substitutes ${rd}/${rdEnd} per run, so each run scans only that slice, not the whole table. ---
	TimeSliceColumn string `json:"timeSliceColumn,omitempty" jsonschema:"Date/timestamp column to slice on (e.g. 'txn_ts'). When set, the tool adds a WHERE \"<col>\" >= '${rd}' AND \"<col>\" < '${rdEnd}' predicate to the job SQL so each scheduled/backrun run scans only that slice. REQUIRED for incremental scheduling — without it, every scheduled run rescans the whole table. Pick a real date/timestamp column from prepare_create_dq_job's columns."`
	TimeSliceSize   int    `json:"timeSliceSize,omitempty" jsonschema:"Time-slice window width (default 1 when timeSliceColumn set). Combined with timeSliceUnit, e.g. size=1 unit=DAYS = one day per run."`
	TimeSliceUnit   string `json:"timeSliceUnit,omitempty" jsonschema:"HOURS | DAYS | WEEKS | MONTHS (default DAYS)."`

	// --- Schedule (recurrence). Omit/NEVER = run once now. To slice incrementally per run, set a
	// timeSliceColumn too (the schedule provides the cadence; the ${rd} window provides the slice). ---
	ScheduleRepeat      string   `json:"scheduleRepeat,omitempty" jsonschema:"NEVER (default, run once now) | HOURLY | DAILY | WEEKLY | WEEKDAYS | MONTHLY. WEEKLY needs scheduleDaysOfWeek; MONTHLY uses scheduleDayOfMonth or scheduleMonthlyMode."`
	ScheduleRunTime     string   `json:"scheduleRunTime,omitempty" jsonschema:"Scheduled run time, UTC HH:mm[:ss] (e.g. '00:01:00'). Defaults to 00:00:00 when a schedule is set."`
	ScheduleDaysOfWeek  []string `json:"scheduleDaysOfWeek,omitempty" jsonschema:"For WEEKLY: the days to run, e.g. ['MONDAY','THURSDAY'] (MONDAY..SUNDAY). Ignored for other modes (DAILY runs every day; WEEKDAYS runs Mon-Fri)."`
	ScheduleDayOfMonth  int      `json:"scheduleDayOfMonth,omitempty" jsonschema:"For MONTHLY with scheduleMonthlyMode=DAY (the default): day of month 1-28."`
	ScheduleMonthlyMode string   `json:"scheduleMonthlyMode,omitempty" jsonschema:"For MONTHLY: DAY (default — run on scheduleDayOfMonth) | FIRST (first day of month) | LAST (last day of month)."`
	RunDateOffset       string   `json:"runDateOffset,omitempty" jsonschema:"How far back the slice's run date (${rd}) is from the execution time. Default SCHEDULED (the run's own period). DAILY/WEEKLY/WEEKDAYS: SCHEDULED | ONE_DAY..SEVEN_DAYS. HOURLY: SCHEDULED | ONE_HOUR | TWO_HOURS. MONTHLY: SCHEDULED | FIRST_OF_CURRENT_MONTH | FIRST_OF_PRIOR_MONTH | LAST_OF_PRIOR_MONTH. Use a non-SCHEDULED offset so each run scans the prior, already-complete period."`

	// --- Back runs (historical backfill, walks forward one slice/run from runDate - binValue*timeBin). ---
	BackrunEnabled  bool   `json:"backrunEnabled,omitempty" jsonschema:"Enable historical backfill runs."`
	BackrunBinValue int    `json:"backrunBinValue,omitempty" jsonschema:"Number of prior bins to backfill."`
	BackrunTimeBin  string `json:"backrunTimeBin,omitempty" jsonschema:"DAY | MONTH | YEAR."`

	// --- Notifications (optional). Configured when `notify` or `notifyRecipients` is set; otherwise
	// no notifications. The invoking user is always the default recipient. ---
	Notify                         []string `json:"notify,omitempty" jsonschema:"Notification keys to enable (authoritative; omit but set notifyRecipients to use the defaults jobFailed/rowsBelow/scoreBelow/runTimeAbove). Keys (see prepare_create_dq_job notifications): jobFailed, rowsBelow, scoreBelow, runTimeAbove, jobCompleted, runsWithoutData, daysWithoutData."`
	NotifyRowsBelow                int      `json:"notifyRowsBelow,omitempty" jsonschema:"Threshold for rowsBelow — alert when row count <= this. Default 1."`
	NotifyScoreBelow               int      `json:"notifyScoreBelow,omitempty" jsonschema:"Threshold for scoreBelow — alert when score (0-100) <= this. Default 75."`
	NotifyRunTimeAboveMinutes      int      `json:"notifyRunTimeAboveMinutes,omitempty" jsonschema:"Threshold for runTimeAbove — alert when run time minutes > this. Default 60."`
	NotifyRunsWithoutData          int      `json:"notifyRunsWithoutData,omitempty" jsonschema:"Threshold for runsWithoutData — alert when runs without data >= this. Default 1."`
	NotifyDaysWithoutData          int      `json:"notifyDaysWithoutData,omitempty" jsonschema:"Threshold for daysWithoutData — alert when days without data >= this. Default 1."`
	NotifyMessage                  string   `json:"notifyMessage,omitempty" jsonschema:"Optional global message applied to the enabled notifications."`
	NotifyRecipients               []string `json:"notifyRecipients,omitempty" jsonschema:"Additional recipients by username or email (the invoking user is always included). Each is validated against active Collibra accounts; unresolved ones are reported."`
	NotifyProceedWithoutUnresolved bool     `json:"notifyProceedWithoutUnresolved,omitempty" jsonschema:"If some notifyRecipients can't be resolved to an active account, set true to create anyway with the resolvable recipients. Default false: the tool returns needs_input listing the unresolved ones so you can fix or confirm."`

	// --- Per-notification message overrides. Keyed by notification key (see prepare's notifications);
	// a per-key message overrides notifyMessage for just that notification. ---
	NotifyMessages map[string]string `json:"notifyMessages,omitempty" jsonschema:"Per-notification message overrides, keyed by notification key (jobFailed, rowsBelow, scoreBelow, runTimeAbove, jobCompleted, runsWithoutData, daysWithoutData). A per-key message overrides notifyMessage for just that notification (the wizard's individual-message mode)."`

	// --- Sizing (PULLUP only — the wizard's "Size Job Resources" step). Default = automatic
	// ("Automate job resources"). Setting ANY manual field below switches to manual sizing (advanced;
	// for large datasets). Memory fields are GB (e.g. "1", "2"). ---
	SizingMaxExecutors     int    `json:"sizingMaxExecutors,omitempty" jsonschema:"PULLUP manual sizing: number of executors. Setting any sizing* field switches off automatic sizing. Default 1 when manual sizing is engaged."`
	SizingExecutorCores    int    `json:"sizingExecutorCores,omitempty" jsonschema:"PULLUP manual sizing: cores per executor (maxExecutorCores). Default 1 when manual sizing is engaged."`
	SizingExecutorMemoryGb string `json:"sizingExecutorMemoryGb,omitempty" jsonschema:"PULLUP manual sizing: memory per executor in GB, as a string (e.g. '1'). Default '1' when manual sizing is engaged."`
	SizingDriverCores      int    `json:"sizingDriverCores,omitempty" jsonschema:"PULLUP manual sizing: driver cores. Default 1 when manual sizing is engaged."`
	SizingDriverMemoryGb   string `json:"sizingDriverMemoryGb,omitempty" jsonschema:"PULLUP manual sizing: driver memory in GB, as a string (e.g. '1'). Default '1' when manual sizing is engaged."`
	SizingMemoryOverheadGb string `json:"sizingMemoryOverheadGb,omitempty" jsonschema:"PULLUP manual sizing: memory overhead in GB, as a string (e.g. '1'). Default '1' when manual sizing is engaged."`
	SizingNumPartitions    int    `json:"sizingNumPartitions,omitempty" jsonschema:"PULLUP: number of partitions (load options). Omit/0 lets Spark decide."`

	// --- Parallel JDBC (PULLUP advanced sizing). mode AUTO (column+count auto) | AUTO_COLUMN (column
	// auto, count required) | MANUAL (column+count required). Mode is inferred when omitted: a
	// partition column implies MANUAL, a partition count alone implies AUTO_COLUMN. ---
	ParallelJdbcMode            string `json:"parallelJdbcMode,omitempty" jsonschema:"PULLUP Parallel JDBC mode: AUTO | AUTO_COLUMN | MANUAL. AUTO needs nothing else; AUTO_COLUMN requires parallelJdbcPartitionNumber; MANUAL requires both parallelJdbcPartitionColumn and parallelJdbcPartitionNumber. Omit to infer from the other fields."`
	ParallelJdbcPartitionColumn string `json:"parallelJdbcPartitionColumn,omitempty" jsonschema:"PULLUP Parallel JDBC: the column to partition on (MANUAL mode only). Setting a specific column requires a manual parallelJdbcPartitionNumber (auto-calculate is not allowed)."`
	ParallelJdbcPartitionNumber int    `json:"parallelJdbcPartitionNumber,omitempty" jsonschema:"PULLUP Parallel JDBC: number of partitions. Required for AUTO_COLUMN and MANUAL modes."`

	SparkSqlProperties map[string]string `json:"sparkSqlProperties,omitempty" jsonschema:"PULLUP: additional Spark SQL key/value properties to set on the job."`

	// --- Compute settings (PUSHDOWN only — the wizard's Review step). Defaults connections 10
	// (range 1-50), threads 2 (range 1-10). ---
	PushdownConnections int `json:"pushdownConnections,omitempty" jsonschema:"PUSHDOWN compute: number of connections (1-50). Default 10 when omitted."`
	PushdownThreads     int `json:"pushdownThreads,omitempty" jsonschema:"PUSHDOWN compute: number of threads (1-10). Default 2 when omitted."`

	// --- Catalog linkage (optional). When set (e.g. from prepare_create_dq_job's resolved table
	// asset), the success result includes a deep link to the catalog Table asset. ---
	TableAssetID string `json:"tableAssetId,omitempty" jsonschema:"Catalog Table asset UUID this job monitors (from prepare_create_dq_job). When set, the success result includes a catalog deep link to the asset."`

	// AcknowledgeDescriptiveStatistics must be true to enable the descriptiveStatistics monitor — it
	// UNMASKS sensitive values. Without it the tool refuses to proceed (the wizard's explicit-confirm).
	AcknowledgeDescriptiveStatistics bool `json:"acknowledgeDescriptiveStatistics,omitempty" jsonschema:"Required true when monitors includes descriptiveStatistics — confirms you accept that it UNMASKS sensitive values. Without it the tool refuses to proceed."`

	Confirm bool `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) returns a PREVIEW of the exact request WITHOUT creating it — review with the user. true submits."`
}

type Output struct {
	Status             Status                      `json:"status" jsonschema:"preview | created | needs_input | error."`
	Message            string                      `json:"message" jsonschema:"Human-readable outcome."`
	Request            *clients.CreateDqJobRequest `json:"request,omitempty" jsonschema:"The exact public-API payload (POST /rest/dq/1.0/jobs) that will be / was submitted. Review on preview."`
	JobName            string                      `json:"jobName,omitempty" jsonschema:"Final job name."`
	JobID              string                      `json:"jobId,omitempty" jsonschema:"Created job id (the internal create returns jobId, not a run id)."`
	JobType            string                      `json:"jobType,omitempty" jsonschema:"Resolved job type."`
	JobDetailsLink     string                      `json:"jobDetailsLink,omitempty" jsonschema:"On success: the Job Details deep-link path, relative to the Collibra instance URL (e.g. /data-quality/jobs?jobName=...)."`
	TableAssetLink     string                      `json:"tableAssetLink,omitempty" jsonschema:"On success: catalog deep-link path to the related Table asset (when tableAssetId was provided), relative to the instance URL (/asset/<uuid>)."`
	Warnings           []string                    `json:"warnings,omitempty" jsonschema:"Non-fatal warnings to surface to the user (e.g. missing Schedule/Run permission, dropped schedule)."`
	AffectedStep       string                      `json:"affectedStep,omitempty" jsonschema:"On a submission error: the configuration step the error maps to, so you can return the user there and re-call with the other inputs preserved."`
	UnsupportedOptions []string                    `json:"unsupportedOptions,omitempty" jsonschema:"Wizard options this tool still does NOT set (server defaults apply). Surface to the user."`
	Guidance           string                      `json:"guidance,omitempty" jsonschema:"On needs_input/error, what to do next."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "create_dq_job",
		Title: "Create Data Quality Job",
		Description: "Creates a Collibra data-quality job for a table (and queues a run). Provide the table location " +
			"(edgeSiteName, edgeConnectionName, dataSourceName, schemaName, tableName — easiest from prepare_create_dq_job's " +
			"`resolved`). Optional structured config: column selection (selectedColumns — subset to monitor; omit for all), monitors (monitors — " +
			"set of monitor keys to enable; omit for defaults) with adaptive settings (dataLookback/learningPhase), row sampling (sampleSize — sample N " +
			"rows; omit for all), row filter (filterColumn/filterOperator/filterValue — single-column predicate like amount > 100, inlined into the " +
			"source-query WHERE), time slice (timeSliceColumn/Size/Unit — writes a ${rd}/${rdEnd} WHERE so each run scans one slice), schedule " +
			"(scheduleRepeat/RunTime/DaysOfWeek/DayOfMonth/MonthlyMode + runDateOffset — pair with a timeSliceColumn for incremental runs), " +
			"back runs (backrun*), notifications (notify keys + thresholds + notifyRecipients — the invoking user is always a recipient). jobType resolves from the connection if omitted. " +
			"Posts to the PUBLIC DQ API (POST /rest/dq/1.0/jobs). Defaults to a PREVIEW: " +
			"confirm=false returns the exact payload without creating; call again with confirm=true after the user approves.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if missing := missingLocationFields(input); len(missing) > 0 {
			return Output{
				Status:   StatusNeedsInput,
				Message:  fmt.Sprintf("Missing required field(s): %s.", strings.Join(missing, ", ")),
				Guidance: "Call prepare_create_dq_job to resolve connection/data source/schema/table, then pass its `resolved` block here.",
			}, nil
		}

		// A row filter needs at least a column and an operator (value may be empty for
		// valueless operators like IS NULL). Catch a half-specified filter before any API call.
		if hasFilterFields(input) {
			if strings.TrimSpace(input.FilterColumn) == "" {
				return Output{Status: StatusNeedsInput, Message: "filterOperator/filterValue given without filterColumn.", Guidance: "Set filterColumn (a column from prepare_create_dq_job) to apply a row filter, or omit all filter fields."}, nil
			}
			if strings.TrimSpace(input.FilterOperator) == "" {
				return Output{Status: StatusNeedsInput, Message: "filterColumn set but filterOperator is missing.", Guidance: "Provide filterOperator (e.g. =, >, <, LIKE, IN, IS NULL), plus filterValue unless the operator takes none."}, nil
			}
		}

		// Reject unknown monitor keys before any API call.
		if len(input.Monitors) > 0 {
			if _, unknown := clients.BuildProfileMonitors(input.Monitors); len(unknown) > 0 {
				return Output{
					Status:   StatusNeedsInput,
					Message:  fmt.Sprintf("Unknown monitor(s): %s.", strings.Join(unknown, ", ")),
					Guidance: "Use monitor keys from prepare_create_dq_job's `monitors`: " + strings.Join(clients.MonitorKeys(), ", ") + ".",
				}, nil
			}
		}

		// Descriptive statistics UNMASKS sensitive data — require explicit acknowledgement (the
		// wizard's "explicit confirmation"). Hard-gate before preview or submit.
		if monitorsIncludeDescriptiveStats(input.Monitors) && !input.AcknowledgeDescriptiveStatistics {
			return Output{
				Status:       StatusNeedsInput,
				AffectedStep: stepMonitors,
				Message:      "Enabling descriptive statistics may expose sensitive values if they are present in the columns included in the scan.",
				Guidance:     "To proceed with descriptiveStatistics, set acknowledgeDescriptiveStatistics=true; otherwise remove it from monitors.",
			}, nil
		}

		// Validate + build the schedule (nil = run once now) before any API call.
		schedule, schedErr := clients.BuildSchedulingSettings(clients.DqScheduleInput{
			Repeat:        input.ScheduleRepeat,
			RunTime:       input.ScheduleRunTime,
			DaysOfWeek:    input.ScheduleDaysOfWeek,
			DayOfMonth:    input.ScheduleDayOfMonth,
			MonthlyMode:   input.ScheduleMonthlyMode,
			RunDateOffset: input.RunDateOffset,
		})
		if schedErr != nil {
			return Output{
				Status:   StatusNeedsInput,
				Message:  schedErr.Error(),
				Guidance: "Fix the schedule input: scheduleRepeat (NEVER/HOURLY/DAILY/WEEKLY/WEEKDAYS/MONTHLY), scheduleRunTime, scheduleDaysOfWeek (WEEKLY), scheduleDayOfMonth/scheduleMonthlyMode (MONTHLY), runDateOffset.",
			}, nil
		}

		// Notifications: configured only when the caller engages them (notify keys, recipients, or a
		// global message). The invoking user is always the default recipient. Unlike rowFilter/sample,
		// this structured field IS applied by the server (→ alert conditions).
		var notifications *clients.DqJobNotifications
		if len(input.Notify) > 0 || len(input.NotifyRecipients) > 0 || strings.TrimSpace(input.NotifyMessage) != "" {
			keys := input.Notify
			if len(keys) == 0 {
				keys = clients.DefaultNotificationKeys()
			}
			notifyMessages := map[string]string{}
			for k, v := range input.NotifyMessages {
				notifyMessages[strings.ToLower(strings.TrimSpace(k))] = v
			}
			opts, unknown := clients.BuildNotificationOptions(keys, map[string]int{
				"rowsbelow":       input.NotifyRowsBelow,
				"scorebelow":      input.NotifyScoreBelow,
				"runtimeabove":    input.NotifyRunTimeAboveMinutes,
				"runswithoutdata": input.NotifyRunsWithoutData,
				"dayswithoutdata": input.NotifyDaysWithoutData,
			}, notifyMessages)
			if len(unknown) > 0 {
				return Output{
					Status:   StatusNeedsInput,
					Message:  fmt.Sprintf("Unknown notification(s): %s.", strings.Join(unknown, ", ")),
					Guidance: "Use notification keys from prepare_create_dq_job's `notifications`: " + strings.Join(clients.NotificationKeys(), ", ") + ".",
				}, nil
			}
			// Recipients: invoking user (always) + any additional usernames/emails, de-duped. The public
			// notification channel takes platform USERNAMES, not UUIDs (no UUID resolution needed).
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
			res, err := clients.ResolveNotificationRecipients(ctx, collibraClient, input.NotifyRecipients)
			if err != nil {
				return Output{Status: StatusError, Message: fmt.Sprintf("Failed to resolve notification recipients: %v", err), Guidance: "Check the recipient usernames/emails and retry."}, nil
			}
			if len(res.Unresolved) > 0 && !input.NotifyProceedWithoutUnresolved {
				return Output{
					Status:   StatusNeedsInput,
					Message:  fmt.Sprintf("These notification recipients have no active Collibra account: %s.", strings.Join(res.Unresolved, ", ")),
					Guidance: "Fix the username/email, or set notifyProceedWithoutUnresolved=true to create anyway with the valid recipients (the unresolved ones are dropped).",
				}, nil
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
			notifications = &clients.DqJobNotifications{
				NotificationOptions:   opts,
				GlobalMessage:         strings.TrimSpace(input.NotifyMessage),
				UseIndividualMessages: useIndividual,
				Channels:              []clients.DqNotificationChannel{{Channel: "EMAIL", Recipients: recipients}},
			}
		}

		// Resolve connectionId + edgeSiteId (+ jobType fallback) from the connection — the
		// internal request needs the IDs, not just names. findConnectionByName hits the
		// INTERNAL connections list (clients.ListDqConnections); the public API has no
		// equivalent that returns capabilityTypes/edgeSiteId.
		conn, connErr := findConnectionByName(ctx, collibraClient, input.EdgeConnectionName)
		jobType := strings.ToUpper(strings.TrimSpace(input.JobType))
		if jobType == "" && connErr == nil && conn != nil {
			switch len(conn.CapabilityTypes) {
			case 1:
				jobType = conn.CapabilityTypes[0]
			case 0:
				return Output{Status: StatusNeedsInput, Message: fmt.Sprintf("Connection %q advertises no DQ capability.", input.EdgeConnectionName), Guidance: "Confirm a Data Quality Pushdown/Pullup capability is enabled, or set jobType."}, nil
			default:
				return Output{Status: StatusNeedsInput, Message: fmt.Sprintf("Connection %q supports multiple job types (%s).", input.EdgeConnectionName, strings.Join(conn.CapabilityTypes, ", ")), Guidance: "Set jobType explicitly."}, nil
			}
		}
		if jobType == "" {
			return Output{Status: StatusNeedsInput, Message: "Could not determine job type.", Guidance: "Set jobType to PUSHDOWN or PULLUP, or call prepare_create_dq_job."}, nil
		}
		if connErr != nil || conn == nil {
			return Output{Status: StatusNeedsInput, Message: fmt.Sprintf("Could not resolve connection %q (needed for connectionId/edgeSiteId).", input.EdgeConnectionName), Guidance: "Verify the connection name via prepare_create_dq_job."}, nil
		}

		// Job name: default from the server's collision-free generator (auto-increments, e.g. "..._2");
		// validate a user-provided name against the server's rules (rejects special chars).
		jobName := strings.TrimSpace(input.JobName)
		if jobName == "" {
			if generated, err := clients.GenerateUniqueJobName(ctx, collibraClient, input.SchemaName, input.TableName); err == nil && strings.TrimSpace(generated) != "" {
				jobName = generated
			} else {
				jobName = input.SchemaName + "." + input.TableName
			}
		} else if ok, err := clients.IsValidDqJobName(ctx, collibraClient, jobName); err == nil && !ok {
			guidance := "Use only letters, numbers, '-' and '_'."
			if suggestion, sErr := clients.GenerateUniqueJobName(ctx, collibraClient, input.SchemaName, input.TableName); sErr == nil && strings.TrimSpace(suggestion) != "" {
				guidance += fmt.Sprintf(" A valid default is %q.", suggestion)
			}
			return Output{Status: StatusNeedsInput, AffectedStep: stepSelectData, Message: fmt.Sprintf("Job name %q is invalid (special characters other than - and _ are not allowed).", jobName), Guidance: guidance}, nil
		}

		// Type-specific config must match the job type (sizing/Parallel JDBC are PULLUP-only; compute is PUSHDOWN-only).
		if err := validateTypeSpecificConfig(input, jobType); err != nil {
			return Output{Status: StatusNeedsInput, AffectedStep: stepForTypeConfigError(jobType), Message: err.Error(), Guidance: "Provide settings that match the resolved job type, or omit them."}, nil
		}

		// Build the type-specific settings (manual sizing / Parallel JDBC for PULLUP, compute for PUSHDOWN).
		jobSettings, settingsErr := buildJobSettings(input, jobType)
		if settingsErr != nil {
			return Output{Status: StatusNeedsInput, AffectedStep: stepForTypeConfigError(jobType), Message: settingsErr.Error(), Guidance: "Fix the sizing / Parallel JDBC inputs (see parallelJdbcMode rules), then retry."}, nil
		}

		// Permission preflight: DQ create/run/schedule are CONNECTION-resource permissions (granted by
		// the Data Quality Editor/Manager roles), checked as `global || resource` like the DQ UI. Hard-gate
		// on Create; degrade gracefully on Schedule (drop it) and warn on Run. Best-effort: if the
		// permission lookup itself fails, proceed with a warning and let the server enforce — never block
		// on a failed lookup.
		var warnings []string
		if global, resource, permErr := clients.GetDqConnectionPermissions(ctx, collibraClient, conn.ConnectionID); permErr == nil {
			has := func(p string) bool { return clients.HasPermission(global, p) || clients.HasPermission(resource, p) }
			manageAll := has(clients.PermResourceManageAll)
			if !manageAll && !has(clients.PermDqJobCreate) {
				return Output{
					Status:   StatusError,
					JobType:  jobType,
					Message:  fmt.Sprintf("You do not have the Data Quality Job > Create permission (DATA_QUALITY_JOB_CREATE) on connection %q required to create a DQ job.", conn.ConnectionName),
					Guidance: "Ask an administrator to grant you the Data Quality Editor or Data Quality Manager role on this connection, then retry.",
				}, nil
			}
			if schedule != nil && !manageAll && !has(clients.PermDqJobSchedule) {
				warnings = append(warnings, "You lack the Data Quality Job > Schedule permission on this connection — the schedule was dropped; the job will run once on demand.")
				schedule = nil
			}
			if !manageAll && !has(clients.PermDqJobRun) {
				warnings = append(warnings, "You lack the Data Quality Job > Run permission on this connection. This tool's create endpoint always queues a run (no save-without-run via the internal API), so the run may be rejected server-side.")
			}
		} else {
			warnings = append(warnings, fmt.Sprintf("Could not verify your DQ permissions (%v) — proceeding; the server will enforce them.", permErr))
		}

		req := buildPublicRequest(input, conn, jobType, jobName, jobSettings, schedule, notifications)

		if !input.Confirm {
			columnsDesc := "all"
			if len(input.SelectedColumns) > 0 {
				columnsDesc = fmt.Sprintf("%d selected", len(input.SelectedColumns))
			}
			filterDesc := "none"
			if strings.TrimSpace(input.FilterColumn) != "" {
				filterDesc = strings.TrimSpace(fmt.Sprintf("%s %s %s", input.FilterColumn, input.FilterOperator, input.FilterValue))
			}
			monitorKeys := input.Monitors
			if len(monitorKeys) == 0 {
				monitorKeys = clients.DefaultMonitorKeys()
			}
			pm, _ := clients.BuildProfileMonitors(monitorKeys)
			enabledMonitors := clients.EnabledMonitorKeys(pm)
			am := req.MonitoringSettings.AdaptiveMonitors
			sensitiveWarning := ""
			if am != nil && am.DescriptiveStatistics {
				sensitiveWarning = " WARNING: descriptive statistics is ON — this UNMASKS sensitive data in profiling output; confirm the user explicitly accepts this."
			}
			hasSlice := strings.TrimSpace(input.TimeSliceColumn) != ""
			rdNote := ""
			if hasSlice {
				rdNote = " The source query uses ${rd}/${rdEnd} on the time-slice column (see request.sourceQuery) so each run scans one slice; review it."
			} else if req.SchedulingSettings != nil {
				rdNote = " NOTE: no timeSliceColumn set, so every scheduled run rescans the WHOLE table (no ${rd} slice). Set timeSliceColumn for incremental runs."
			}
			sampleDesc := "all rows"
			if input.SampleSize > 0 {
				sampleDesc = fmt.Sprintf("sample %d", input.SampleSize)
			}
			adaptiveDesc := "defaults (lookback 10, learning 4)"
			if am != nil && am.Settings != nil {
				adaptiveDesc = fmt.Sprintf("lookback %d, learning %d", am.Settings.DataLookBack, am.Settings.LearningPhase)
			}
			notifyDesc := "none"
			if req.Notifications != nil {
				notifyDesc = fmt.Sprintf("%d alert(s) -> %d recipient(s)", len(req.Notifications.NotificationOptions), countRecipients(req.Notifications))
			}
			warnNote := ""
			if len(warnings) > 0 {
				warnNote = " WARNINGS: " + strings.Join(warnings, " ")
			}
			return Output{
				Status:             StatusPreview,
				JobType:            jobType,
				JobName:            jobName,
				Request:            req,
				Warnings:           warnings,
				UnsupportedOptions: clients.UnsupportedWizardOptions(jobType),
				Message: fmt.Sprintf("Preview only — nothing created. Will create a %s job %q for %s.%s (columns=%s, rows=%s, filter=%q, monitors=[%s], adaptive=%s, timeSlice=%v, schedule=%s, backrun=%v, notifications=%s, %s). "+
					"Review with the user (job name is overridable via jobName), then call again with confirm=true.%s%s%s",
					jobType, jobName, input.SchemaName, input.TableName, columnsDesc, sampleDesc, filterDesc, strings.Join(enabledMonitors, ", "), adaptiveDesc, hasSlice, describeSchedule(req.SchedulingSettings), req.Backrun != nil, notifyDesc, describeCompute(req.JobSettings, jobType), sensitiveWarning, rdNote, warnNote),
			}, nil
		}

		// confirm=true -> submit to the PUBLIC endpoint POST /rest/dq/1.0/jobs.
		resp, err := clients.CreateDqJob(ctx, collibraClient, *req)
		if err != nil {
			return Output{
				Status:       StatusError,
				JobType:      jobType,
				Request:      req,
				Warnings:     warnings,
				AffectedStep: affectedStepForError(err),
				Message:      fmt.Sprintf("Create failed: %v", err),
				Guidance:     "A validation error (400) from the public API names the offending field; affectedStep points to the most likely step — return the user there, fix it, and re-call preserving the other inputs.",
			}, nil
		}
		finalName := jobName
		if strings.TrimSpace(resp.JobName) != "" {
			finalName = resp.JobName
		}
		tableAssetLink := ""
		if id := strings.TrimSpace(input.TableAssetID); id != "" {
			tableAssetLink = clients.CatalogAssetPath(id)
		}
		msg := fmt.Sprintf("Created %s job %q (jobRunId %s). Job Details: %s", jobType, finalName, resp.JobRunID, clients.DqJobDetailsPath(finalName))
		if tableAssetLink != "" {
			msg += " | Catalog asset: " + tableAssetLink
		}
		msg += " (links are relative to your Collibra instance URL)."
		return Output{
			Status:             StatusCreated,
			JobName:            finalName,
			JobID:              resp.JobRunID,
			JobType:            jobType,
			JobDetailsLink:     clients.DqJobDetailsPath(finalName),
			TableAssetLink:     tableAssetLink,
			Warnings:           warnings,
			UnsupportedOptions: clients.UnsupportedWizardOptions(jobType),
			Message:            msg,
		}, nil
	}
}

// buildPublicRequest assembles the public JobDefinitionCreateRequest (POST /rest/dq/1.0/jobs). Column
// selection, row filter, sampling and the time-slice ${rd}/${rdEnd} predicate are all composed INTO
// sourceQuery (the public API has no structured fields for them; sourceQuery drives the scan). When a
// time slice is set, the initial runDate/runDateEnd window is seeded to the most recent slice so the
// queued run has concrete ${rd}/${rdEnd} values. Monitors map onto monitoringSettings.adaptiveMonitors.
func buildPublicRequest(input Input, conn *clients.DqConnection, jobType, jobName string, jobSettings *clients.DqPublicJobSettings, schedule *clients.DqSchedulingSettings, notifications *clients.DqJobNotifications) *clients.CreateDqJobRequest {
	// Monitors: caller's authoritative set, or the wizard defaults when omitted. Unknown keys are
	// already rejected in the handler. Mapped onto the public adaptiveMonitors shape.
	monitorKeys := input.Monitors
	if len(monitorKeys) == 0 {
		monitorKeys = clients.DefaultMonitorKeys()
	}
	profileMonitors, _ := clients.BuildProfileMonitors(monitorKeys)
	adaptive := clients.PublicAdaptiveMonitorsFromProfile(profileMonitors)

	// Adaptive monitor settings: only send `settings` if the caller customized one of them;
	// default the unset one to the wizard default (lookback 10, learning phase 4).
	if input.DataLookback > 0 || input.LearningPhase > 0 {
		lookback := input.DataLookback
		if lookback <= 0 {
			lookback = 10
		}
		learning := input.LearningPhase
		if learning <= 0 {
			learning = 4
		}
		adaptive.Settings = &clients.DqPublicAdaptiveMonitorSettings{DataLookBack: lookback, LearningPhase: learning}
	}

	// Build the scan query with the dialect-aware builder (clients.BuildDqSourceQuery). The engine
	// scans from sourceQuery, so column selection, row filter, time-slice (${rd}/${rdEnd}) and sampling
	// are all composed INTO it, per the source dialect.
	tsCol := strings.TrimSpace(input.TimeSliceColumn)
	sourceQuery := clients.BuildDqSourceQuery(clients.DqSourceQueryInput{
		DatabaseProduct: conn.DatabaseProductName,
		SchemaName:      input.SchemaName,
		TableName:       input.TableName,
		SelectedColumns: input.SelectedColumns,
		FilterColumn:    strings.TrimSpace(input.FilterColumn),
		FilterOperator:  strings.TrimSpace(input.FilterOperator),
		FilterValue:     input.FilterValue,
		TimeSliceColumn: tsCol,
		RunDateFormat:   "DATE",
		SampleSize:      input.SampleSize,
	})

	// Run-date window: when slicing, seed runDate/runDateEnd to the most recent slice and set the
	// matching dateFormat so the immediate queued run has concrete ${rd}/${rdEnd}. Otherwise stamp
	// runDate = today (the server defaults it when omitted, but seeding it matches prior behavior).
	var runDate, runDateEnd *clients.DqPublicRunDate
	if tsCol != "" {
		tsUnit := strings.ToUpper(strings.TrimSpace(input.TimeSliceUnit))
		if tsUnit == "" {
			tsUnit = "DAYS"
		}
		tsSize := input.TimeSliceSize
		if tsSize <= 0 {
			tsSize = 1
		}
		start, end, kind, dateFormat := sliceWindowPublic(tsSize, tsUnit)
		runDate = &clients.DqPublicRunDate{Kind: kind, Value: start}
		runDateEnd = &clients.DqPublicRunDate{Kind: kind, Value: end}
		if jobSettings != nil {
			jobSettings.DateFormat = dateFormat
		}
	} else {
		runDate = &clients.DqPublicRunDate{Kind: "DATE", Value: time.Now().UTC().Format("2006-01-02")}
	}

	req := &clients.CreateDqJobRequest{
		JobType: jobType,
		JobName: jobName,
		DataLocation: clients.DqDataLocation{
			EdgeSiteName:        input.EdgeSiteName,
			EdgeConnectionName:  conn.ConnectionName,
			DataSourceName:      input.DataSourceName,
			SchemaName:          input.SchemaName,
			TableName:           input.TableName,
			DatabaseProductName: conn.DatabaseProductName,
		},
		SourceQuery:        sourceQuery,
		RunDate:            runDate,
		RunDateEnd:         runDateEnd,
		JobSettings:        jobSettings,
		MonitoringSettings: &clients.DqPublicMonitoringSettings{AdaptiveMonitors: adaptive},
		Notifications:      notifications,
		SchedulingSettings: schedule,
	}

	if input.BackrunEnabled {
		bin := strings.ToUpper(strings.TrimSpace(input.BackrunTimeBin))
		if bin == "" {
			bin = "DAY"
		}
		binValue := input.BackrunBinValue
		if binValue < 1 {
			binValue = 1 // the public Backrun requires binValue >= 1
		}
		req.Backrun = &clients.DqPublicBackrun{TimeBin: bin, BinValue: binValue}
	}

	return req
}

// sliceWindowPublic returns the initial [start, end) run window for the immediate queued run plus the
// matching runDate kind and jobSettings dateFormat: the most recent slice of width size*unit ending
// now. HOURS uses an RFC3339 TIMESTAMP; the rest use a DATE (yyyy-MM-dd).
func sliceWindowPublic(size int, unit string) (start, end, kind, dateFormat string) {
	now := time.Now().UTC()
	switch strings.ToUpper(unit) {
	case "HOURS":
		const f = "2006-01-02T15:04:05Z"
		return now.Add(time.Duration(-size) * time.Hour).Format(f), now.Format(f), "TIMESTAMP", "TIMESTAMP"
	case "WEEKS":
		return now.AddDate(0, 0, -7*size).Format("2006-01-02"), now.Format("2006-01-02"), "DATE", "DATE"
	case "MONTHS":
		return now.AddDate(0, -size, 0).Format("2006-01-02"), now.Format("2006-01-02"), "DATE", "DATE"
	default: // DAYS
		return now.AddDate(0, 0, -size).Format("2006-01-02"), now.Format("2006-01-02"), "DATE", "DATE"
	}
}

// countRecipients totals the recipients across a notifications object's channels.
func countRecipients(n *clients.DqJobNotifications) int {
	total := 0
	for _, c := range n.Channels {
		total += len(c.Recipients)
	}
	return total
}

// describeSchedule renders a one-line summary of the resolved schedule for the preview.
func describeSchedule(s *clients.DqSchedulingSettings) string {
	if s == nil {
		return "run once now"
	}
	switch s.SchedulerMode {
	case "HOURLY":
		return fmt.Sprintf("HOURLY (offset %s)", s.Hourly.HourlyOffset)
	case "DAILY":
		days := "every day"
		if s.Daily != nil && len(s.Daily.DaysOfWeek) > 0 && len(s.Daily.DaysOfWeek) < 7 {
			days = strings.Join(s.Daily.DaysOfWeek, ",")
		}
		return fmt.Sprintf("DAILY [%s] @ %s UTC (offset %s)", days, s.ScheduledRunTime, s.Daily.DailyOffset)
	case "MONTHLY":
		when := s.Monthly.MonthlyRepeat
		if s.Monthly.MonthlyRepeat == "DAY" {
			when = fmt.Sprintf("day %d", s.Monthly.DayNumber)
		}
		return fmt.Sprintf("MONTHLY (%s) @ %s UTC (offset %s)", when, s.ScheduledRunTime, s.Monthly.MonthlyOffset)
	}
	return s.SchedulerMode
}

// buildJobSettings populates the type-specific public jobSettings. PUSHDOWN carries compute settings
// (connections/threads). PULLUP carries loadOptions (+ optional Parallel JDBC) and, for manual sizing,
// sparkJobSizing — omitting sparkJobSizing entirely tells the public API to size automatically (it has
// no autoSizing flag). Returns an error on out-of-range compute, invalid Parallel JDBC, or non-integer
// memory GB.
func buildJobSettings(input Input, jobType string) (*clients.DqPublicJobSettings, error) {
	if strings.EqualFold(jobType, "PUSHDOWN") {
		connections := orDefaultInt(input.PushdownConnections, 10)
		threads := orDefaultInt(input.PushdownThreads, 2)
		if connections < 1 || connections > 50 {
			return nil, fmt.Errorf("pushdownConnections must be between 1 and 50 (got %d)", input.PushdownConnections)
		}
		if threads < 1 || threads > 10 {
			return nil, fmt.Errorf("pushdownThreads must be between 1 and 10 (got %d)", input.PushdownThreads)
		}
		return &clients.DqPublicJobSettings{PushdownSettings: &clients.DqPublicPushdownSettings{Connections: connections, Threads: threads}}, nil
	}
	// PULLUP.
	loadOptions := &clients.DqPublicLoadOptions{NumPartitions: input.SizingNumPartitions} // 0 = let Spark decide
	pj, err := buildParallelJdbc(input)
	if err != nil {
		return nil, err
	}
	loadOptions.ParallelJdbcOptions = pj
	sizing, err := buildSparkJobSizing(input)
	if err != nil {
		return nil, err
	}
	return &clients.DqPublicJobSettings{PullupSettings: &clients.DqPublicPullupSettings{
		LoadOptions:        loadOptions,
		SparkJobSizing:     sizing, // nil => automatic sizing
		SparkSqlProperties: input.SparkSqlProperties,
	}}, nil
}

// buildSparkJobSizing returns nil for automatic sizing (the public API auto-sizes when sparkJobSizing
// is omitted) unless the caller set any manual sizing field, in which case manual sizing is used with
// wizard defaults (1) for the unset fields. Memory GB strings are parsed to integers (SparkMemoryGB).
func buildSparkJobSizing(input Input) (*clients.DqPublicSparkJobSizing, error) {
	if !hasManualSizing(input) {
		return nil, nil
	}
	execMem, err := gbToInt(input.SizingExecutorMemoryGb, 1)
	if err != nil {
		return nil, fmt.Errorf("sizingExecutorMemoryGb %w", err)
	}
	driverMem, err := gbToInt(input.SizingDriverMemoryGb, 1)
	if err != nil {
		return nil, fmt.Errorf("sizingDriverMemoryGb %w", err)
	}
	overhead, err := gbToInt(input.SizingMemoryOverheadGb, 1)
	if err != nil {
		return nil, fmt.Errorf("sizingMemoryOverheadGb %w", err)
	}
	return &clients.DqPublicSparkJobSizing{
		NumExecutors:     orDefaultInt(input.SizingMaxExecutors, 1),
		NumExecutorCores: orDefaultInt(input.SizingExecutorCores, 1),
		ExecutorMemoryGb: execMem,
		DriverCores:      orDefaultInt(input.SizingDriverCores, 1),
		DriverMemoryGb:   driverMem,
		MemoryOverheadGb: overhead,
	}, nil
}

// gbToInt parses a GB string to a positive integer (the public SparkMemoryGB is an integer; fractional
// values are not supported). An empty string uses def.
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

func hasManualSizing(input Input) bool {
	return input.SizingMaxExecutors > 0 || input.SizingExecutorCores > 0 || strings.TrimSpace(input.SizingExecutorMemoryGb) != "" ||
		input.SizingDriverCores > 0 || strings.TrimSpace(input.SizingDriverMemoryGb) != "" || strings.TrimSpace(input.SizingMemoryOverheadGb) != ""
}

// buildParallelJdbc maps the caller's Parallel JDBC inputs onto the ParallelJdbcOptions contract,
// inferring the mode when omitted and enforcing the wizard's rules: AUTO needs nothing; AUTO_COLUMN
// requires a partition count (and no column); MANUAL requires both a column and a count (a specific
// partition column disables auto-calculate). Returns (nil, nil) when Parallel JDBC isn't configured.
func buildParallelJdbc(input Input) (*clients.DqParallelJdbcOptions, error) {
	mode := strings.ToUpper(strings.TrimSpace(input.ParallelJdbcMode))
	col := strings.TrimSpace(input.ParallelJdbcPartitionColumn)
	num := input.ParallelJdbcPartitionNumber
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

// validateTypeSpecificConfig rejects sizing/Parallel JDBC inputs on a PUSHDOWN job and compute inputs
// on a PULLUP job, so the caller can't silently set options that don't apply to the resolved type.
func validateTypeSpecificConfig(input Input, jobType string) error {
	if strings.EqualFold(jobType, "PUSHDOWN") {
		if hasManualSizing(input) || strings.TrimSpace(input.ParallelJdbcMode) != "" ||
			strings.TrimSpace(input.ParallelJdbcPartitionColumn) != "" || input.ParallelJdbcPartitionNumber > 0 ||
			input.SizingNumPartitions > 0 || len(input.SparkSqlProperties) > 0 {
			return fmt.Errorf("sizing / Parallel JDBC settings apply to PULLUP jobs only, but this is a PUSHDOWN job")
		}
		return nil
	}
	if input.PushdownConnections > 0 || input.PushdownThreads > 0 {
		return fmt.Errorf("compute settings (connections/threads) apply to PUSHDOWN jobs only, but this is a PULLUP job")
	}
	return nil
}

// Conversational step labels — used for affectedStep so the caller can route the user back to the
// failing step and re-call with the other inputs preserved.
const (
	stepSelectData    = "Select the data (Step 1)"
	stepMonitors      = "Add monitors (Step 2)"
	stepSizing        = "Size job resources (Pullup)"
	stepSchedule      = "Set a run schedule"
	stepNotifications = "Add notifications"
	stepReview        = "Review and run"
)

// stepForTypeConfigError points type-specific config errors at the right step (compute lives in the
// Pushdown Review step; sizing is the Pullup Sizing step).
func stepForTypeConfigError(jobType string) string {
	if strings.EqualFold(jobType, "PUSHDOWN") {
		return stepReview
	}
	return stepSizing
}

// affectedStepForError maps a submission error message to the most likely configuration step.
func affectedStepForError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "sizing") || strings.Contains(msg, "executor") || strings.Contains(msg, "partition") || strings.Contains(msg, "spark"):
		return stepSizing
	case strings.Contains(msg, "schedul"):
		return stepSchedule
	case strings.Contains(msg, "notif") || strings.Contains(msg, "recipient") || strings.Contains(msg, "alert"):
		return stepNotifications
	case strings.Contains(msg, "monitor") || strings.Contains(msg, "profile"):
		return stepMonitors
	case strings.Contains(msg, "column") || strings.Contains(msg, "filter") || strings.Contains(msg, "sample") || strings.Contains(msg, "query") || strings.Contains(msg, "name"):
		return stepSelectData
	default:
		return stepReview
	}
}

// describeCompute renders a one-line summary of the type-specific settings for the preview. For PULLUP,
// a nil sparkJobSizing means automatic sizing (the public API auto-sizes when it is omitted).
func describeCompute(js *clients.DqPublicJobSettings, jobType string) string {
	if js == nil {
		return "compute=defaults"
	}
	if strings.EqualFold(jobType, "PUSHDOWN") {
		if js.PushdownSettings != nil {
			return fmt.Sprintf("compute=connections %d/threads %d", js.PushdownSettings.Connections, js.PushdownSettings.Threads)
		}
		return "compute=defaults"
	}
	if js.PullupSettings == nil {
		return "sizing=auto"
	}
	sizing := "auto"
	if s := js.PullupSettings.SparkJobSizing; s != nil {
		sizing = fmt.Sprintf("manual(executors %d, cores %d, execMem %dGB, driverMem %dGB)", s.NumExecutors, s.NumExecutorCores, s.ExecutorMemoryGb, s.DriverMemoryGb)
	}
	pj := "off"
	if lo := js.PullupSettings.LoadOptions; lo != nil && lo.ParallelJdbcOptions != nil {
		pj = lo.ParallelJdbcOptions.Mode
	}
	return fmt.Sprintf("sizing=%s, parallelJdbc=%s", sizing, pj)
}

// monitorsIncludeDescriptiveStats reports whether the caller asked to enable descriptiveStatistics.
func monitorsIncludeDescriptiveStats(keys []string) bool {
	for _, k := range keys {
		if strings.EqualFold(strings.TrimSpace(k), "descriptiveStatistics") {
			return true
		}
	}
	return false
}

// hasFilterFields reports whether the caller supplied any row-filter input at all.
func hasFilterFields(in Input) bool {
	return strings.TrimSpace(in.FilterColumn) != "" ||
		strings.TrimSpace(in.FilterOperator) != "" ||
		strings.TrimSpace(in.FilterValue) != ""
}

func missingLocationFields(in Input) []string {
	var missing []string
	for name, val := range map[string]string{
		"edgeSiteName":       in.EdgeSiteName,
		"edgeConnectionName": in.EdgeConnectionName,
		"dataSourceName":     in.DataSourceName,
		"schemaName":         in.SchemaName,
		"tableName":          in.TableName,
	} {
		if strings.TrimSpace(val) == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

func findConnectionByName(ctx context.Context, client *http.Client, name string) (*clients.DqConnection, error) {
	conns, err := clients.ListDqConnections(ctx, client)
	if err != nil {
		return nil, err
	}
	for i := range conns {
		if strings.EqualFold(conns[i].ConnectionName, name) {
			return &conns[i], nil
		}
	}
	return nil, fmt.Errorf("connection %q not found", name)
}
