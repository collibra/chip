// Package create_dq_rule_template implements the
// create_data_quality_rule_template MCP tool: it adds a reusable, parameterized
// rule template to the data quality template library.
package create_dq_rule_template

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the overall outcome of a create_data_quality_rule_template call.
type OutputStatus string

const (
	// StatusSuccess means the template was created.
	StatusSuccess OutputStatus = "success"
	// StatusPreview means confirm was not set: the tool echoed the template it
	// would create and wrote nothing.
	StatusPreview OutputStatus = "preview"
	// StatusValidationError means the inputs failed validation before any write.
	StatusValidationError OutputStatus = "validation_error"
	// StatusError means the create failed due to a downstream DQ error.
	StatusError OutputStatus = "error"
)

// Limits enforced by the public rule template API, checked here so a preventable
// mistake never reaches the service as a raw 400.
const (
	maxNameLength        = 255
	maxDescriptionLength = 1000
	maxSQLLength         = 10000
	maxDialectLength     = 255
	maxDimensions        = 20
	maxBusinessRuleLinks = 100
)

// Input is the tool's typed input.
type Input struct {
	Name              string   `json:"name" jsonschema:"Required. Unique name for the new rule template, up to 255 characters, e.g. 'Row Count Range'. This name is the template's key: every other rule template tool addresses it by name, and creating a template whose name is already taken fails."`
	SQL               string   `json:"sql" jsonschema:"Required. The parameterized SQL query defining the check, up to 10000 characters. Use a {{column}} placeholder where the column being checked should be substituted at deploy time, e.g. 'select * from @dataset where {{column}} is null'. The data quality service must be able to translate it to the dialects of the jobs it is deployed to."`
	Dialect           string   `json:"dialect" jsonschema:"Required. The SQL dialect the query is authored in, e.g. 'snowflake', 'postgres', 'bigquery'. Not a fixed list in the API: the data quality service validates the value and rejects one it does not support."`
	Dimensions        []string `json:"dimensions" jsonschema:"Required, at least one and at most 20. Data quality dimensions the template's rules contribute to, e.g. ['Completeness'] or ['Validity','Accuracy']. Required by the data quality API even though it reads as optional in some documentation."`
	Description       string   `json:"description" jsonschema:"Required, up to 1000 characters. Human-readable explanation of what the template checks and when to use it. Required by the data quality API even though it reads as optional in some documentation."`
	BusinessRuleLinks []string `json:"businessRuleLinks,omitempty" jsonschema:"Optional, up to 100. Business Rule assets in the Collibra catalog that this template implements, each given either as the asset's exact name or as its UUID. Names are resolved to UUIDs before the write; a name matching no asset, or several, is reported as a validation error rather than guessed."`
	Confirm           bool     `json:"confirm,omitempty" jsonschema:"Safety checkpoint. false (default) returns a PREVIEW of the exact template that would be created and writes NOTHING, so it can be reviewed with the user. Set true to actually create the template after the user has approved."`
}

// TemplateDefinition is the full set of fields that will be, or were, written.
// Everything the tool sends appears here so a preview cannot hide a field.
type TemplateDefinition struct {
	Name                 string   `json:"name" jsonschema:"The template's name."`
	Description          string   `json:"description" jsonschema:"The template's description."`
	SQL                  string   `json:"sql" jsonschema:"The parameterized SQL the template runs."`
	Dialect              string   `json:"dialect" jsonschema:"The SQL dialect the query is authored in."`
	Dimensions           []string `json:"dimensions" jsonschema:"Data quality dimensions the template's rules contribute to."`
	BusinessRuleAssetIDs []string `json:"businessRuleAssetIds,omitempty" jsonschema:"UUIDs of the linked Business Rule assets, after resolving any names supplied in businessRuleLinks."`
}

// CreatedTemplate is the template as the service stored it.
type CreatedTemplate struct {
	ID                   string   `json:"id" jsonschema:"Server-assigned identifier of the created template."`
	Name                 string   `json:"name"`
	Description          string   `json:"description,omitempty"`
	SQL                  string   `json:"sql,omitempty"`
	Dialect              string   `json:"dialect,omitempty"`
	Dimensions           []string `json:"dimensions,omitempty"`
	BusinessRuleAssetIDs []string `json:"businessRuleAssetIds,omitempty"`
	IsSystem             bool     `json:"isSystem" jsonschema:"Whether the template is system-defined (out-of-the-box). Always false for a template created here."`
}

// Output is the typed response.
type Output struct {
	Status   OutputStatus        `json:"status" jsonschema:"'preview' when confirm was not set (nothing was created — review and call again with confirm=true); 'success' when the template was created; 'validation_error' for bad inputs; 'error' for downstream DQ failures."`
	Message  string              `json:"message" jsonschema:"Human-readable summary, or the reason the template could not be created."`
	Preview  *TemplateDefinition `json:"preview,omitempty" jsonschema:"The exact template that would be created, returned when confirm=false. Nothing was written."`
	Template *CreatedTemplate    `json:"template,omitempty" jsonschema:"The created template including its assigned id, on success."`
	Guidance string              `json:"guidance,omitempty" jsonschema:"On preview/validation_error/error, what to do next."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "create_data_quality_rule_template",
		Title: "Create Data Quality Rule Template",
		Description: "Create a reusable data quality rule template — a parameterized SQL pattern, with a {{column}} placeholder, that can later be deployed as concrete rules " +
			"(single data-quality checks on a table's data; Collibra calls them 'monitors') across many columns and jobs (a job, also called a 'dataset', is a saved check on ONE database table). " +
			"Creating a template does not check any data by itself: it adds an entry to the template library, which deploy_data_quality_rule_template then instantiates against real jobs. " +
			"Use this to add a new reusable check to the library; to change one that already exists use update_data_quality_rule_template, and to write a one-off check on a single job use create_data_quality_rule instead. " +
			"name must be unique across the library — creating a template whose name is already taken fails, so check with list_data_quality_rule_templates first if unsure. " +
			"sql, dialect, dimensions and description are all required by the data quality service. dialect is the SQL dialect the query is authored in (e.g. 'snowflake'); the service rejects a dialect it cannot translate. " +
			"businessRuleLinks optionally ties the template to Business Rule assets in the catalog, given by exact name or UUID — names are resolved before the write and an ambiguous name is reported rather than guessed. " +
			"Built around a confirm checkpoint: confirm=false (default) returns a PREVIEW of the exact template that would be created and writes nothing — review it with the user; confirm=true creates it. " +
			"Returns the created template including its server-assigned id. Requires permission to manage rule templates. " +
			"Example user requests: \"Add a reusable null-check template\"; \"Create a rule template for row count ranges in Snowflake\"; \"Make a template that enforces our email format rule\".",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: chip.Ptr(false), IdempotentHint: false, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		definition, out := validate(input)
		if out != nil {
			return *out, nil
		}

		ids, problems, err := clients.ResolveBusinessRuleAssetRefs(ctx, collibraClient, input.BusinessRuleLinks)
		if err != nil {
			return Output{
				Status:   StatusError,
				Message:  fmt.Sprintf("Could not resolve the businessRuleLinks: %v", err),
				Guidance: "The Business Rule asset lookup failed. Retry, or pass asset UUIDs instead of names to skip the lookup.",
			}, nil
		}
		if len(problems) > 0 {
			return Output{
				Status:   StatusValidationError,
				Message:  "Could not resolve every entry in businessRuleLinks: " + strings.Join(problems, "; ") + ".",
				Guidance: "Fix or remove the listed businessRuleLinks entries and call again. Nothing was created.",
			}, nil
		}
		definition.BusinessRuleAssetIDs = ids

		// Confirm checkpoint: without confirm, echo the full template and write
		// nothing.
		if !input.Confirm {
			return Output{
				Status:   StatusPreview,
				Message:  fmt.Sprintf("Ready to create rule template %q. Nothing has been created yet.", definition.Name),
				Preview:  definition,
				Guidance: "Review every field above with the user, then call again with confirm=true to create the template.",
			}, nil
		}

		created, err := clients.CreateDQRuleTemplate(ctx, collibraClient, clients.DQRuleTemplateWriteRequest{
			Name:                 definition.Name,
			Description:          definition.Description,
			SQL:                  definition.SQL,
			Dialect:              definition.Dialect,
			Dimensions:           definition.Dimensions,
			BusinessRuleAssetIDs: definition.BusinessRuleAssetIDs,
		})
		if err != nil {
			return createError(err, definition.Name), nil
		}

		return Output{
			Status:  StatusSuccess,
			Message: fmt.Sprintf("Created rule template %q (id %s).", created.Name, created.ID),
			Template: &CreatedTemplate{
				ID:                   created.ID,
				Name:                 created.Name,
				Description:          created.Description,
				SQL:                  created.SQL,
				Dialect:              created.Dialect,
				Dimensions:           created.Dimensions,
				BusinessRuleAssetIDs: created.BusinessRuleAssetIDs,
				IsSystem:             created.IsSystem,
			},
		}, nil
	}
}

// validate checks every documented constraint before any network call and
// returns the trimmed definition the tool would write.
func validate(input Input) (*TemplateDefinition, *Output) {
	name := strings.TrimSpace(input.Name)
	sql := strings.TrimSpace(input.SQL)
	dialect := strings.TrimSpace(input.Dialect)
	description := strings.TrimSpace(input.Description)

	switch {
	case name == "":
		return nil, invalid("name is required — the unique name of the rule template to create, e.g. 'Row Count Range'.")
	case len(name) > maxNameLength:
		return nil, invalid(fmt.Sprintf("name is %d characters; the maximum is %d.", len(name), maxNameLength))
	case sql == "":
		return nil, invalid("sql is required — the parameterized SQL defining the check, using a {{column}} placeholder for the column being checked.")
	case len(sql) > maxSQLLength:
		return nil, invalid(fmt.Sprintf("sql is %d characters; the maximum is %d.", len(sql), maxSQLLength))
	case dialect == "":
		return nil, invalid("dialect is required — the SQL dialect the query is authored in, e.g. 'snowflake' or 'postgres'.")
	case len(dialect) > maxDialectLength:
		return nil, invalid(fmt.Sprintf("dialect is %d characters; the maximum is %d.", len(dialect), maxDialectLength))
	case description == "":
		return nil, invalid("description is required by the data quality service — a short explanation of what the template checks.")
	case len(description) > maxDescriptionLength:
		return nil, invalid(fmt.Sprintf("description is %d characters; the maximum is %d.", len(description), maxDescriptionLength))
	case len(input.BusinessRuleLinks) > maxBusinessRuleLinks:
		return nil, invalid(fmt.Sprintf("businessRuleLinks has %d entries; the maximum is %d.", len(input.BusinessRuleLinks), maxBusinessRuleLinks))
	}

	dimensions, out := cleanDimensions(input.Dimensions)
	if out != nil {
		return nil, out
	}

	return &TemplateDefinition{
		Name:        name,
		Description: description,
		SQL:         sql,
		Dialect:     dialect,
		Dimensions:  dimensions,
	}, nil
}

// cleanDimensions trims the supplied dimensions and enforces the API's
// at-least-one, at-most-twenty rule.
func cleanDimensions(dimensions []string) ([]string, *Output) {
	cleaned := make([]string, 0, len(dimensions))
	for _, dimension := range dimensions {
		if trimmed := strings.TrimSpace(dimension); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	switch {
	case len(cleaned) == 0:
		return nil, invalid("dimensions is required by the data quality service — supply at least one data quality dimension, e.g. ['Completeness'].")
	case len(cleaned) > maxDimensions:
		return nil, invalid(fmt.Sprintf("dimensions has %d entries; the maximum is %d.", len(cleaned), maxDimensions))
	}
	return cleaned, nil
}

func invalid(message string) *Output {
	return &Output{
		Status:   StatusValidationError,
		Message:  message,
		Guidance: "Correct the input and call again. Nothing was created.",
	}
}

func createError(err error, name string) Output {
	if errors.Is(err, clients.ErrDQRuleTemplateNameTaken) {
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("A rule template named %q already exists, so it was not created: %v", name, err),
			Guidance: "Choose a different name, or change the existing template with update_data_quality_rule_template. Inspect it first with get_data_quality_rule_template.",
		}
	}
	return Output{
		Status:   StatusError,
		Message:  fmt.Sprintf("Could not create rule template %q: %v", name, err),
		Guidance: "Check the SQL, dialect and businessRuleLinks against the message above, then retry. Nothing was created.",
	}
}
