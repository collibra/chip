// Package search_catalog_columns implements the search_catalog_columns MCP tool:
// it finds catalog Column assets by metadata the public REST search cannot filter
// on (attribute values, assigned roles, relations to other assets), via the DGC
// Knowledge Graph GraphQL API. Multiple filters are combined with AND.
package search_catalog_columns

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
)

// OutputStatus is the overall outcome of a search_catalog_columns call.
type OutputStatus string

const (
	// StatusSuccess means the search ran.
	StatusSuccess OutputStatus = "success"
	// StatusValidationError means the inputs failed validation before any call.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the search failed (e.g. KG endpoint unavailable).
	StatusError OutputStatus = "error"
)

const defaultLimit = 25

// Input is the tool's typed input. All filters are optional but at least one
// must be set; they are combined with AND. The search is always scoped to Column
// assets.
type Input struct {
	Domain          string `json:"domain,omitempty" jsonschema:"Optional. Exact domain name the column lives in."`
	Community       string `json:"community,omitempty" jsonschema:"Optional. Exact community name (the column's domain's parent)."`
	Description     string `json:"description,omitempty" jsonschema:"Optional. Substring to match in the column's Description attribute."`
	DataType        string `json:"dataType,omitempty" jsonschema:"Optional. Substring to match in the column's Data Type attribute."`
	DataStewardRole string `json:"dataStewardRole,omitempty" jsonschema:"Optional. Match columns that have a responsibility with this role name assigned (e.g. 'Data Steward'). Matches by role, not by a specific person."`
	BusinessTerm    string `json:"businessTerm,omitempty" jsonschema:"Optional. Exact display name of a Business Term the column represents."`
	BusinessRule    string `json:"businessRule,omitempty" jsonschema:"Optional. Exact display name of a Business Rule that governs the column."`
	DataElement     string `json:"dataElement,omitempty" jsonschema:"Optional. Exact display name of a Data Element the column targets (technical lineage)."`
	DataAttribute   string `json:"dataAttribute,omitempty" jsonschema:"Optional. Exact display name of a Data Attribute that represents the column."`
	Limit           int    `json:"limit,omitempty" jsonschema:"Optional. Max columns to return. Defaults to 25."`
	Offset          int    `json:"offset,omitempty" jsonschema:"Optional. Pagination offset (min 0). Defaults to 0."`
}

// ColumnResult is one matching column.
type ColumnResult struct {
	ID          string `json:"id" jsonschema:"Column asset UUID."`
	FullName    string `json:"fullName" jsonschema:"Fully-qualified column name."`
	DisplayName string `json:"displayName,omitempty" jsonschema:"Column display name."`
	Domain      string `json:"domain,omitempty" jsonschema:"The column's domain."`
}

// Output is the typed response.
type Output struct {
	Status  OutputStatus   `json:"status" jsonschema:"'success' when the search ran; 'validation_error' for bad inputs; 'error' for downstream failures (incl. KG endpoint unavailable)."`
	Message string         `json:"message" jsonschema:"Human-readable summary."`
	Columns []ColumnResult `json:"columns,omitempty" jsonschema:"Matching columns."`
	Count   int            `json:"count" jsonschema:"Number of columns returned."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "search_catalog_columns",
		Title: "Search Catalog Columns by Metadata",
		Description: "Find catalog Column assets by metadata that keyword search cannot filter on — Description / Data Type " +
			"(attribute values), a Data Steward role, or relations to a Business Term, Business Rule, Data Element or Data Attribute " +
			"(by name). Filters are combined with AND. Useful for picking out columns to attach data-quality rules (checks; Collibra calls them 'monitors') to at scale. " +
			"Requires the DGC Knowledge Graph API (Collibra's metadata graph query service) to be enabled on the instance; classification-tag filtering is not supported. " +
			"A broad lone substring filter (e.g. description alone) can exceed the Knowledge Graph query timeout — combine with a domain or another selective filter.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.Offset < 0 {
			return Output{Status: StatusValidationError, Message: "offset must be >= 0."}, nil
		}
		params := clients.CatalogColumnSearchParams{
			Domain:        strings.TrimSpace(input.Domain),
			Community:     strings.TrimSpace(input.Community),
			Description:   strings.TrimSpace(input.Description),
			DataType:      strings.TrimSpace(input.DataType),
			StewardRole:   strings.TrimSpace(input.DataStewardRole),
			BusinessTerm:  strings.TrimSpace(input.BusinessTerm),
			BusinessRule:  strings.TrimSpace(input.BusinessRule),
			DataElement:   strings.TrimSpace(input.DataElement),
			DataAttribute: strings.TrimSpace(input.DataAttribute),
			Limit:         input.Limit,
			Offset:        input.Offset,
		}
		if !hasAnyFilter(params) {
			return Output{Status: StatusValidationError, Message: "Provide at least one filter (domain, community, description, dataType, dataStewardRole, businessTerm, businessRule, dataElement, or dataAttribute)."}, nil
		}
		if params.Limit == 0 {
			params.Limit = defaultLimit
		}

		cols, err := clients.SearchCatalogColumns(ctx, collibraClient, params)
		if err != nil {
			return Output{Status: StatusError, Message: fmt.Sprintf("Could not search columns: %v", err)}, nil
		}

		results := make([]ColumnResult, 0, len(cols))
		for _, c := range cols {
			results = append(results, ColumnResult{
				ID:          c.ID,
				FullName:    c.FullName,
				DisplayName: c.DisplayName,
				Domain:      c.Domain.Name,
			})
		}

		return Output{
			Status:  StatusSuccess,
			Message: fmt.Sprintf("Found %d matching column(s).", len(results)),
			Columns: results,
			Count:   len(results),
		}, nil
	}
}

func hasAnyFilter(p clients.CatalogColumnSearchParams) bool {
	return p.Domain != "" || p.Community != "" || p.Description != "" || p.DataType != "" ||
		p.StewardRole != "" || p.BusinessTerm != "" || p.BusinessRule != "" ||
		p.DataElement != "" || p.DataAttribute != ""
}
