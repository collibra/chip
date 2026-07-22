// Package get_data_source_setup_guide implements the get_data_source_setup_guide MCP
// tool: looks up driver class, JDBC connection string format, and per-auth-method
// connection properties for any of Collibra's ~68 supported data sources, so
// edge_create_connection can be filled in correctly without the agent (or the user)
// needing to already know a given vendor's JDBC driver properties.
//
// This is a documented gap, not a guaranteed integration: the underlying data comes
// from scraping Collibra's public product documentation site (see the package doc
// on clients.GetConnectionProperties for the full explanation and how to re-derive
// it if it breaks). There is no published API for this — Collibra could restructure
// their docs site at any time without notice. Treat a failure from this tool as "ask
// the user for the connection properties directly," not a bug to retry around.
package get_data_source_setup_guide

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	DataSource string `json:"dataSource" jsonschema:"The data source to look up (e.g. 'Snowflake', 'PostgreSQL', 'Amazon Redshift'). Matched case-insensitively against Collibra's ~68 supported data sources; if ambiguous or not found, the response lists candidates to retry with."`
}

type Output struct {
	DataSourceID           string                         `json:"dataSourceId,omitempty" jsonschema:"The matched data source's internal id (slug)."`
	DataSourceName         string                         `json:"dataSourceName,omitempty" jsonschema:"The matched data source's display name."`
	DriverClassName        string                         `json:"driverClassName,omitempty" jsonschema:"The JDBC driver class name for edge_create_connection's 'driver-class' parameter."`
	ConnectionStringFormat string                         `json:"connectionStringFormat,omitempty" jsonschema:"The JDBC connection string format for edge_create_connection's 'connection-string' parameter, with placeholders (e.g. 'jdbc:snowflake://<accountname>.snowflakecomputing.com')."`
	MarketplaceURL         string                         `json:"marketplaceUrl,omitempty" jsonschema:"Collibra Marketplace listing URL for this data source's JDBC driver, if the driver needs to be downloaded before uploading via edge_create_connection's driverJarUrl/driverJarFilename or the upload_file tool."`
	Prerequisites          string                         `json:"prerequisites,omitempty" jsonschema:"Documented prerequisites before creating this connection."`
	AuthMethods            []clients.AuthMethodProperties `json:"authMethods,omitempty" jsonschema:"Documented connection properties, grouped by supported authentication method. Pick the method matching what credentials the user has, then pass its properties as edge_create_connection's additionalProperties (name/type/value/secret per entry — secret=true when the property's Value Type here is 'Secret'). A 'File' type property's value must first be uploaded via upload_file or edge_create_connection's driverJarUrl, then referenced by the returned artifact URI."`
	Matches                []clients.DataSourceSlug       `json:"matches,omitempty" jsonschema:"Candidate data sources when the lookup was ambiguous or not found. Retry with one of these exact ids."`
	Success                bool                           `json:"success" jsonschema:"Whether at least some setup information was found."`
	Error                  string                         `json:"error,omitempty" jsonschema:"Details if part or all of the lookup failed. A failure here does not mean the data source is unsupported by Edge — it means this best-effort documentation lookup didn't find it; ask the user for the connection properties directly."`
}

func NewTool(_ *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "get_data_source_setup_guide",
		Title:       "Get Data Source Connection Setup Guide",
		Description: "Looks up driver class, JDBC connection string format, and per-auth-method connection properties for a data source, from Collibra's public documentation (best-effort scrape, not a stable API — see tool source for details). Use before edge_create_connection when the required parameters for a data source (e.g. Snowflake's Role/Warehouse/private key) aren't already known.",
		Handler:     handler(),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

// DocsClient hits Collibra's public documentation site directly, never the tenant's
// Collibra instance — it must not use the injected, tenant-scoped Collibra client.
// Overridable in tests.
var DocsClient = http.DefaultClient

func handler() chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if strings.TrimSpace(input.DataSource) == "" {
			return Output{}, fmt.Errorf("dataSource is required")
		}

		sources, err := clients.ListDataSources(ctx, DocsClient)
		if err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to list data sources: %s", err.Error())}, nil
		}

		matches := matchDataSources(sources, input.DataSource)
		if len(matches) == 0 {
			return Output{Success: false, Error: fmt.Sprintf("no data source found matching %q", input.DataSource), Matches: sources}, nil
		}
		if len(matches) > 1 {
			return Output{Success: false, Error: fmt.Sprintf("multiple data sources matched %q; retry with one exact id", input.DataSource), Matches: matches}, nil
		}

		slug := matches[0]
		output := Output{DataSourceID: slug.ID, DataSourceName: slug.Name}

		var errs []string

		if info, err := clients.GetDataSourceInfo(ctx, DocsClient, slug.ID); err != nil {
			errs = append(errs, fmt.Sprintf("driver/connection-string info unavailable: %s", err.Error()))
		} else {
			output.DriverClassName = info.DriverClassName
			output.ConnectionStringFormat = info.ConnectionStringFormat
			output.MarketplaceURL = info.MarketplaceURL
			output.Prerequisites = info.Prerequisites
		}

		if methods, err := clients.GetConnectionProperties(ctx, DocsClient, slug.ID); err != nil {
			errs = append(errs, fmt.Sprintf("connection properties unavailable: %s", err.Error()))
		} else {
			output.AuthMethods = methods
		}

		output.Success = output.DriverClassName != "" || len(output.AuthMethods) > 0
		output.Error = strings.Join(errs, "; ")
		return output, nil
	}
}

// matchDataSources resolves a free-text query against the known data source list:
// exact id/name match wins outright; otherwise falls back to substring matches.
func matchDataSources(sources []clients.DataSourceSlug, query string) []clients.DataSourceSlug {
	q := strings.ToLower(strings.TrimSpace(query))

	var exact, partial []clients.DataSourceSlug
	for _, s := range sources {
		id, name := strings.ToLower(s.ID), strings.ToLower(s.Name)
		switch {
		case id == q || name == q:
			exact = append(exact, s)
		case strings.Contains(id, q) || strings.Contains(name, q):
			partial = append(partial, s)
		}
	}

	if len(exact) > 0 {
		return exact
	}
	return partial
}
