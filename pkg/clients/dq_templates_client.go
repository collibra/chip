package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// DQRuleTemplate is a data quality rule template (a "RuleTemplate") — a
// parameterized SQL pattern (with a {{column}} placeholder) that can be deployed
// as concrete rules across many columns/jobs. IsSystem marks the built-in
// (system) templates; custom templates are user-defined. Field names follow the
// public /rest/dq/1.0/ruleTemplates API.
type DQRuleTemplate struct {
	ID                   string   `json:"id"`
	Name                 string   `json:"ruleTemplateName"`
	Description          string   `json:"description,omitempty"`
	SQL                  string   `json:"sql,omitempty"`
	Dialect              string   `json:"dialect,omitempty"`
	Dimensions           []string `json:"dimensions,omitempty"`
	Tolerance            *int     `json:"tolerance,omitempty"`
	IsSystem             bool     `json:"isSystem"`
	DeployedRuleCount    int64    `json:"deployedRuleCount"`
	BusinessRuleAssetIDs []string `json:"businessRuleAssetIds,omitempty"`
}

// dqRuleTemplateListResponse is the paginated list envelope
// (RuleTemplatePaginated).
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
// templates. IsSystem is a tri-state: nil = all, true = system (built-in) only,
// false = custom only.
type ListDQRuleTemplatesParams struct {
	Name      string
	Dimension string
	CreatedBy string
	IsSystem  *bool
	SortBy    string
	SortDir   string
	Offset    int
	Limit     int
}

// ListDQRuleTemplates lists rule templates (system and custom) —
// GET /rest/dq/1.0/ruleTemplates.
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
	if params.IsSystem != nil {
		q.Set("isSystem", strconv.FormatBool(*params.IsSystem))
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

	path := "/rest/dq/1.0/ruleTemplates"
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

// GetDQRuleTemplate fetches a single rule template by name —
// GET /rest/dq/1.0/ruleTemplates/{ruleTemplateName}.
func GetDQRuleTemplate(ctx context.Context, client *http.Client, ruleTemplateName string) (*DQRuleTemplate, error) {
	path := "/rest/dq/1.0/ruleTemplates/" + url.PathEscape(ruleTemplateName)
	respBody, status, err := dqDo(ctx, client, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("getting dq rule template: %w", err)
	}
	if status != http.StatusOK {
		switch status {
		case http.StatusForbidden:
			return nil, fmt.Errorf("getting dq rule template: missing permission to view templates: %s", string(respBody))
		case http.StatusNotFound:
			return nil, fmt.Errorf("getting dq rule template: %w: %q: %s", ErrDQRuleTemplateNotFound, ruleTemplateName, string(respBody))
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
// ColumnName and ConnectionName are optional.
type DQTemplateDeployTarget struct {
	JobName        string `json:"jobName"`
	ColumnName     string `json:"columnName,omitempty"`
	ConnectionName string `json:"connectionName,omitempty"`
}

// dqTemplateDeployRequest is the request body for the deploy endpoint.
type dqTemplateDeployRequest struct {
	Targets []DQTemplateDeployTarget `json:"targets"`
}

// DQTemplateDeployOutcome is the per-target result of a deploy. Status reports
// whether the target was deployed or skipped; Reason explains skips/failures.
type DQTemplateDeployOutcome struct {
	JobName          string `json:"jobName"`
	ColumnName       string `json:"columnName,omitempty"`
	DeployedRuleName string `json:"deployedRuleName,omitempty"`
	Status           string `json:"status"`
	Reason           string `json:"reason,omitempty"`
}

// DQTemplateDeployResult is the deploy response (RuleTemplateDeployResult): a
// per-target outcome list. Deploy is partial-success — individual targets may be
// deployed or skipped independently.
type DQTemplateDeployResult struct {
	Results []DQTemplateDeployOutcome `json:"results"`
}

// DeployDQRuleTemplate instantiates a template as concrete rules on the given
// targets, using dialect-specific SQL resolved server-side —
// POST /rest/dq/1.0/ruleTemplates/{ruleTemplateName}/deploy. The deploy is
// partial-success: it returns HTTP 200 with a per-target outcome list even when
// some targets are skipped.
func DeployDQRuleTemplate(ctx context.Context, client *http.Client, ruleTemplateName string, targets []DQTemplateDeployTarget) (*DQTemplateDeployResult, error) {
	path := "/rest/dq/1.0/ruleTemplates/" + url.PathEscape(ruleTemplateName) + "/deploy"
	respBody, status, err := dqDo(ctx, client, http.MethodPost, path, dqTemplateDeployRequest{Targets: targets})
	if err != nil {
		return nil, fmt.Errorf("deploying dq rule template: %w", err)
	}
	if status != http.StatusOK {
		switch status {
		case http.StatusBadRequest:
			return nil, fmt.Errorf("deploying dq rule template: bad request (e.g. invalid targets or incompatible template): %s", string(respBody))
		case http.StatusForbidden:
			return nil, fmt.Errorf("deploying dq rule template: missing permission to deploy templates: %s", string(respBody))
		case http.StatusNotFound:
			return nil, fmt.Errorf("deploying dq rule template: template %q not found: %s", ruleTemplateName, string(respBody))
		default:
			return nil, fmt.Errorf("deploying dq rule template: unexpected status %d: %s", status, string(respBody))
		}
	}
	var result DQTemplateDeployResult
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("deploying dq rule template: decoding response: %w", err)
	}
	return &result, nil
}
