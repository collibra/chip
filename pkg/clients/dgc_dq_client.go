package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// This client wraps the DGC Unified Data Quality (UDQ) APIs used by the
// data-quality job-creation wizard. Discovery (connections, data sources,
// schemas, tables, columns) uses the internal BFF surface
// (/rest/dq/internal/v1/*) — the same calls the UI makes. Job creation uses
// the public DQ API (/rest/dq/1.0/jobs).

// DqConnection is a data-quality edge connection as returned by
// /rest/dq/internal/v1/connections and /connections/{id}. capabilityTypes
// drives job-type detection: a connection advertises PUSHDOWN, PULLUP, or both.
type DqConnection struct {
	ConnectionID        string        `json:"connectionId"`
	ConnectionName      string        `json:"connectionName"`
	CapabilityTypes     []string      `json:"capabilityTypes"`
	DatabaseProductName string        `json:"databaseProductName"`
	EdgeSiteID          string        `json:"edgeSiteId"`
	EdgeSiteName        string        `json:"edgeSiteName"`
	SourceType          *DqSourceType `json:"sourceType,omitempty"`
	// SystemAssetID is the DGC System asset this connection ingests from (set via the catalog
	// system-asset config). It is the bridge from a catalog asset back to a DQ connection.
	SystemAssetID string `json:"systemAssetId,omitempty"`
}

type DqSourceType struct {
	Provider string `json:"provider"`
	Type     string `json:"type"`
}

type dqConnectionListResponse struct {
	Results []DqConnection `json:"results"`
}

// DqDataSource is a database/catalog within a connection (e.g. "postgres").
type DqDataSource struct {
	DataSourceName  string `json:"dataSourceName"`
	SupportsSchemas bool   `json:"supportsSchemas"`
	TotalJobs       int    `json:"totalJobs"`
}

// DqSchema is a schema within a data source.
type DqSchema struct {
	Name string `json:"name"`
}

// DqTable is a table within a schema.
type DqTable struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// DqColumn is a column within a table.
type DqColumn struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Disabled bool   `json:"disabled"`
}

type dqDataSourceListResponse struct {
	Results []DqDataSource `json:"results"`
	Total   int            `json:"total"`
}

type dqSchemaListResponse struct {
	Results []DqSchema `json:"results"`
	Total   int        `json:"total"`
}

type dqTableListResponse struct {
	Results []DqTable `json:"results"`
	Total   int       `json:"total"`
}

type dqColumnListResponse struct {
	Results []DqColumn `json:"results"`
}

// DqDataLocation identifies where a job reads data from. All five fields are
// required by the create API; databaseProductName is optional/read-only.
type DqDataLocation struct {
	EdgeSiteName        string `json:"edgeSiteName"`
	EdgeConnectionName  string `json:"edgeConnectionName"`
	DataSourceName      string `json:"dataSourceName"`
	SchemaName          string `json:"schemaName"`
	TableName           string `json:"tableName"`
	DatabaseProductName string `json:"databaseProductName,omitempty"`
}

// CreateDqJobRequest is the body for POST /rest/dq/1.0/jobs — the PUBLIC create
// (JobDefinitionCreateRequest in dq/udq-app-client/oas/dq-v1-public-oas-spec.yaml). It is the full
// job definition: sourceQuery (into which column selection, row filter, sampling and the ${rd}/${rdEnd}
// time-slice predicate are composed — see BuildDqSourceQuery), the runDate window, monitors, schedule,
// back-run, notifications, and pullup/pushdown settings. Unlike the internal BFF endpoint, the public
// server is null-tolerant ("provide only the fields you want to override") and auto-generates +
// auto-increments jobName when it is omitted. queueRun is currently always treated as true server-side
// (create-only is not yet supported), so it is left at its default and not sent.
type CreateDqJobRequest struct {
	JobType            string                      `json:"jobType"`
	JobName            string                      `json:"jobName,omitempty"`
	DataLocation       DqDataLocation              `json:"dataLocation"`
	SourceQuery        string                      `json:"sourceQuery,omitempty"`
	RunDate            *DqPublicRunDate            `json:"runDate,omitempty"`
	RunDateEnd         *DqPublicRunDate            `json:"runDateEnd,omitempty"`
	Backrun            *DqPublicBackrun            `json:"backrun,omitempty"`
	JobSettings        *DqPublicJobSettings        `json:"jobSettings,omitempty"`
	MonitoringSettings *DqPublicMonitoringSettings `json:"monitoringSettings,omitempty"`
	Notifications      *DqJobNotifications         `json:"notifications,omitempty"`
	SchedulingSettings *DqSchedulingSettings       `json:"schedulingSettings,omitempty"`
}

// DqPublicRunDate is the discriminated runDate/runDateEnd value ({kind, value}) from the public spec.
// kind=DATE => value is yyyy-MM-dd; kind=TIMESTAMP => value is RFC3339 (yyyy-MM-ddTHH:mm:ssZ). The
// engine substitutes ${rd}/${rdEnd} in sourceQuery from these per run (formatted per jobSettings.dateFormat).
type DqPublicRunDate struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// DqPublicBackrun is the public Backrun: presence enables it (no `enabled` flag); binValue >= 1.
type DqPublicBackrun struct {
	TimeBin  string `json:"timeBin"` // DAY | MONTH | YEAR
	BinValue int    `json:"binValue"`
}

// DqPublicJobSettings is jobSettings: the run-date dateFormat plus the type-specific tuning.
type DqPublicJobSettings struct {
	DateFormat       string                    `json:"dateFormat,omitempty"` // DATE | TIMESTAMP
	PushdownSettings *DqPublicPushdownSettings `json:"pushdownSettings,omitempty"`
	PullupSettings   *DqPublicPullupSettings   `json:"pullupSettings,omitempty"`
}

// DqPublicPushdownSettings is the Pushdown compute (source-system concurrency).
type DqPublicPushdownSettings struct {
	Connections int `json:"connections,omitempty"`
	Threads     int `json:"threads,omitempty"`
}

// DqPublicPullupSettings is the Pullup tuning. Omitting sparkJobSizing = automatic sizing (the public
// API has no autoSizing flag — absence means auto).
type DqPublicPullupSettings struct {
	LoadOptions        *DqPublicLoadOptions    `json:"loadOptions,omitempty"`
	SparkJobSizing     *DqPublicSparkJobSizing `json:"sparkJobSizing,omitempty"`
	SparkSqlProperties map[string]string       `json:"sparkSqlProperties,omitempty"`
}

// DqPublicLoadOptions mirrors the public LoadOptions (numPartitions 0 = let Spark decide).
type DqPublicLoadOptions struct {
	NumPartitions       int                    `json:"numPartitions"`
	ParallelJdbcOptions *DqParallelJdbcOptions `json:"parallelJdbcOptions,omitempty"`
}

// DqPublicSparkJobSizing is manual Spark sizing; memory fields are INTEGER GB (SparkMemoryGB). Send
// this object only for manual sizing — omit it for automatic sizing.
type DqPublicSparkJobSizing struct {
	NumExecutors     int `json:"numExecutors,omitempty"`
	DriverCores      int `json:"driverCores,omitempty"`
	NumExecutorCores int `json:"numExecutorCores,omitempty"`
	ExecutorMemoryGb int `json:"executorMemoryGb,omitempty"`
	DriverMemoryGb   int `json:"driverMemoryGb,omitempty"`
	MemoryOverheadGb int `json:"memoryOverheadGb,omitempty"`
}

// DqPublicMonitoringSettings is monitoringSettings (currently only adaptive monitors).
type DqPublicMonitoringSettings struct {
	AdaptiveMonitors *DqPublicAdaptiveMonitors `json:"adaptiveMonitors,omitempty"`
}

// DqPublicAdaptiveMonitors mirrors the public AdaptiveMonitors — the same toggle set as the internal
// profileMonitors. settings holds the adaptive lookback/learning tuning.
type DqPublicAdaptiveMonitors struct {
	DescriptiveStatistics bool                             `json:"descriptiveStatistics"`
	EmptyFields           bool                             `json:"emptyFields"`
	ExecutionTime         bool                             `json:"executionTime"`
	Max                   bool                             `json:"max"`
	Mean                  bool                             `json:"mean"`
	Min                   bool                             `json:"min"`
	NullValues            bool                             `json:"nullValues"`
	RowCount              bool                             `json:"rowCount"`
	Uniqueness            bool                             `json:"uniqueness"`
	Settings              *DqPublicAdaptiveMonitorSettings `json:"settings,omitempty"`
}

// DqPublicAdaptiveMonitorSettings is the adaptive tuning. NOTE: the public field is dataLookBack
// (capital B), unlike the internal dataLookback.
type DqPublicAdaptiveMonitorSettings struct {
	DataLookBack  int `json:"dataLookBack"`
	LearningPhase int `json:"learningPhase"`
}

// PublicAdaptiveMonitorsFromProfile maps the (shared) DqProfileMonitors toggle set onto the public
// AdaptiveMonitors shape, so callers keep using BuildProfileMonitors for the monitor selection.
func PublicAdaptiveMonitorsFromProfile(pm *DqProfileMonitors) *DqPublicAdaptiveMonitors {
	if pm == nil {
		return nil
	}
	return &DqPublicAdaptiveMonitors{
		DescriptiveStatistics: pm.DescriptiveStatistics,
		EmptyFields:           pm.EmptyFields,
		ExecutionTime:         pm.ExecutionTime,
		Max:                   pm.Max,
		Mean:                  pm.Mean,
		Min:                   pm.Min,
		NullValues:            pm.NullValues,
		RowCount:              pm.RowCount,
		Uniqueness:            pm.Uniqueness,
	}
}

// CreateDqJobResponse is the public create response (JobDefinitionCreateResponse): the created job
// definition plus the queued run id.
type CreateDqJobResponse struct {
	JobName      string         `json:"jobName"`
	JobType      string         `json:"jobType"`
	JobRunID     string         `json:"jobRunId"`
	DataLocation DqDataLocation `json:"dataLocation"`
	SourceQuery  string         `json:"sourceQuery,omitempty"`
}

// ListDqConnections returns all data-quality connections on the instance.
func ListDqConnections(ctx context.Context, collibraHttpClient *http.Client) ([]DqConnection, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "/rest/dq/internal/v1/connections", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}
	var resp dqConnectionListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse connections response: %w", err)
	}
	return resp.Results, nil
}

// GetDqConnection fetches a single connection by id. The response carries the
// job-type (capabilityTypes) and the dataLocation fields edgeSiteName,
// connectionName, and databaseProductName.
func GetDqConnection(ctx context.Context, collibraHttpClient *http.Client, connectionID string) (*DqConnection, error) {
	endpoint := "/rest/dq/internal/v1/connections/" + url.PathEscape(connectionID)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}
	var conn DqConnection
	if err := json.Unmarshal(body, &conn); err != nil {
		return nil, fmt.Errorf("failed to parse connection response: %w", err)
	}
	return &conn, nil
}

// ListDqDataSources lists the databases/catalogs reachable through a connection.
func ListDqDataSources(ctx context.Context, collibraHttpClient *http.Client, connectionID string, limit, offset int) ([]DqDataSource, error) {
	endpoint := fmt.Sprintf("/rest/dq/internal/v1/monitoring/edge/connections/%s/dataSources?%s",
		url.PathEscape(connectionID), dqPageQuery(nil, limit, offset))
	body, err := dqGet(ctx, collibraHttpClient, endpoint)
	if err != nil {
		return nil, err
	}
	var resp dqDataSourceListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse data sources response: %w", err)
	}
	return resp.Results, nil
}

// ListDqSchemas lists schemas in a data source (live edge query).
func ListDqSchemas(ctx context.Context, collibraHttpClient *http.Client, siteID, connectionID, dataSourceName string, limit, offset int) ([]DqSchema, error) {
	endpoint := fmt.Sprintf("/rest/dq/internal/v1/monitoring/edge/%s/connections/%s/schemas?%s",
		url.PathEscape(siteID), url.PathEscape(connectionID),
		dqPageQuery(url.Values{"dataSourceName": {dataSourceName}}, limit, offset))
	body, err := dqGet(ctx, collibraHttpClient, endpoint)
	if err != nil {
		return nil, err
	}
	var resp dqSchemaListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse schemas response: %w", err)
	}
	return resp.Results, nil
}

// ListDqTables lists tables in a schema (live edge query).
func ListDqTables(ctx context.Context, collibraHttpClient *http.Client, siteID, connectionID, dataSourceName, schemaName string, limit, offset int) ([]DqTable, error) {
	endpoint := fmt.Sprintf("/rest/dq/internal/v1/monitoring/edge/%s/connections/%s/tables?%s",
		url.PathEscape(siteID), url.PathEscape(connectionID),
		dqPageQuery(url.Values{"dataSourceName": {dataSourceName}, "schemaName": {schemaName}}, limit, offset))
	body, err := dqGet(ctx, collibraHttpClient, endpoint)
	if err != nil {
		return nil, err
	}
	var resp dqTableListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse tables response: %w", err)
	}
	return resp.Results, nil
}

// ListDqColumns lists columns in a table (live edge query).
func ListDqColumns(ctx context.Context, collibraHttpClient *http.Client, siteID, connectionID, dataSourceName, schemaName, tableName string, limit, offset int) ([]DqColumn, error) {
	endpoint := fmt.Sprintf("/rest/dq/internal/v1/monitoring/edge/%s/connections/%s/columns?%s",
		url.PathEscape(siteID), url.PathEscape(connectionID),
		dqPageQuery(url.Values{"dataSourceName": {dataSourceName}, "schemaName": {schemaName}, "tableName": {tableName}}, limit, offset))
	body, err := dqGet(ctx, collibraHttpClient, endpoint)
	if err != nil {
		return nil, err
	}
	var resp dqColumnListResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse columns response: %w", err)
	}
	return resp.Results, nil
}

// CreateDqJob creates a data-quality job and queues an immediate run.
func CreateDqJob(ctx context.Context, collibraHttpClient *http.Client, request CreateDqJobRequest) (*CreateDqJobResponse, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Creating DQ job '%s' (%s) for %s.%s",
		request.JobName, request.JobType, request.DataLocation.SchemaName, request.DataLocation.TableName))

	jsonData, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal create job request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "/rest/dq/1.0/jobs", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}
	var resp CreateDqJobResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse create job response: %w", err)
	}
	return &resp, nil
}

// DqParallelJdbcOptions is the wizard's "Parallel JDBC" advanced-sizing control (PULLUP). mode is the
// ParallelJdbcMode enum (udq-app): AUTO (column + partition count both auto), AUTO_COLUMN (column auto,
// partitionNumber required), MANUAL (partitionColumn + partitionNumber both required). Validator rules
// (ParallelJdbcOptionsValidator): AUTO needs neither; AUTO_COLUMN needs partitionNumber and no column;
// MANUAL needs both.
type DqParallelJdbcOptions struct {
	Mode            string `json:"mode"`                      // AUTO | AUTO_COLUMN | MANUAL
	PartitionColumn string `json:"partitionColumn,omitempty"` // only for MANUAL
	PartitionNumber int    `json:"partitionNumber,omitempty"` // required for AUTO_COLUMN and MANUAL
}

// Valid ParallelJdbcMode values (udq-app ParallelJdbcMode enum).
const (
	ParallelJdbcAuto       = "AUTO"
	ParallelJdbcAutoColumn = "AUTO_COLUMN"
	ParallelJdbcManual     = "MANUAL"
)

// DqProfileMonitorSettings — the "Advanced monitor settings" (adaptive behavior). dataLookback
// = how many prior runs feed the adaptive baseline (wizard default 10); learningPhase = runs
// before adaptive monitors start alerting (wizard default 4). Null-safe: JobMonitorsMapper reads
// it only when non-null. Contract: ProfileMonitorSettings in ui-v1-private-oas-spec.yaml.
type DqProfileMonitorSettings struct {
	DataLookback  int `json:"dataLookback"`
	LearningPhase int `json:"learningPhase"`
}

// DqProfileMonitors is the monitor-toggle set (the "Monitors" step). Wizard defaults =
// rowCount/uniqueness/nullValues/emptyFields ON, the rest OFF. BuildProfileMonitors builds it from the
// selected keys; PublicAdaptiveMonitorsFromProfile maps it onto the public adaptiveMonitors shape.
type DqProfileMonitors struct {
	DescriptiveStatistics bool                      `json:"descriptiveStatistics"`
	EmptyFields           bool                      `json:"emptyFields"`
	ExecutionTime         bool                      `json:"executionTime"`
	Max                   bool                      `json:"max"`
	Mean                  bool                      `json:"mean"`
	Min                   bool                      `json:"min"`
	NullValues            bool                      `json:"nullValues"`
	RowCount              bool                      `json:"rowCount"`
	Uniqueness            bool                      `json:"uniqueness"`
	Settings              *DqProfileMonitorSettings `json:"settings,omitempty"`
}

// DqMonitorInfo describes one profile monitor for display and selection. Key matches the
// DqProfileMonitors JSON field and the create_dq_job `monitors` input; DefaultEnabled marks
// the monitors the wizard turns on by default.
type DqMonitorInfo struct {
	Key            string `json:"key"`
	Label          string `json:"label"`
	Description    string `json:"description"`
	DefaultEnabled bool   `json:"defaultEnabled"`
}

// DqMonitorCatalog is the ordered list of selectable profile monitors with their defaults.
// Defaults match the DQ wizard: row count, null values, empty values, and uniqueness ON; the
// numeric/timing monitors OFF; descriptiveStatistics OFF because enabling it UNMASKS sensitive
// data (the server sets maskSensitive = !descriptiveStatistics in JobMonitorsMapper).
func DqMonitorCatalog() []DqMonitorInfo {
	return []DqMonitorInfo{
		{Key: "rowCount", Label: "Row count", Description: "Track the row count per run.", DefaultEnabled: true},
		{Key: "nullValues", Label: "Null values", Description: "Track the share of NULLs per column.", DefaultEnabled: true},
		{Key: "emptyFields", Label: "Empty values", Description: "Track empty/blank values per column.", DefaultEnabled: true},
		{Key: "uniqueness", Label: "Uniqueness", Description: "Track distinct/duplicate values per column.", DefaultEnabled: true},
		{Key: "min", Label: "Minimum value", Description: "Track the minimum value per numeric column.", DefaultEnabled: false},
		{Key: "mean", Label: "Mean value", Description: "Track the mean value per numeric column.", DefaultEnabled: false},
		{Key: "max", Label: "Maximum value", Description: "Track the maximum value per numeric column.", DefaultEnabled: false},
		{Key: "executionTime", Label: "Execution time", Description: "Track how long each run takes.", DefaultEnabled: false},
		{Key: "descriptiveStatistics", Label: "Descriptive statistics", Description: "Compute descriptive statistics. WARNING: UNMASKS sensitive data (maskSensitive=false) — enable only with explicit user confirmation.", DefaultEnabled: false},
	}
}

// MonitorKeys returns every valid monitor key, in catalog order.
func MonitorKeys() []string {
	cat := DqMonitorCatalog()
	keys := make([]string, 0, len(cat))
	for _, m := range cat {
		keys = append(keys, m.Key)
	}
	return keys
}

// DefaultMonitorKeys returns the keys of the monitors that are enabled by default.
func DefaultMonitorKeys() []string {
	var keys []string
	for _, m := range DqMonitorCatalog() {
		if m.DefaultEnabled {
			keys = append(keys, m.Key)
		}
	}
	return keys
}

// BuildProfileMonitors turns a set of enabled monitor keys (case-insensitive) into a
// DqProfileMonitors. Keys not in the catalog are returned in `unknown` so the caller can
// reject them; the returned monitors reflect only the recognized keys (all others OFF).
func BuildProfileMonitors(enabledKeys []string) (*DqProfileMonitors, []string) {
	valid := map[string]bool{}
	for _, m := range DqMonitorCatalog() {
		valid[strings.ToLower(m.Key)] = true
	}
	on := map[string]bool{}
	var unknown []string
	for _, k := range enabledKeys {
		lk := strings.ToLower(strings.TrimSpace(k))
		if lk == "" {
			continue
		}
		if !valid[lk] {
			unknown = append(unknown, k)
			continue
		}
		on[lk] = true
	}
	return &DqProfileMonitors{
		DescriptiveStatistics: on["descriptivestatistics"],
		EmptyFields:           on["emptyfields"],
		ExecutionTime:         on["executiontime"],
		Max:                   on["max"],
		Mean:                  on["mean"],
		Min:                   on["min"],
		NullValues:            on["nullvalues"],
		RowCount:              on["rowcount"],
		Uniqueness:            on["uniqueness"],
	}, unknown
}

// EnabledMonitorKeys returns the catalog keys enabled in pm, in catalog order (for display).
func EnabledMonitorKeys(pm *DqProfileMonitors) []string {
	if pm == nil {
		return nil
	}
	state := map[string]bool{
		"rowcount": pm.RowCount, "nullvalues": pm.NullValues, "emptyfields": pm.EmptyFields,
		"uniqueness": pm.Uniqueness, "min": pm.Min, "mean": pm.Mean, "max": pm.Max,
		"executiontime": pm.ExecutionTime, "descriptivestatistics": pm.DescriptiveStatistics,
	}
	var keys []string
	for _, m := range DqMonitorCatalog() {
		if state[strings.ToLower(m.Key)] {
			keys = append(keys, m.Key)
		}
	}
	return keys
}

// DqAdaptiveMonitorSetting describes one tunable "Advanced monitor setting" (the adaptive
// behavior in the Monitors step). Key matches the create_dq_job input; Default is the wizard
// default applied when the user doesn't override.
type DqAdaptiveMonitorSetting struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Default     int    `json:"default"`
}

// DqAdaptiveMonitorSettings is the catalog of advanced monitor settings to show the user,
// surfaced alongside the monitor toggles so the agent can offer them explicitly.
func DqAdaptiveMonitorSettings() []DqAdaptiveMonitorSetting {
	return []DqAdaptiveMonitorSetting{
		{Key: "dataLookback", Label: "Data lookback", Description: "Number of prior runs used as the adaptive baseline.", Default: 10},
		{Key: "learningPhase", Label: "Learning phase", Description: "Number of runs before adaptive monitors begin alerting.", Default: 4},
	}
}

// DqSchedulingSettings — schedulerMode HOURLY/DAILY/MONTHLY + scheduledRunTime (HH:mm:ss UTC),
// plus the mode-specific sub-object the server's SchedulingSettingsMapper requires:
//   - DAILY (also Weekly/Weekdays): daily.daysOfWeek + daily.dailyOffset
//   - HOURLY:                       hourly.hourlyOffset
//   - MONTHLY:                      monthly.{monthlyRepeat, dayNumber, monthlyOffset}
//
// The mapper dereferences these sub-objects unguarded, so the chosen mode's sub-object MUST be
// populated. The *Offset enums set the run-date offset that drives ${rd}/${rdEnd} per run.
type DqSchedulingSettings struct {
	SchedulerMode    string             `json:"schedulerMode"`
	ScheduledRunTime string             `json:"scheduledRunTime"`
	IsActive         bool               `json:"isActive"`
	Daily            *DqDailySchedule   `json:"daily,omitempty"`
	Hourly           *DqHourlySchedule  `json:"hourly,omitempty"`
	Monthly          *DqMonthlySchedule `json:"monthly,omitempty"`
}

// DqDailySchedule drives DAILY/Weekly/Weekdays. daysOfWeek is required (>=1); dailyOffset is the
// run-date offset (SCHEDULED = the run's own day, ONE_DAY..SEVEN_DAYS = that many days back).
type DqDailySchedule struct {
	DailyOffset string   `json:"dailyOffset"`
	DaysOfWeek  []string `json:"daysOfWeek"`
}

// DqHourlySchedule drives HOURLY. hourlyOffset: SCHEDULED | ONE_HOUR | TWO_HOURS.
type DqHourlySchedule struct {
	HourlyOffset string `json:"hourlyOffset"`
}

// DqMonthlySchedule drives MONTHLY. monthlyRepeat FIRST|LAST|DAY (DAY uses dayNumber 1-28);
// monthlyOffset: SCHEDULED | FIRST_OF_CURRENT_MONTH | FIRST_OF_PRIOR_MONTH | LAST_OF_PRIOR_MONTH.
type DqMonthlySchedule struct {
	DayNumber     int    `json:"dayNumber,omitempty"`
	MonthlyOffset string `json:"monthlyOffset"`
	MonthlyRepeat string `json:"monthlyRepeat"`
}

// Valid offset/day enum values (from ui-v1-private-oas-spec.yaml).
var (
	dqHourlyOffsets  = []string{"SCHEDULED", "ONE_HOUR", "TWO_HOURS"}
	dqDailyOffsets   = []string{"SCHEDULED", "ONE_DAY", "TWO_DAYS", "THREE_DAYS", "FOUR_DAYS", "FIVE_DAYS", "SIX_DAYS", "SEVEN_DAYS"}
	dqMonthlyOffsets = []string{"SCHEDULED", "FIRST_OF_CURRENT_MONTH", "FIRST_OF_PRIOR_MONTH", "LAST_OF_PRIOR_MONTH"}
	dqDaysOfWeek     = []string{"MONDAY", "TUESDAY", "WEDNESDAY", "THURSDAY", "FRIDAY", "SATURDAY", "SUNDAY"}
	dqWeekdays       = []string{"MONDAY", "TUESDAY", "WEDNESDAY", "THURSDAY", "FRIDAY"}
)

// DqScheduleInput is the friendly schedule request that BuildSchedulingSettings validates and maps
// onto the API's mode-specific sub-objects.
type DqScheduleInput struct {
	Repeat        string   // NEVER (default) | HOURLY | DAILY | WEEKLY | WEEKDAYS | MONTHLY
	RunTime       string   // HH:mm[:ss] UTC; defaults to 00:00:00
	DaysOfWeek    []string // for WEEKLY
	DayOfMonth    int      // for MONTHLY DAY mode (1-28)
	MonthlyMode   string   // for MONTHLY: DAY (default) | FIRST | LAST
	RunDateOffset string   // mode-specific offset; defaults to SCHEDULED
}

// BuildSchedulingSettings validates the friendly schedule input and maps it onto the API shape.
// Returns (nil, nil) for NEVER/empty (run once now), the populated settings on success, or a
// descriptive error the caller can surface as needs_input.
func BuildSchedulingSettings(in DqScheduleInput) (*DqSchedulingSettings, error) {
	repeat := strings.ToUpper(strings.TrimSpace(in.Repeat))
	if repeat == "" || repeat == "NEVER" {
		return nil, nil
	}
	offset := strings.ToUpper(strings.TrimSpace(in.RunDateOffset))
	if offset == "" {
		offset = "SCHEDULED"
	}
	settings := &DqSchedulingSettings{ScheduledRunTime: normalizeRunTime(in.RunTime), IsActive: true}

	switch repeat {
	case "HOURLY":
		if !dqContains(dqHourlyOffsets, offset) {
			return nil, fmt.Errorf("runDateOffset %q is invalid for HOURLY (use one of: %s)", offset, strings.Join(dqHourlyOffsets, ", "))
		}
		settings.SchedulerMode = "HOURLY"
		settings.Hourly = &DqHourlySchedule{HourlyOffset: offset}

	case "DAILY", "WEEKLY", "WEEKDAYS":
		if !dqContains(dqDailyOffsets, offset) {
			return nil, fmt.Errorf("runDateOffset %q is invalid for %s (use one of: %s)", offset, repeat, strings.Join(dqDailyOffsets, ", "))
		}
		days, err := resolveDaysOfWeek(repeat, in.DaysOfWeek)
		if err != nil {
			return nil, err
		}
		settings.SchedulerMode = "DAILY"
		settings.Daily = &DqDailySchedule{DailyOffset: offset, DaysOfWeek: days}

	case "MONTHLY":
		if !dqContains(dqMonthlyOffsets, offset) {
			return nil, fmt.Errorf("runDateOffset %q is invalid for MONTHLY (use one of: %s)", offset, strings.Join(dqMonthlyOffsets, ", "))
		}
		mode := strings.ToUpper(strings.TrimSpace(in.MonthlyMode))
		if mode == "" {
			mode = "DAY"
		}
		monthly := &DqMonthlySchedule{MonthlyOffset: offset, MonthlyRepeat: mode}
		switch mode {
		case "DAY":
			if in.DayOfMonth < 1 || in.DayOfMonth > 28 {
				return nil, fmt.Errorf("scheduleDayOfMonth must be 1-28 for MONTHLY DAY mode (got %d)", in.DayOfMonth)
			}
			monthly.DayNumber = in.DayOfMonth
		case "FIRST", "LAST":
			// run on the first/last day of the month; no day number needed
		default:
			return nil, fmt.Errorf("scheduleMonthlyMode %q is invalid (use DAY, FIRST, or LAST)", mode)
		}
		settings.SchedulerMode = "MONTHLY"
		settings.Monthly = monthly

	default:
		return nil, fmt.Errorf("scheduleRepeat %q is invalid (use NEVER, HOURLY, DAILY, WEEKLY, WEEKDAYS, or MONTHLY)", repeat)
	}
	return settings, nil
}

// resolveDaysOfWeek returns the active days for a daily-family mode: all 7 for DAILY, Mon-Fri for
// WEEKDAYS, or the validated user set for WEEKLY (which requires at least one valid day).
func resolveDaysOfWeek(repeat string, userDays []string) ([]string, error) {
	switch repeat {
	case "DAILY":
		return dqDaysOfWeek, nil
	case "WEEKDAYS":
		return dqWeekdays, nil
	default: // WEEKLY
		var days []string
		for _, d := range userDays {
			up := strings.ToUpper(strings.TrimSpace(d))
			if up == "" {
				continue
			}
			if !dqContains(dqDaysOfWeek, up) {
				return nil, fmt.Errorf("scheduleDaysOfWeek contains invalid day %q (use: %s)", d, strings.Join(dqDaysOfWeek, ", "))
			}
			days = append(days, up)
		}
		if len(days) == 0 {
			return nil, fmt.Errorf("WEEKLY requires at least one day in scheduleDaysOfWeek (e.g. MONDAY)")
		}
		return days, nil
	}
}

// normalizeRunTime pads HH:mm to HH:mm:ss and defaults empty to 00:00:00.
func normalizeRunTime(t string) string {
	t = strings.TrimSpace(t)
	if t == "" {
		return "00:00:00"
	}
	if strings.Count(t, ":") == 1 {
		return t + ":00"
	}
	return t
}

func dqContains(set []string, v string) bool {
	for _, s := range set {
		if s == v {
			return true
		}
	}
	return false
}

// GetDqDataDistribution wraps the wizard's "Show distribution" / days-with-data helper.
// WHY INTERNAL: there is no public equivalent — this BFF endpoint runs a live edge query to
// return the row distribution over a date column, so the agent can show the user which days
// actually have data before choosing a time-slice / backrun range.
// GET /rest/dq/internal/v1/explorer/{connId}/data/distribution
func GetDqDataDistribution(ctx context.Context, collibraHttpClient *http.Client, connectionID, dataSourceName, schemaName, tableName, columnName, groupBy string, isDate bool) (json.RawMessage, error) {
	v := url.Values{
		"databaseName": {dataSourceName},
		"schemaName":   {schemaName},
		"tableName":    {tableName},
		"columnName":   {columnName},
		"groupBy":      {groupBy}, // e.g. DAY
		"isDate":       {strconv.FormatBool(isDate)},
	}
	endpoint := fmt.Sprintf("/rest/dq/internal/v1/explorer/%s/data/distribution?%s", url.PathEscape(connectionID), v.Encode())
	body, err := dqGet(ctx, collibraHttpClient, endpoint)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(body), nil
}

// =====================================================================================
// Job-name helpers (the wizard's Step 1 name handling) — all on the internal jobs surface.
// =====================================================================================

type dqJobNameRequest struct {
	SchemaName string `json:"schemaName"`
	TableName  string `json:"tableName"`
}

type dqJobNameResponse struct {
	JobName string `json:"jobName"`
}

// GenerateUniqueJobName asks the server for a collision-free default job name for schema.table.
// The server auto-increments (e.g. "sales.orders_2") when the base name is taken, so the user is
// never asked to resolve a name conflict manually — POST /rest/dq/internal/v1/jobs/name.
func GenerateUniqueJobName(ctx context.Context, collibraHttpClient *http.Client, schemaName, tableName string) (string, error) {
	payload, err := json.Marshal(dqJobNameRequest{SchemaName: schemaName, TableName: tableName})
	if err != nil {
		return "", fmt.Errorf("failed to marshal job-name request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "/rest/dq/internal/v1/jobs/name", bytes.NewBuffer(payload))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return "", err
	}
	var resp dqJobNameResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", fmt.Errorf("failed to parse job-name response: %w", err)
	}
	return resp.JobName, nil
}

// IsValidDqJobName asks the server whether a job name is syntactically valid — the wizard rejects
// special characters other than - and _. GET /rest/dq/internal/v1/jobs/{name}/validJobName.
func IsValidDqJobName(ctx context.Context, collibraHttpClient *http.Client, jobName string) (bool, error) {
	return dqGetBool(ctx, collibraHttpClient, "/rest/dq/internal/v1/jobs/"+url.PathEscape(jobName)+"/validJobName")
}

// DqJobExists reports whether a job with the given name already exists —
// GET /rest/dq/internal/v1/jobs/{name}/exists.
func DqJobExists(ctx context.Context, collibraHttpClient *http.Client, jobName string) (bool, error) {
	return dqGetBool(ctx, collibraHttpClient, "/rest/dq/internal/v1/jobs/"+url.PathEscape(jobName)+"/exists")
}

func dqGetBool(ctx context.Context, collibraHttpClient *http.Client, endpoint string) (bool, error) {
	body, err := dqGet(ctx, collibraHttpClient, endpoint)
	if err != nil {
		return false, err
	}
	var b bool
	if err := json.Unmarshal(body, &b); err != nil {
		return false, fmt.Errorf("failed to parse boolean response from %s: %w", endpoint, err)
	}
	return b, nil
}

// =====================================================================================
// DQ permission preflight (DGC-core global permissions).
//
// The DQ wizard gates the Create/Schedule/Run actions on the invoking user's GLOBAL permissions
// (the same identifiers the frontend checks via currentUser.global). We read them from the public
// core endpoint GET /rest/2.0/users/current/globalPermissions, which returns the enum names below.
// Source of identifiers: iam-authorization Permission enum (DATA_QUALITY_JOB_*).
// =====================================================================================

const (
	PermDqJobCreate       = "DATA_QUALITY_JOB_CREATE"
	PermDqJobRun          = "DATA_QUALITY_JOB_RUN"
	PermDqJobSchedule     = "DATA_QUALITY_JOB_SCHEDULE"
	PermDqJobEdit         = "DATA_QUALITY_JOB_EDIT"
	PermResourceManageAll = "RESOURCE_MANAGE_ALL"
)

// dqConnectionResourcePrefix is the resource-id prefix the DQ UI uses for connection-scoped
// permissions (frontend DATA_QUALITY_CONNECTION_RESOURCE_PREFIX).
const dqConnectionResourcePrefix = "DataQualityConnection"

// HasPermission reports whether perm is present in perms (case-insensitive).
func HasPermission(perms []string, perm string) bool {
	for _, p := range perms {
		if strings.EqualFold(strings.TrimSpace(p), perm) {
			return true
		}
	}
	return false
}

// GetCurrentUserGlobalPermissions returns the invoking user's GLOBAL permission identifiers
// (e.g. DATA_QUALITY_JOB_CREATE) — GET /rest/2.0/users/current/globalPermissions. NOTE: DQ
// create/run/schedule are usually granted as CONNECTION-resource permissions, not global ones —
// use GetDqConnectionPermissions for the DQ preflight; this is kept for global-only checks.
func GetCurrentUserGlobalPermissions(ctx context.Context, collibraHttpClient *http.Client) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "/rest/2.0/users/current/globalPermissions", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		GlobalPermissions []string `json:"globalPermissions"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse permissions response: %w", err)
	}
	return resp.GlobalPermissions, nil
}

// dqPermissionsQuery mirrors the DQ UI's permission lookup (frontend permissions query): it returns
// the current user's global permissions plus the permissions on a specific resource. The DQ wizard
// gates create/run/schedule on the CONNECTION resource (`global || resource`).
const dqPermissionsQuery = `query permissions($shouldIncludeResource: Boolean!, $resourceId: String) {
  api {
    currentUser {
      global: permissions
      resource: permissions(resourceId: $resourceId) @include(if: $shouldIncludeResource)
    }
  }
}`

type dqPermissionsResponse struct {
	Data *struct {
		Api *struct {
			CurrentUser *struct {
				Global   []string `json:"global"`
				Resource []string `json:"resource"`
			} `json:"currentUser"`
		} `json:"api"`
	} `json:"data"`
	Errors []Error `json:"errors"`
}

// GetDqConnectionPermissions returns the invoking user's (global, connectionResource) permission
// identifiers for the given DQ connection — POST /graphql, the same query the DQ UI uses. Check a
// permission with `HasPermission(global, p) || HasPermission(resource, p)`.
func GetDqConnectionPermissions(ctx context.Context, collibraHttpClient *http.Client, connectionID string) (global, resource []string, err error) {
	reqBody := Request{
		Query: dqPermissionsQuery,
		Variables: map[string]interface{}{
			"shouldIncludeResource": true,
			"resourceId":            dqConnectionResourcePrefix + ":" + connectionID,
		},
	}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal permissions query: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "POST", "/graphql", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return nil, nil, err
	}
	var resp dqPermissionsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, nil, fmt.Errorf("failed to parse permissions response: %w", err)
	}
	if len(resp.Errors) > 0 {
		return nil, nil, fmt.Errorf("permissions query errors: %v", resp.Errors)
	}
	if resp.Data == nil || resp.Data.Api == nil || resp.Data.Api.CurrentUser == nil {
		return nil, nil, fmt.Errorf("permissions response missing currentUser")
	}
	return resp.Data.Api.CurrentUser.Global, resp.Data.Api.CurrentUser.Resource, nil
}

// DqJobDetailsPath returns the Job Details deep-link path for a created job. The DQ SPA route
// /data-quality/jobs takes the jobName and resolves the latest run itself. It is relative to the
// Collibra instance base URL (the chip client reaches DGC over an internal URL, so the public host
// is prepended by the calling surface).
func DqJobDetailsPath(jobName string) string {
	return "/data-quality/jobs?jobName=" + url.QueryEscape(jobName)
}

// CatalogAssetPath returns the catalog deep-link path for an asset (relative to the instance URL).
func CatalogAssetPath(assetID string) string {
	return "/asset/" + url.PathEscape(assetID)
}

// DqTableAssetLocation is a catalog Table asset resolved to its DQ data location.
type DqTableAssetLocation struct {
	TableAssetID   string
	SystemAssetID  string
	ConnectionID   string
	ConnectionName string
	EdgeSiteName   string
	DataSourceName string
	SchemaName     string
	TableName      string
}

// ResolveDqLocationFromTableAsset maps a DGC catalog Table asset to the DQ connection/dataSource/
// schema/table that backs it. It walks the catalog hierarchy up from the table
// (Table -> Schema -> Database -> System) via incoming relations, then matches the DQ connection
// whose systemAssetId equals the table's System ancestor — the same mapping DQ uses for its own
// catalog asset links (so if asset links work from DQ, this resolves).
func ResolveDqLocationFromTableAsset(ctx context.Context, collibraHttpClient *http.Client, tableAssetID string) (*DqTableAssetLocation, error) {
	tableAssetID = strings.TrimSpace(tableAssetID)
	table, err := GetAssetWithRelations(ctx, collibraHttpClient, tableAssetID)
	if err != nil {
		return nil, err
	}
	if table.Type == nil || !strings.EqualFold(table.Type.Name, "Table") {
		return nil, fmt.Errorf("asset %s is not a Table asset (type=%s)", tableAssetID, assetTypeName(table.Type))
	}
	schemaRef := parentAssetOfType(table, "Schema")
	if schemaRef == nil {
		return nil, fmt.Errorf("table asset %s has no parent Schema in the catalog hierarchy", tableAssetID)
	}
	schema, err := GetAssetWithRelations(ctx, collibraHttpClient, schemaRef.ID)
	if err != nil {
		return nil, err
	}
	dbRef := parentAssetOfType(schema, "Database")
	if dbRef == nil {
		return nil, fmt.Errorf("schema asset %s has no parent Database in the catalog hierarchy", schemaRef.ID)
	}
	database, err := GetAssetWithRelations(ctx, collibraHttpClient, dbRef.ID)
	if err != nil {
		return nil, err
	}
	sysRef := parentAssetOfType(database, "System")
	if sysRef == nil {
		return nil, fmt.Errorf("database asset %s has no parent System in the catalog hierarchy", dbRef.ID)
	}

	conns, err := ListDqConnections(ctx, collibraHttpClient)
	if err != nil {
		return nil, err
	}
	for i := range conns {
		sysID := conns[i].SystemAssetID
		if sysID == "" {
			// The list response may omit systemAssetId; fetch the connection detail to fill it.
			if full, e := GetDqConnection(ctx, collibraHttpClient, conns[i].ConnectionID); e == nil && full != nil {
				sysID = full.SystemAssetID
			}
		}
		if sysID != "" && strings.EqualFold(sysID, sysRef.ID) {
			return &DqTableAssetLocation{
				TableAssetID:   tableAssetID,
				SystemAssetID:  sysRef.ID,
				ConnectionID:   conns[i].ConnectionID,
				ConnectionName: conns[i].ConnectionName,
				EdgeSiteName:   conns[i].EdgeSiteName,
				DataSourceName: dbRef.DisplayName,
				SchemaName:     schemaRef.DisplayName,
				TableName:      table.DisplayName,
			}, nil
		}
	}
	return nil, fmt.Errorf("no DQ connection is mapped to the table's System asset %s — map the catalog system on the connection first", sysRef.ID)
}

// parentAssetOfType returns the first incoming-relation source whose asset type matches typeName.
func parentAssetOfType(a *Asset, typeName string) *RelatedAsset {
	if a == nil {
		return nil
	}
	for i := range a.IncomingRelations {
		s := a.IncomingRelations[i].Source
		if s != nil && s.Type != nil && strings.EqualFold(s.Type.Name, typeName) {
			return s
		}
	}
	return nil
}

func assetTypeName(t *AssetType) string {
	if t == nil {
		return "unknown"
	}
	return t.Name
}

// TableAssetMatch is a catalog Table asset candidate from a by-name lookup.
type TableAssetMatch struct {
	ID          string
	DisplayName string
	FullName    string
	DomainName  string
}

// FindTableAssetsByName looks up catalog Table assets by exact signifier (displayName) via the
// public assets API (GET /rest/2.0/assets) — no search index required. Returns all matches so the
// caller can disambiguate.
func FindTableAssetsByName(ctx context.Context, collibraHttpClient *http.Client, name string, limit int) ([]TableAssetMatch, error) {
	tableType, err := GetAssetTypeByPublicID(ctx, collibraHttpClient, "Table")
	if err != nil {
		return nil, fmt.Errorf("resolve Table asset type: %w", err)
	}
	if limit <= 0 {
		limit = 50
	}
	params := url.Values{}
	params.Set("name", name)
	params.Set("nameMatchMode", "EXACT")
	params.Set("typeId", tableType.ID)
	params.Set("limit", strconv.Itoa(limit))
	req, err := http.NewRequestWithContext(ctx, "GET", "/rest/2.0/assets?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	body, err := executeRequest(collibraHttpClient, req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Results []struct {
			ID          string `json:"id"`
			DisplayName string `json:"displayName"`
			Name        string `json:"name"`
			Domain      struct {
				Name string `json:"name"`
			} `json:"domain"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse assets response: %w", err)
	}
	out := make([]TableAssetMatch, 0, len(resp.Results))
	for _, r := range resp.Results {
		out = append(out, TableAssetMatch{ID: r.ID, DisplayName: r.DisplayName, FullName: r.Name, DomainName: r.Domain.Name})
	}
	return out, nil
}

func dqGet(ctx context.Context, collibraHttpClient *http.Client, endpoint string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	return executeRequest(collibraHttpClient, req)
}

func dqPageQuery(values url.Values, limit, offset int) string {
	if values == nil {
		values = url.Values{}
	}
	values.Set("limit", strconv.Itoa(limit))
	values.Set("offset", strconv.Itoa(offset))
	return values.Encode()
}

// UnsupportedWizardOptions lists the data-quality wizard configuration steps the
// create-DQ-job tools do NOT expose yet, each paired with the default the server
// applies. Shared by prepare_create_dq_job (to disclose proactively at the ready
// step) and create_dq_job (to disclose again at preview), so the user is never
// misled about scope. jobType selects the type-specific entry (Sizing for Pullup,
// Compute for Pushdown).
func UnsupportedWizardOptions(jobType string) []string {
	// NOTE: schedule, time-slice, back-runs, column selection, monitor selection, adaptive monitor
	// settings, row filtering, row sampling, notifications (incl. per-type messages), manual Spark
	// sizing + Parallel JDBC (Pullup), and compute settings (Pushdown) ARE all supported now. The
	// only no-code wizard option still not wired is the optional date-cast expression for a
	// non-standard date column (used by the time-slice and row-filter date inputs).
	_ = jobType // retained for API symmetry; the remaining gap is the same for both job types
	return []string{
		"Date cast expression for a non-standard date column (time-slice / row-filter) — use a real date/timestamp column instead",
	}
}
