package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// DQRuleTemplate is a data quality rule template (a "RuleTemplateResponse") — a
// parameterized SQL pattern (with a {{column}} placeholder) that can be deployed
// as concrete rules across many columns/jobs. Ootb marks the built-in templates;
// custom templates are user-defined.
type DQRuleTemplate struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Description     string   `json:"description,omitempty"`
	SQLQuery        string   `json:"sqlQuery,omitempty"`
	SourceDialect   string   `json:"sourceDialect,omitempty"`
	Dimensions      []string `json:"dimensions,omitempty"`
	Tolerance       *int     `json:"tolerance,omitempty"`
	Ootb            bool     `json:"ootb"`
	DeploymentCount int64    `json:"deploymentCount"`
}

// dqRuleTemplateListResponse is the paginated list envelope (PagedResponse).
type dqRuleTemplateListResponse struct {
	Results []DQRuleTemplate `json:"results"`
	Total   int64            `json:"total"`
	Offset  int64            `json:"offset"`
	Limit   int64            `json:"limit"`
}

// DQRuleTemplateList is the returned page of templates plus pagination metadata.
type DQRuleTemplateList struct {
	Results []DQRuleTemplate
	Total   int64
	Offset  int64
	Limit   int64
}

// ListDQRuleTemplatesParams are the optional filters/pagination for listing
// templates. Ootb is a tri-state: nil = all, true = OOTB only, false = custom only.
type ListDQRuleTemplatesParams struct {
	Name      string
	Dimension string
	CreatedBy string
	Ootb      *bool
	SortBy    string
	SortDir   string
	Offset    int
	Limit     int
}

// ListDQRuleTemplates lists rule templates (OOTB and custom) —
// GET /rest/dq/internal/v1/rules/templates.
func ListDQRuleTemplates(ctx context.Context, client *http.Client, params ListDQRuleTemplatesParams) (*DQRuleTemplateList, error) {
	q := url.Values{}
	if params.Name != "" {
		q.Set("name", params.Name)
	}
	if params.Dimension != "" {
		q.Set("dimension", params.Dimension)
	}
	if params.CreatedBy != "" {
		q.Set("createdBy", params.CreatedBy)
	}
	if params.Ootb != nil {
		q.Set("isOotb", strconv.FormatBool(*params.Ootb))
	}
	if params.SortBy != "" {
		q.Set("sortBy", params.SortBy)
	}
	if params.SortDir != "" {
		q.Set("sortDir", params.SortDir)
	}
	q.Set("offset", strconv.Itoa(params.Offset))
	if params.Limit > 0 {
		q.Set("limit", strconv.Itoa(params.Limit))
	}

	path := "/rest/dq/internal/v1/rules/templates"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	respBody, status, err := dqDo(ctx, client, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("listing dq rule templates: %w", err)
	}
	if status != http.StatusOK {
		if status == http.StatusForbidden {
			return nil, fmt.Errorf("listing dq rule templates: missing permission to view templates: %s", string(respBody))
		}
		return nil, fmt.Errorf("listing dq rule templates: unexpected status %d: %s", status, string(respBody))
	}
	var resp dqRuleTemplateListResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("listing dq rule templates: decoding response: %w", err)
	}
	return &DQRuleTemplateList{Results: resp.Results, Total: resp.Total, Offset: resp.Offset, Limit: resp.Limit}, nil
}

// GetDQRuleTemplate fetches a single rule template by id —
// GET /rest/dq/internal/v1/rules/templates/{templateId}.
func GetDQRuleTemplate(ctx context.Context, client *http.Client, templateID string) (*DQRuleTemplate, error) {
	path := "/rest/dq/internal/v1/rules/templates/" + url.PathEscape(templateID)
	respBody, status, err := dqDo(ctx, client, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("getting dq rule template: %w", err)
	}
	if status != http.StatusOK {
		switch status {
		case http.StatusForbidden:
			return nil, fmt.Errorf("getting dq rule template: missing permission to view templates: %s", string(respBody))
		case http.StatusNotFound:
			return nil, fmt.Errorf("getting dq rule template: template %q not found: %s", templateID, string(respBody))
		default:
			return nil, fmt.Errorf("getting dq rule template: unexpected status %d: %s", status, string(respBody))
		}
	}
	var tmpl DQRuleTemplate
	if err := json.Unmarshal(respBody, &tmpl); err != nil {
		return nil, fmt.Errorf("getting dq rule template: decoding response: %w", err)
	}
	return &tmpl, nil
}

// DQTemplateDeployTarget is one deployment target: the job and, for a
// column-level template, the column substituted for the {{column}} placeholder.
// ColumnName is optional for table-level templates.
type DQTemplateDeployTarget struct {
	JobName    string `json:"jobName"`
	ColumnName string `json:"columnName,omitempty"`
}

// dqTemplateDeployRequest is the request body for the deploy endpoint.
type dqTemplateDeployRequest struct {
	Targets []DQTemplateDeployTarget `json:"targets"`
}

// DeployDQRuleTemplate instantiates a template as concrete rules on the given
// targets, using dialect-specific SQL resolved server-side —
// POST /rest/dq/internal/v1/rules/templates/{templateId}/deploy. Each deployed
// rule is named {templateName}_{columnName} by the server. Returns no body on
// success (HTTP 204).
func DeployDQRuleTemplate(ctx context.Context, client *http.Client, templateID string, targets []DQTemplateDeployTarget) error {
	path := "/rest/dq/internal/v1/rules/templates/" + url.PathEscape(templateID) + "/deploy"
	respBody, status, err := dqDo(ctx, client, http.MethodPost, path, dqTemplateDeployRequest{Targets: targets})
	if err != nil {
		return fmt.Errorf("deploying dq rule template: %w", err)
	}
	if status != http.StatusNoContent {
		switch status {
		case http.StatusBadRequest:
			return fmt.Errorf("deploying dq rule template: bad request (e.g. incompatible template/column or invalid target): %s", string(respBody))
		case http.StatusForbidden:
			return fmt.Errorf("deploying dq rule template: missing permission to deploy templates: %s", string(respBody))
		case http.StatusNotFound:
			return fmt.Errorf("deploying dq rule template: template %q or a target job not found: %s", templateID, string(respBody))
		default:
			return fmt.Errorf("deploying dq rule template: unexpected status %d: %s", status, string(respBody))
		}
	}
	return nil
}
