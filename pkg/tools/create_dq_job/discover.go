// Discovery phase for create_data_quality_job. Folded in from the former prepare_create_data_quality_job
// tool: given whatever the caller knows so far (a connection, and optionally a data source / schema /
// table, or a catalog Table asset), it walks the same chain the DQ wizard uses — resolve the connection,
// detect the job type, and enumerate data sources, schemas, tables. Only the FIRST missing location field
// is enumerated (returned as options); fields the caller already supplied are trusted as-is (the public
// create validates them server-side). This keeps the fully-specified path a single build+preview with no
// extra browse round-trips, while an under-specified call still gets one-field-at-a-time discovery.
package create_dq_job

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/clients"
	"github.com/google/uuid"
)

// maxOptions caps how many options are returned in any one response.
const maxOptions = 200

// pageLimit is the per-request page size for the monitoring/edge list endpoints,
// which reject limit > 100 with a 400 VALIDATION_ERROR.
const pageLimit = 100

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

// discover resolves the data location from input, enumerating options for the FIRST missing field
// (connection → data source → schema → table). It mutates input in place (e.g. filling location fields
// from a catalog Table asset). When done=true, return `out` directly — it is an incomplete response
// (options for the next field) or a needs_input response (something couldn't be resolved). When
// done=false, conn + jobType are resolved and input's location fields are complete and trusted.
func discover(ctx context.Context, client *http.Client, input *Input) (conn *clients.DqConnection, jobType string, out Output, done bool) {
	// Catalog table-asset entry point: identify the Table asset by id, URL, or name, then resolve the
	// full location from it (Table -> Schema -> Database -> System -> connection via systemAssetId).
	// Only when no connection was given — if the caller supplied a connection they're using the
	// connection flow, and a tableAssetId is then just the linkage hint for the success deep link.
	assetID := strings.TrimSpace(input.TableAssetID)
	if strings.TrimSpace(input.Connection) == "" && assetID == "" && strings.TrimSpace(input.TableAssetURL) != "" {
		assetID = extractAssetID(input.TableAssetURL)
		if assetID == "" {
			return nil, "", Output{Status: StatusNeedsInput, Message: fmt.Sprintf("Could not extract an asset UUID from URL %q.", input.TableAssetURL), Guidance: "Use a catalog asset URL like .../asset/<uuid>, or pass tableAssetId."}, true
		}
	}
	if strings.TrimSpace(input.Connection) == "" && assetID == "" && strings.TrimSpace(input.TableAssetName) != "" {
		matches, err := clients.FindTableAssetsByName(ctx, client, strings.TrimSpace(input.TableAssetName), maxOptions)
		if err != nil {
			return nil, "", Output{Status: StatusNeedsInput, Message: fmt.Sprintf("Could not look up Table assets named %q: %v", input.TableAssetName, err)}, true
		}
		if d := strings.TrimSpace(input.TableAssetDomain); d != "" {
			matches = filterByDomain(matches, d)
		}
		switch len(matches) {
		case 0:
			return nil, "", Output{Status: StatusNeedsInput, Message: fmt.Sprintf("No Table asset named %q was found.", input.TableAssetName), Guidance: "Provide a more specific name (optionally tableAssetDomain), the table asset URL, or the table asset ID."}, true
		case 1:
			assetID = matches[0].ID
		default:
			return nil, "", Output{
				Status:            StatusNeedsInput,
				Message:           fmt.Sprintf("%d Table assets are named %q — pick one and re-call with its assetId as tableAssetId (or narrow with tableAssetDomain).", len(matches), input.TableAssetName),
				TableAssetOptions: toTableAssetOptions(matches),
			}, true
		}
	}
	if strings.TrimSpace(input.Connection) == "" && assetID != "" {
		loc, err := clients.ResolveDqLocationFromTableAsset(ctx, client, assetID)
		if err != nil {
			return nil, "", Output{Status: StatusNeedsInput, Message: fmt.Sprintf("Could not resolve table asset %s to a DQ location: %v", assetID, err)}, true
		}
		input.Connection = loc.ConnectionID
		input.DataSourceName = loc.DataSourceName
		input.SchemaName = loc.SchemaName
		input.TableName = loc.TableName
		input.TableAssetID = assetID // so the create result emits the catalog deep link
	}

	// A connection is the entry point. Without one, list options.
	if strings.TrimSpace(input.Connection) == "" {
		return nil, "", enumerateConnections(ctx, client, "Provide a connection (or a tableAssetId) to begin. Pick one from connectionOptions and re-call."), true
	}

	conn, err := resolveConnection(ctx, client, input.Connection)
	if err != nil {
		return nil, "", enumerateConnections(ctx, client,
			fmt.Sprintf("Could not resolve connection %q: %v. Pick one from connectionOptions and re-call.", input.Connection, err)), true
	}
	jobType = resolveJobType(input.JobType, conn)

	// Data source: enumerate only when the caller didn't supply one.
	if strings.TrimSpace(input.DataSourceName) == "" {
		dataSources, dsErr := clients.ListDqDataSources(ctx, client, conn.ConnectionID, pageLimit, 0)
		if dsErr != nil {
			return nil, "", Output{Status: StatusNeedsInput, JobType: jobType, Message: fmt.Sprintf("Resolved connection %q but failed to list data sources: %v", conn.ConnectionName, dsErr)}, true
		}
		opts, truncated := capStrings(dataSourceNames(dataSources))
		return nil, "", Output{
			Status:            StatusIncomplete,
			JobType:           jobType,
			Message:           fmt.Sprintf("dataSourceName is required. Pick one from dataSourceOptions on connection %q and re-call.", conn.ConnectionName),
			DataSourceOptions: opts,
			OptionsTruncated:  truncated,
		}, true
	}

	// Schema.
	if strings.TrimSpace(input.SchemaName) == "" {
		schemas, schErr := clients.ListDqSchemas(ctx, client, conn.EdgeSiteID, conn.ConnectionID, input.DataSourceName, pageLimit, 0)
		if schErr != nil {
			return nil, "", Output{Status: StatusNeedsInput, JobType: jobType, Message: fmt.Sprintf("Failed to list schemas in data source %q: %v", input.DataSourceName, schErr)}, true
		}
		opts, truncated := capStrings(schemaNames(schemas))
		return nil, "", Output{
			Status:           StatusIncomplete,
			JobType:          jobType,
			Message:          fmt.Sprintf("schemaName is required. Pick one from schemaOptions in data source %q and re-call.", input.DataSourceName),
			SchemaOptions:    opts,
			OptionsTruncated: truncated,
		}, true
	}

	// Table.
	if strings.TrimSpace(input.TableName) == "" {
		tables, tblErr := clients.ListDqTables(ctx, client, conn.EdgeSiteID, conn.ConnectionID, input.DataSourceName, input.SchemaName, pageLimit, 0)
		if tblErr != nil {
			return nil, "", Output{Status: StatusNeedsInput, JobType: jobType, Message: fmt.Sprintf("Failed to list tables in schema %q: %v", input.SchemaName, tblErr)}, true
		}
		opts, truncated := capStrings(tableNames(tables))
		return nil, "", Output{
			Status:           StatusIncomplete,
			JobType:          jobType,
			Message:          fmt.Sprintf("tableName is required. Pick one from tableOptions in schema %q and re-call.", input.SchemaName),
			TableOptions:     opts,
			OptionsTruncated: truncated,
		}, true
	}

	return conn, jobType, Output{}, false
}

// fetchColumns lists the resolved table's columns for the preview (so the caller can offer a
// selectedColumns subset). Advisory: on error it returns nil and the preview omits columns.
func fetchColumns(ctx context.Context, client *http.Client, conn *clients.DqConnection, input Input) []ColumnInfo {
	cols, err := clients.ListDqColumns(ctx, client, conn.EdgeSiteID, conn.ConnectionID, input.DataSourceName, input.SchemaName, input.TableName, pageLimit, 0)
	if err != nil {
		return nil
	}
	return toColumnInfos(cols)
}

// resolveConnection matches the input against connections by UUID or name (case-insensitive).
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

// resolveJobType returns the caller's explicit jobType if set, else the connection's single capability,
// else "" when the connection advertises zero or multiple (the handler requires an explicit jobType then).
func resolveJobType(override string, conn *clients.DqConnection) string {
	if jt := strings.ToUpper(strings.TrimSpace(override)); jt != "" {
		return jt
	}
	if len(conn.CapabilityTypes) == 1 {
		return conn.CapabilityTypes[0]
	}
	return ""
}

func enumerateConnections(ctx context.Context, client *http.Client, message string) Output {
	conns, err := clients.ListDqConnections(ctx, client)
	if err != nil {
		return Output{Status: StatusNeedsInput, Message: fmt.Sprintf("Failed to list connections: %v", err)}
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
	}
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

func capStrings(in []string) ([]string, bool) {
	if len(in) > maxOptions {
		return in[:maxOptions], true
	}
	return in, false
}

// extractAssetID pulls a UUID out of a catalog asset URL (e.g. https://host/asset/<uuid>?tab=x).
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
