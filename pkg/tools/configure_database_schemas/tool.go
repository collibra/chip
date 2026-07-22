// Package configure_database_schemas implements the configure_database_schemas MCP
// tool: discovers the schemas under an already-registered database and configures
// which schemas/tables get synchronized, via the public catalogDatabase API.
//
// This is the second of two steps in setting up a database for jdbc-ingestion — the
// first, registering the Database asset, is a separate tool (register_database),
// which returns the databaseConnectionId this tool takes as input. See
// register_database's package doc comment for why they're split rather than one call.
//
// Ambiguity guards: schemaNames is required when more than one schema is discovered,
// and include has no default — both force an explicit choice instead of silently
// syncing every schema/table. schemaNames/include still accept an explicit "*" to opt
// into "everything", but that must be a deliberate choice (confirmed with the user),
// never this tool's default behavior. Neither guard risks a dead end: schema discovery
// and selection have no side effects, so an ambiguity error here is a true no-op — the
// database registered by register_database is untouched, and retrying this tool call
// with the missing name(s) filled in just starts clean.
package configure_database_schemas

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/validation"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Sleep is used for polling backoff. Overridable in tests to avoid slow test runs.
var Sleep = time.Sleep

const (
	// pollAttempts * pollInterval bounds the wait for the async refresh. A
	// first-time connection to a data source can take 30-60s to establish, so
	// this budget needs enough headroom to cover that cold-start case.
	pollAttempts = 14
	pollInterval = 5 * time.Second
)

type Input struct {
	DatabaseConnectionID string   `json:"databaseConnectionId" jsonschema:"The databaseConnectionId from register_database's Database output — identifies which registered database's schemas to configure."`
	SchemaNames          []string `json:"schemaNames,omitempty" jsonschema:"Exact names of the schemas to configure for synchronization, as they appear at the data source. Required if the database exposes more than one schema; if there is exactly one, it is selected automatically. Pass the single value '*' to configure every discovered schema — only do this after confirming with the user which schemas they actually want synced, never as a silent default when multiple schemas exist."`

	Include                     string `json:"include" jsonschema:"Comma-separated table name pattern to synchronize, '*' wildcard supported. Required — there is no default. Confirm with the user which tables they want before calling; do not pass '*' (all tables) without asking first. Applies to every schema selected via schemaNames."`
	Exclude                     string `json:"exclude,omitempty" jsonschema:"Optional. Comma-separated table name pattern to exclude from synchronization."`
	TargetDomainID              string `json:"targetDomainId,omitempty" jsonschema:"Optional. UUID of a domain to create synchronized assets in. If omitted, an automatically created domain per schema is used."`
	SkipViews                   bool   `json:"skipViews,omitempty" jsonschema:"Optional. If true, database views are excluded from synchronization."`
	RegisterSourceTags          bool   `json:"registerSourceTags,omitempty" jsonschema:"Optional. If true, registers tags from the data source (when supported by the driver)."`
	IngestSemanticViews         bool   `json:"ingestSemanticViews,omitempty" jsonschema:"Optional. If true, ingests semantic views (when supported by the driver)."`
	RegisterDataUsageStatistics bool   `json:"registerDataUsageStatistics,omitempty" jsonschema:"Optional. If true, calculates data usage statistics for the synchronized tables (when supported by the data source)."`
}

type Output struct {
	SchemaConnections []clients.SchemaConnection `json:"schemaConnections,omitempty" jsonschema:"The schema connections discovered under the database, now configured for synchronization."`
	Success           bool                       `json:"success" jsonschema:"Whether the schemas were fully configured."`
	Error             string                     `json:"error,omitempty" jsonschema:"Error message if configuration failed or is still in progress (safe to retry)."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "configure_database_schemas",
		Title:       "Configure Database Schema Synchronization",
		Description: "Discovers the schemas under a database registered via register_database and configures which schemas/tables get synchronized. Prerequisite for start_ingestion. If more than one schema is discovered, or include isn't provided, this returns an error naming the candidates instead of guessing — confirm the schemas and table pattern with the user, do not default to configuring everything. Safe to call again later to change which schemas/tables are synced.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if err := validation.UUID("databaseConnectionId", input.DatabaseConnectionID); err != nil {
			return Output{}, err
		}
		if err := validation.UUIDOptional("targetDomainId", input.TargetDomainID); err != nil {
			return Output{}, err
		}
		if strings.TrimSpace(input.Include) == "" {
			return Output{}, fmt.Errorf("include is required — confirm with the user which tables to synchronize (or that they want '*', all tables) before calling")
		}

		discoveredSchemaConnections, err := discoverSchemaConnections(ctx, collibraClient, input.DatabaseConnectionID)
		if err != nil {
			return Output{Success: false, Error: err.Error()}, nil
		}

		schemaConnections, err := selectSchemaConnections(discoveredSchemaConnections, input.SchemaNames)
		if err != nil {
			return Output{Success: false, Error: err.Error()}, nil
		}

		configurations := make([]clients.SchemaMetadataConfiguration, len(schemaConnections))
		for i, schemaConnection := range schemaConnections {
			configurations[i] = clients.SchemaMetadataConfiguration{
				SchemaConnectionID: schemaConnection.ID,
				SynchronizationRules: []clients.MetadataSynchronizationRule{
					{
						Include:                     input.Include,
						Exclude:                     input.Exclude,
						TargetDomainID:              input.TargetDomainID,
						SkipViews:                   input.SkipViews,
						RegisterSourceTags:          input.RegisterSourceTags,
						IngestSemanticViews:         input.IngestSemanticViews,
						RegisterDataUsageStatistics: input.RegisterDataUsageStatistics,
					},
				},
			}
		}

		if _, err := clients.SetSchemaMetadataConfigurationsBatch(ctx, collibraClient, configurations); err != nil {
			return Output{SchemaConnections: schemaConnections, Success: false,
				Error: fmt.Sprintf("failed to set schema synchronization rules: %s", err.Error())}, nil
		}

		return Output{SchemaConnections: schemaConnections, Success: true}, nil
	}
}

// discoverSchemaConnections triggers a refresh of schema connections for the given
// database connection and polls until at least one appears.
func discoverSchemaConnections(ctx context.Context, client *http.Client, databaseConnectionID string) ([]clients.SchemaConnection, error) {
	if err := clients.RefreshSchemaConnections(ctx, client, databaseConnectionID); err != nil {
		return nil, fmt.Errorf("failed to refresh schema connections: %w", err)
	}

	for attempt := 0; attempt < pollAttempts; attempt++ {
		if attempt > 0 {
			Sleep(pollInterval)
		}
		found, err := clients.ListSchemaConnections(ctx, client, databaseConnectionID)
		if err != nil {
			return nil, fmt.Errorf("failed to list schema connections: %w", err)
		}
		if len(found) > 0 {
			return found, nil
		}
	}

	return nil, fmt.Errorf("no schemas were discovered for database connection %s after %d attempts. Either the refresh is still in progress (retry this tool call), or no capability referencing this connection exists yet — verify with edge_create_capability", databaseConnectionID, pollAttempts)
}

// selectSchemaConnections narrows discovered to the schemas named in schemaNames. An
// empty schemaNames is only accepted when there's no ambiguity (a single discovered
// schema); with more than one discovered schema, the caller must either name the ones
// they want or pass ["*"] to explicitly opt into configuring all of them.
func selectSchemaConnections(discovered []clients.SchemaConnection, schemaNames []string) ([]clients.SchemaConnection, error) {
	if len(schemaNames) == 1 && schemaNames[0] == "*" {
		return discovered, nil
	}

	if len(schemaNames) == 0 {
		if len(discovered) > 1 {
			names := make([]string, len(discovered))
			for i, s := range discovered {
				names[i] = s.Name
			}
			return nil, fmt.Errorf("multiple schemas discovered (%s); specify schemaNames to select which to configure, or pass [\"*\"] to configure all of them", strings.Join(names, ", "))
		}
		return discovered, nil
	}

	byName := make(map[string]clients.SchemaConnection, len(discovered))
	for _, s := range discovered {
		byName[s.Name] = s
	}

	selected := make([]clients.SchemaConnection, 0, len(schemaNames))
	var missing []string
	for _, name := range schemaNames {
		s, ok := byName[name]
		if !ok {
			missing = append(missing, name)
			continue
		}
		selected = append(selected, s)
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("schema(s) not found among discovered schemas: %s", strings.Join(missing, ", "))
	}

	return selected, nil
}
