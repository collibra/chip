// Package prepare_create_dq_job implements the prepare_create_dq_job MCP tool —
// a read-only companion to create_dq_job. Given whatever the agent knows so far
// (a connection, and optionally a data source / schema / table), it walks the
// same discovery chain the data-quality job-creation wizard uses: resolve the
// connection, detect the job type (PUSHDOWN/PULLUP) from its capabilities, and
// enumerate data sources, schemas, tables, and columns. It returns a status
// that tells the agent whether it has everything needed to call create_dq_job,
// what's still missing (with the options to choose from), or what couldn't be
// resolved. It performs NO mutations.
package prepare_create_dq_job

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// maxOptions caps how many options are returned in any one response.
const maxOptions = 200

// pageLimit is the per-request page size for the monitoring/edge list endpoints,
// which reject limit > 100 with a 400 VALIDATION_ERROR.
const pageLimit = 100

// Status is the discovery outcome that drives the next conversational turn.
type Status string

const (
	// StatusReady means connection + data source + schema + table all resolved;
	// `resolved` holds the exact inputs to pass to create_dq_job.
	StatusReady Status = "ready"
	// StatusIncomplete means a required selection is missing; the response
	// includes the pre-fetched options for the next field to choose.
	StatusIncomplete Status = "incomplete"
	// StatusNeedsClarification means an input could not be resolved (unknown
	// connection, ambiguous job type, etc.); options for recovery are included.
	StatusNeedsClarification Status = "needs_clarification"
)

// Input is the tool's typed input. Provide as much as is known; omit a field to
// enumerate the options for it.
type Input struct {
	// TableAssetID resolves the whole data location from a catalog Table asset (the "from a table
	// asset page" entry point). When set, the tool walks Table -> Schema -> Database -> System and
	// matches the DQ connection by its systemAssetId, then enumerates as usual — no need to pass
	// connection/dataSource/schema/table.
	TableAssetID string `json:"tableAssetId,omitempty" jsonschema:"DGC catalog Table asset UUID. When provided, the connection/dataSource/schema/table are resolved from the asset (via the connection's systemAssetId mapping); the other location fields can be omitted."`
	// TableAssetURL / TableAssetName are alternative ways to identify the catalog Table asset (the
	// AC's URL / name identifiers). tableAssetId wins if set; then URL; then name (with optional
	// tableAssetDomain to disambiguate). Name lookup uses the public assets API (no search index).
	TableAssetURL    string `json:"tableAssetUrl,omitempty" jsonschema:"Catalog Table asset URL (e.g. https://<instance>/asset/<uuid>). The asset UUID is extracted from it."`
	TableAssetName   string `json:"tableAssetName,omitempty" jsonschema:"Catalog Table asset name (signifier, e.g. 'transactions'). Resolved via the public assets API; if multiple match, the options are returned for the user to pick one (optionally narrow with tableAssetDomain)."`
	TableAssetDomain string `json:"tableAssetDomain,omitempty" jsonschema:"Optional domain/path substring (e.g. 'sales') to disambiguate when tableAssetName matches multiple assets."`

	Connection     string `json:"connection,omitempty" jsonschema:"The data-quality edge connection — accepts a connection UUID or its name (case-insensitive, e.g. 'POSTGRES-SOURCE'). On a catalog table-asset page pass tableAssetId instead; from an LLM, omit it to list available connections and ask the user."`
	DataSourceName string `json:"dataSourceName,omitempty" jsonschema:"The database/catalog within the connection (e.g. 'postgres'). Omit to list the data sources available on the resolved connection."`
	SchemaName     string `json:"schemaName,omitempty" jsonschema:"The schema within the data source (e.g. 'sales'). Omit to list the schemas in the resolved data source."`
	TableName      string `json:"tableName,omitempty" jsonschema:"The table to monitor (e.g. 'customers'). Omit to list the tables in the resolved schema."`
}

// Output is the structured discovery response.
type Output struct {
	Status                  Status                             `json:"status" jsonschema:"ready when connection+dataSource+schema+table are all resolved; incomplete when a selection is missing (options provided); needs_clarification when an input could not be resolved."`
	Message                 string                             `json:"message" jsonschema:"Human-readable summary of the outcome and what to do next."`
	Resolved                *ResolvedPlan                      `json:"resolved,omitempty" jsonschema:"Present only when status=ready. Pass these fields straight to create_dq_job."`
	JobType                 string                             `json:"jobType,omitempty" jsonschema:"Detected job type for the resolved connection: PUSHDOWN or PULLUP. Empty when the connection advertises more than one and the user must choose."`
	ConnectionOptions       []ConnectionOption                 `json:"connectionOptions,omitempty" jsonschema:"Connections to choose from — returned when 'connection' was omitted or could not be resolved."`
	TableAssetOptions       []TableAssetOption                 `json:"tableAssetOptions,omitempty" jsonschema:"Returned when tableAssetName matched multiple Table assets. Present these to the user; re-call with the chosen asset's id as tableAssetId."`
	DataSourceOptions       []string                           `json:"dataSourceOptions,omitempty" jsonschema:"Data source names to choose from — returned when 'dataSourceName' was omitted or unmatched."`
	SchemaOptions           []string                           `json:"schemaOptions,omitempty" jsonschema:"Schema names to choose from — returned when 'schemaName' was omitted or unmatched."`
	TableOptions            []string                           `json:"tableOptions,omitempty" jsonschema:"Table names to choose from — returned when 'tableName' was omitted or unmatched."`
	Columns                 []ColumnInfo                       `json:"columns,omitempty" jsonschema:"Columns of the resolved table (the actual schema). Offer these so the user can pick a subset to monitor, then pass the chosen names to create_dq_job.selectedColumns. Omit selectedColumns to monitor all columns (the default)."`
	Monitors                []clients.DqMonitorInfo            `json:"monitors,omitempty" jsonschema:"Present only when status=ready. The available profile monitors, each with a defaultEnabled flag. Show these to the user so they can choose a set, then pass the chosen keys to create_dq_job.monitors (omit to use the defaults). Note: enabling descriptiveStatistics unmasks sensitive data."`
	AdaptiveMonitorSettings []clients.DqAdaptiveMonitorSetting `json:"adaptiveMonitorSettings,omitempty" jsonschema:"Present only when status=ready. The 'Advanced monitor settings' (adaptive behavior) the user can tune, each with its default. ALWAYS surface these together with monitors — do not omit them: tell the user data lookback and learning phase are adjustable (defaults 10 and 4) and pass overrides to create_dq_job.dataLookback / learningPhase."`
	Notifications           []clients.DqNotificationInfo       `json:"notifications,omitempty" jsonschema:"Present only when status=ready. The available notification alerts, each with a defaultEnabled flag and whether it takes a threshold quantity. Offer these to the user; pass the chosen keys to create_dq_job.notify (+ thresholds + notifyRecipients). The invoking user is always a recipient; additional recipients are validated against active accounts."`
	UnsupportedOptions      []string                           `json:"unsupportedOptions,omitempty" jsonschema:"Present only when status=ready. The wizard options this tool does NOT set, each with the default the server will apply. Present these to the user PROACTIVELY at the ready step (do not wait to be asked) so they know what is auto-filled and what would require the DQ UI."`
	OptionsTruncated        bool                               `json:"optionsTruncated" jsonschema:"True when an options list was truncated below the instance's true total."`
}

// ResolvedPlan is the set of inputs create_dq_job needs, fully resolved.
type ResolvedPlan struct {
	JobType             string `json:"jobType" jsonschema:"PUSHDOWN or PULLUP. Empty if the connection supports both and the user must pick."`
	SuggestedJobName    string `json:"suggestedJobName" jsonschema:"Default job name '<schema>.<table>'. The user may override; the server auto-increments on collision."`
	EdgeSiteName        string `json:"edgeSiteName" jsonschema:"dataLocation.edgeSiteName for create_dq_job."`
	EdgeConnectionName  string `json:"edgeConnectionName" jsonschema:"dataLocation.edgeConnectionName for create_dq_job."`
	DataSourceName      string `json:"dataSourceName" jsonschema:"dataLocation.dataSourceName for create_dq_job."`
	SchemaName          string `json:"schemaName" jsonschema:"dataLocation.schemaName for create_dq_job."`
	TableName           string `json:"tableName" jsonschema:"dataLocation.tableName for create_dq_job."`
	DatabaseProductName string `json:"databaseProductName,omitempty" jsonschema:"dataLocation.databaseProductName for create_dq_job (e.g. POSTGRES)."`
	TableAssetLink      string `json:"tableAssetLink,omitempty" jsonschema:"Catalog deep-link path to the resolved Table asset (relative to the instance URL). Present only when resolved from tableAssetId."`
}

// ConnectionOption is one selectable connection.
type ConnectionOption struct {
	ConnectionID    string   `json:"connectionId" jsonschema:"Connection UUID."`
	ConnectionName  string   `json:"connectionName" jsonschema:"Connection display name."`
	CapabilityTypes []string `json:"capabilityTypes" jsonschema:"DQ capabilities the connection supports (PUSHDOWN/PULLUP) — i.e. the possible job types."`
	DatabaseProduct string   `json:"databaseProductName,omitempty" jsonschema:"Database vendor (e.g. POSTGRES)."`
}

// TableAssetOption is one catalog Table asset candidate for disambiguation.
type TableAssetOption struct {
	AssetID     string `json:"assetId" jsonschema:"Catalog Table asset UUID — pass as tableAssetId to select it."`
	DisplayName string `json:"displayName" jsonschema:"Table signifier (name)."`
	Domain      string `json:"domain,omitempty" jsonschema:"Domain/path of the asset (helps tell duplicates apart)."`
	FullName    string `json:"fullName,omitempty" jsonschema:"Fully-qualified asset name."`
}

// ColumnInfo is one column of the resolved table.
type ColumnInfo struct {
	Name string `json:"name" jsonschema:"Column name."`
	Type string `json:"type" jsonschema:"Source column type (e.g. int4, text, numeric, timestamp)."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "prepare_create_dq_job",
		Title: "Prepare to Create Data Quality Job",
		Description: "Read-only companion to create_dq_job. Walks the data-quality wizard's discovery chain — resolve the edge " +
			"connection, detect the job type (PUSHDOWN/PULLUP) from its capabilities, and enumerate data sources, schemas, " +
			"tables, and columns — driven by whatever inputs you already have. Returns status='ready' with a fully-resolved " +
			"plan to hand to create_dq_job, 'incomplete' with the options for the next field to pick, or 'needs_clarification' " +
			"when something couldn't be resolved. Call this to gather/validate inputs before create_dq_job; make one selection " +
			"per turn (connection -> dataSource -> schema -> table) and re-call until status='ready'.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		// Step 0: catalog table-asset entry point. Identify the Table asset by id, URL, or name
		// (signifier), then resolve the full data location from it (Table -> Schema -> Database ->
		// System -> connection via systemAssetId) and fall through to the normal discovery chain.
		assetID := strings.TrimSpace(input.TableAssetID)
		if assetID == "" && strings.TrimSpace(input.TableAssetURL) != "" {
			assetID = extractAssetID(input.TableAssetURL)
			if assetID == "" {
				return Output{Status: StatusNeedsClarification, Message: fmt.Sprintf("Could not extract an asset UUID from URL %q. Use a catalog asset URL like .../asset/<uuid>, or pass tableAssetId.", input.TableAssetURL)}, nil
			}
		}
		if assetID == "" && strings.TrimSpace(input.TableAssetName) != "" {
			matches, err := clients.FindTableAssetsByName(ctx, collibraClient, strings.TrimSpace(input.TableAssetName), maxOptions)
			if err != nil {
				return Output{Status: StatusNeedsClarification, Message: fmt.Sprintf("Could not look up Table assets named %q: %v", input.TableAssetName, err)}, nil
			}
			if d := strings.TrimSpace(input.TableAssetDomain); d != "" {
				matches = filterByDomain(matches, d)
			}
			switch len(matches) {
			case 0:
				return Output{Status: StatusNeedsClarification, Message: fmt.Sprintf("No Table asset named %q was found. Provide a more specific name (optionally tableAssetDomain), the table asset URL, or the table asset ID.", input.TableAssetName)}, nil
			case 1:
				assetID = matches[0].ID
			default:
				return Output{
					Status:            StatusNeedsClarification,
					Message:           fmt.Sprintf("%d Table assets are named %q — pick one and call again with its assetId as tableAssetId (or narrow with tableAssetDomain).", len(matches), input.TableAssetName),
					TableAssetOptions: toTableAssetOptions(matches),
				}, nil
			}
		}
		if assetID != "" {
			loc, err := clients.ResolveDqLocationFromTableAsset(ctx, collibraClient, assetID)
			if err != nil {
				return Output{
					Status:  StatusNeedsClarification,
					Message: fmt.Sprintf("Could not resolve table asset %s to a DQ location: %v", assetID, err),
				}, nil
			}
			input.Connection = loc.ConnectionID
			input.DataSourceName = loc.DataSourceName
			input.SchemaName = loc.SchemaName
			input.TableName = loc.TableName
			input.TableAssetID = assetID // so the ready block emits the catalog deep link
		}

		// Step 1: a connection is the entry point. Without one, list options.
		if strings.TrimSpace(input.Connection) == "" {
			return enumerateConnections(ctx, collibraClient, "Provide a connection (or a tableAssetId) to begin. Pick one from connectionOptions and call again.")
		}

		conn, err := resolveConnection(ctx, collibraClient, input.Connection)
		if err != nil {
			return enumerateConnections(ctx, collibraClient,
				fmt.Sprintf("Could not resolve connection %q: %v. Pick one from connectionOptions and call again.", input.Connection, err))
		}

		// Job type comes straight off the connection's capabilities.
		jobType, jobTypeMsg := detectJobType(conn)
		out := Output{JobType: jobType}

		// Step 2: data source.
		dataSources, err := clients.ListDqDataSources(ctx, collibraClient, conn.ConnectionID, pageLimit, 0)
		if err != nil {
			return Output{Status: StatusNeedsClarification, Message: fmt.Sprintf("Resolved connection %q but failed to list data sources: %v", conn.ConnectionName, err)}, nil
		}
		dsNames := dataSourceNames(dataSources)
		if strings.TrimSpace(input.DataSourceName) == "" {
			out.Status = StatusIncomplete
			out.Message = joinMsg(jobTypeMsg, fmt.Sprintf("dataSourceName is required. Pick one from dataSourceOptions on connection %q and call again.", conn.ConnectionName))
			out.DataSourceOptions, out.OptionsTruncated = capStrings(dsNames)
			return out, nil
		}
		matchedDS, ok := matchIgnoreCase(dsNames, input.DataSourceName)
		if !ok {
			out.Status = StatusNeedsClarification
			out.Message = fmt.Sprintf("Data source %q was not found on connection %q. Pick one from dataSourceOptions.", input.DataSourceName, conn.ConnectionName)
			out.DataSourceOptions, out.OptionsTruncated = capStrings(dsNames)
			return out, nil
		}

		// Step 3: schema.
		schemas, err := clients.ListDqSchemas(ctx, collibraClient, conn.EdgeSiteID, conn.ConnectionID, matchedDS, pageLimit, 0)
		if err != nil {
			return Output{Status: StatusNeedsClarification, Message: fmt.Sprintf("Failed to list schemas in data source %q: %v", matchedDS, err)}, nil
		}
		schemaNames := schemaNames(schemas)
		if strings.TrimSpace(input.SchemaName) == "" {
			out.Status = StatusIncomplete
			out.Message = joinMsg(jobTypeMsg, fmt.Sprintf("schemaName is required. Pick one from schemaOptions in data source %q and call again.", matchedDS))
			out.SchemaOptions, out.OptionsTruncated = capStrings(schemaNames)
			return out, nil
		}
		matchedSchema, ok := matchIgnoreCase(schemaNames, input.SchemaName)
		if !ok {
			out.Status = StatusNeedsClarification
			out.Message = fmt.Sprintf("Schema %q was not found in data source %q. Pick one from schemaOptions.", input.SchemaName, matchedDS)
			out.SchemaOptions, out.OptionsTruncated = capStrings(schemaNames)
			return out, nil
		}

		// Step 4: table.
		tables, err := clients.ListDqTables(ctx, collibraClient, conn.EdgeSiteID, conn.ConnectionID, matchedDS, matchedSchema, pageLimit, 0)
		if err != nil {
			return Output{Status: StatusNeedsClarification, Message: fmt.Sprintf("Failed to list tables in schema %q: %v", matchedSchema, err)}, nil
		}
		tableNames := tableNames(tables)
		if strings.TrimSpace(input.TableName) == "" {
			out.Status = StatusIncomplete
			out.Message = joinMsg(jobTypeMsg, fmt.Sprintf("tableName is required. Pick one from tableOptions in schema %q and call again.", matchedSchema))
			out.TableOptions, out.OptionsTruncated = capStrings(tableNames)
			return out, nil
		}
		matchedTable, ok := matchIgnoreCase(tableNames, input.TableName)
		if !ok {
			out.Status = StatusNeedsClarification
			out.Message = fmt.Sprintf("Table %q was not found in schema %q. Pick one from tableOptions.", input.TableName, matchedSchema)
			out.TableOptions, out.OptionsTruncated = capStrings(tableNames)
			return out, nil
		}

		// Everything resolved — surface columns and the ready-to-create plan.
		columns, err := clients.ListDqColumns(ctx, collibraClient, conn.EdgeSiteID, conn.ConnectionID, matchedDS, matchedSchema, matchedTable, pageLimit, 0)
		if err != nil {
			// Columns are advisory; don't fail the resolution if they can't be fetched.
			out.Message = fmt.Sprintf("(columns could not be fetched: %v) ", err)
		} else {
			out.Columns = toColumnInfos(columns)
		}

		out.Status = StatusReady
		out.Monitors = clients.DqMonitorCatalog()
		out.AdaptiveMonitorSettings = clients.DqAdaptiveMonitorSettings()
		out.Notifications = clients.DqNotificationCatalog()
		out.UnsupportedOptions = clients.UnsupportedWizardOptions(jobType)

		// Suggested job name: ask the server for a collision-free default (auto-increments, e.g. "..._2").
		suggestedName := matchedSchema + "." + matchedTable
		if generated, gErr := clients.GenerateUniqueJobName(ctx, collibraClient, matchedSchema, matchedTable); gErr == nil && strings.TrimSpace(generated) != "" {
			suggestedName = generated
		}

		// Permission preflight (best-effort): Create is a hard prerequisite — if it's missing, the flow
		// should not begin. Schedule/Run gaps are surfaced as a note (create_dq_job degrades gracefully).
		permNote := ""
		if global, resource, permErr := clients.GetDqConnectionPermissions(ctx, collibraClient, conn.ConnectionID); permErr == nil {
			has := func(p string) bool { return clients.HasPermission(global, p) || clients.HasPermission(resource, p) }
			manageAll := has(clients.PermResourceManageAll)
			if !manageAll && !has(clients.PermDqJobCreate) {
				return Output{
					Status:  StatusNeedsClarification,
					JobType: jobType,
					Message: fmt.Sprintf("You do not have the Data Quality Job > Create permission (DATA_QUALITY_JOB_CREATE) on connection %q required to create a DQ job on %s.%s, so the flow cannot begin. Ask an administrator for the Data Quality Editor or Data Quality Manager role on this connection.", conn.ConnectionName, matchedSchema, matchedTable),
				}, nil
			}
			var missing []string
			if !manageAll && !has(clients.PermDqJobSchedule) {
				missing = append(missing, "Schedule")
			}
			if !manageAll && !has(clients.PermDqJobRun) {
				missing = append(missing, "Run")
			}
			if len(missing) > 0 {
				permNote = fmt.Sprintf(" NOTE: you lack the Data Quality Job > %s permission(s) on this connection; create_dq_job will warn and degrade (a schedule is dropped; a run may be rejected server-side).", strings.Join(missing, " and "))
			}
		}

		out.Resolved = &ResolvedPlan{
			JobType:             jobType,
			SuggestedJobName:    suggestedName,
			EdgeSiteName:        conn.EdgeSiteName,
			EdgeConnectionName:  conn.ConnectionName,
			DataSourceName:      matchedDS,
			SchemaName:          matchedSchema,
			TableName:           matchedTable,
			DatabaseProductName: conn.DatabaseProductName,
		}
		if id := strings.TrimSpace(input.TableAssetID); id != "" {
			out.Resolved.TableAssetLink = clients.CatalogAssetPath(id)
		}
		out.Message += fmt.Sprintf("Ready. Resolved a %s job for %s.%s on connection %q (%d column(s)). Before creating, PROACTIVELY tell the user (do not wait for them to ask): (1) only connection/data source/schema/table are required — everything else is auto-filled, and the exact defaults are listed in unsupportedOptions; (2) offer them a custom job name (default %q); (3) offer column selection (a subset from `columns` → create_dq_job.selectedColumns; default = all %d) and an optional single-column row filter (create_dq_job.filterColumn/filterOperator/filterValue, e.g. amount > 100); (4) walk the Monitors step, which has THREE parts you must each mention: (a) monitor toggles — `monitors` lists them with defaultEnabled flags, pass a custom set to create_dq_job.monitors (enabling descriptiveStatistics unmasks sensitive data); (b) ADVANCED monitor settings — explicitly tell the user that Data Lookback and Learning Phase are adjustable (see `adaptiveMonitorSettings`; defaults 10 and 4) and can be overridden via create_dq_job.dataLookback/learningPhase; (c) row sampling — create_dq_job.sampleSize (default = all rows); (5) for recurring monitoring, offer a schedule (create_dq_job.scheduleRepeat = HOURLY/DAILY/WEEKLY/WEEKDAYS/MONTHLY + scheduleRunTime) and PAIR it with a timeSliceColumn (a date/timestamp column from `columns`) so each run scans only that period's slice — the tool writes a ${rd}/${rdEnd} WHERE for you; without a timeSliceColumn every run rescans the whole table; (6) offer notifications — `notifications` lists the available alerts with defaultEnabled flags; pass chosen keys to create_dq_job.notify (+ thresholds), and additional recipients via create_dq_job.notifyRecipients (the invoking user is always included; others are validated against active accounts); (7) note those listed options are not configurable through this tool and would need the DQ UI. Then call create_dq_job with `resolved` — it returns a preview before anything is created.",
			displayJobType(jobType), matchedSchema, matchedTable, conn.ConnectionName, len(out.Columns), suggestedName, len(out.Columns))
		out.Message += " (8) for a PULLUP job, optionally Size Job Resources — automatic by default (recommended); manual sizing (create_dq_job.sizing*) and Parallel JDBC (create_dq_job.parallelJdbc*) are advanced; for a PUSHDOWN job, optionally set compute via create_dq_job.pushdownConnections/pushdownThreads (defaults 10/2). Per-notification messages can be set via create_dq_job.notifyMessages." + permNote
		if jobTypeMsg != "" {
			out.Message = joinMsg(jobTypeMsg, out.Message)
		}
		return out, nil
	}
}

// resolveConnection matches the input against connections by UUID or name
// (case-insensitive). Errors carry enough context for enumerateConnections.
func resolveConnection(ctx context.Context, client *http.Client, identifier string) (*clients.DqConnection, error) {
	conns, err := clients.ListDqConnections(ctx, client)
	if err != nil {
		return nil, err
	}
	id := strings.TrimSpace(identifier)
	var byName []clients.DqConnection
	for i := range conns {
		if conns[i].ConnectionID == id {
			return &conns[i], nil
		}
		if strings.EqualFold(conns[i].ConnectionName, id) {
			byName = append(byName, conns[i])
		}
	}
	switch len(byName) {
	case 1:
		return &byName[0], nil
	case 0:
		return nil, fmt.Errorf("no connection matched")
	default:
		return nil, fmt.Errorf("%d connections share that name — use the connection UUID", len(byName))
	}
}

// detectJobType returns the single capability type, or "" plus a clarifying
// message when the connection advertises zero or multiple.
func detectJobType(conn *clients.DqConnection) (string, string) {
	switch len(conn.CapabilityTypes) {
	case 1:
		return conn.CapabilityTypes[0], ""
	case 0:
		return "", fmt.Sprintf("Note: connection %q advertises no DQ capability — it may not support data-quality jobs.", conn.ConnectionName)
	default:
		return "", fmt.Sprintf("Note: connection %q supports multiple job types (%s); set jobType explicitly when calling create_dq_job.",
			conn.ConnectionName, strings.Join(conn.CapabilityTypes, ", "))
	}
}

func enumerateConnections(ctx context.Context, client *http.Client, message string) (Output, error) {
	conns, err := clients.ListDqConnections(ctx, client)
	if err != nil {
		return Output{Status: StatusNeedsClarification, Message: fmt.Sprintf("Failed to list connections: %v", err)}, nil
	}
	opts := make([]ConnectionOption, 0, len(conns))
	for _, c := range conns {
		opts = append(opts, ConnectionOption{
			ConnectionID:    c.ConnectionID,
			ConnectionName:  c.ConnectionName,
			CapabilityTypes: c.CapabilityTypes,
			DatabaseProduct: c.DatabaseProductName,
		})
	}
	truncated := false
	if len(opts) > maxOptions {
		opts = opts[:maxOptions]
		truncated = true
	}
	return Output{
		Status:            StatusIncomplete,
		Message:           message,
		ConnectionOptions: opts,
		OptionsTruncated:  truncated,
	}, nil
}

func dataSourceNames(in []clients.DqDataSource) []string {
	out := make([]string, 0, len(in))
	for _, d := range in {
		out = append(out, d.DataSourceName)
	}
	return out
}

func schemaNames(in []clients.DqSchema) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		out = append(out, s.Name)
	}
	return out
}

func tableNames(in []clients.DqTable) []string {
	out := make([]string, 0, len(in))
	for _, t := range in {
		out = append(out, t.Name)
	}
	return out
}

func toColumnInfos(in []clients.DqColumn) []ColumnInfo {
	out := make([]ColumnInfo, 0, len(in))
	for _, c := range in {
		out = append(out, ColumnInfo{Name: c.Name, Type: c.Type})
	}
	return out
}

func matchIgnoreCase(options []string, value string) (string, bool) {
	for _, o := range options {
		if strings.EqualFold(o, value) {
			return o, true
		}
	}
	return "", false
}

func capStrings(in []string) ([]string, bool) {
	if len(in) > maxOptions {
		return in[:maxOptions], true
	}
	return in, false
}

func joinMsg(a, b string) string {
	if a == "" {
		return b
	}
	return a + " " + b
}

// extractAssetID pulls a UUID out of a catalog asset URL (e.g. https://host/asset/<uuid>?tab=x).
// Returns "" if no UUID path segment is found.
func extractAssetID(raw string) string {
	raw = strings.TrimSpace(raw)
	if i := strings.IndexAny(raw, "?#"); i >= 0 {
		raw = raw[:i]
	}
	raw = strings.TrimRight(raw, "/")
	for _, seg := range strings.Split(raw, "/") {
		if _, err := uuid.Parse(seg); err == nil {
			return seg
		}
	}
	return ""
}

// filterByDomain keeps matches whose domain/path contains the given substring (case-insensitive).
func filterByDomain(in []clients.TableAssetMatch, domain string) []clients.TableAssetMatch {
	d := strings.ToLower(strings.TrimSpace(domain))
	var out []clients.TableAssetMatch
	for _, m := range in {
		if strings.Contains(strings.ToLower(m.DomainName), d) {
			out = append(out, m)
		}
	}
	return out
}

func toTableAssetOptions(in []clients.TableAssetMatch) []TableAssetOption {
	out := make([]TableAssetOption, 0, len(in))
	for _, m := range in {
		out = append(out, TableAssetOption{AssetID: m.ID, DisplayName: m.DisplayName, Domain: m.DomainName, FullName: m.FullName})
	}
	return out
}

func displayJobType(jobType string) string {
	if jobType == "" {
		return "data-quality"
	}
	return jobType
}
