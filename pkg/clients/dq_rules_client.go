package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// CreateDQRuleRequest is the request body for
// POST /rest/dq/internal/v1/monitoring/monitor. The field shapes mirror the
// DQ `Monitor` DTO so the wire format matches what the DQ service expects.
type CreateDQRuleRequest struct {
	JobName      string   `json:"jobName"`
	MonitorName  string   `json:"monitorName"`
	MonitorType  string   `json:"monitorType"`
	MonitorValue string   `json:"monitorValue"`
	FilterQuery  string   `json:"filterQuery,omitempty"`
	ColumnName   string   `json:"columnName,omitempty"`
	Description  string   `json:"description,omitempty"`
	Dimensions   []string `json:"dimensions,omitempty"`
	Tolerance    int      `json:"tolerance"`
	IsActive     int      `json:"isActive"`
	IsSuppressed bool     `json:"isSuppressed"`
	TemplateID   string   `json:"templateId,omitempty"`
}

// CreateDQRuleResponse is the response from
// POST /rest/dq/internal/v1/monitoring/monitor.
type CreateDQRuleResponse struct {
	JobName     string `json:"jobName"`
	MonitorName string `json:"monitorName"`
}

// CreateDQRule creates a data quality rule (monitor) on an existing DQ job.
func CreateDQRule(ctx context.Context, client *http.Client, request CreateDQRuleRequest) (*CreateDQRuleResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("creating dq rule: marshaling request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/rest/dq/internal/v1/monitoring/monitor", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("creating dq rule: building request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("creating dq rule: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("creating dq rule: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusBadRequest:
			return nil, fmt.Errorf("creating dq rule: bad request (invalid rule definition): %s", string(respBody))
		case http.StatusForbidden:
			return nil, fmt.Errorf("creating dq rule: missing permission to create rules on this job: %s", string(respBody))
		case http.StatusNotFound:
			return nil, fmt.Errorf("creating dq rule: job or template not found: %s", string(respBody))
		case http.StatusUnprocessableEntity:
			return nil, fmt.Errorf("creating dq rule: rule creation not allowed for this job (e.g. dataset is not of type PUSHDOWN): %s", string(respBody))
		default:
			return nil, fmt.Errorf("creating dq rule: unexpected status %d: %s", resp.StatusCode, string(respBody))
		}
	}

	var result CreateDQRuleResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("creating dq rule: decoding response: %w", err)
	}

	return &result, nil
}

// dqDo executes a DQ API request against the Collibra client and returns the raw
// response body and status code. It marshals body (when non-nil) as JSON and only
// returns a non-nil error for transport/encoding failures — callers inspect the
// returned status code so they can map DQ error statuses to descriptive messages.
func dqDo(ctx context.Context, client *http.Client, method, path string, body any) ([]byte, int, error) {
	var bodyReader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, 0, fmt.Errorf("marshaling request: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, path, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("building request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response: %w", err)
	}
	return respBody, resp.StatusCode, nil
}

// DQRule is the full monitor (rule) definition returned by GetDQRule. Its shape
// mirrors the DQ `Monitor` DTO.
type DQRule struct {
	JobName      string   `json:"jobName"`
	MonitorName  string   `json:"monitorName"`
	MonitorType  string   `json:"monitorType"`
	MonitorValue string   `json:"monitorValue"`
	FilterQuery  string   `json:"filterQuery,omitempty"`
	ColumnName   string   `json:"columnName,omitempty"`
	Description  string   `json:"description,omitempty"`
	Dimensions   []string `json:"dimensions,omitempty"`
	Tolerance    int      `json:"tolerance"`
	IsActive     int      `json:"isActive"`
	IsSuppressed bool     `json:"isSuppressed"`
	TemplateID   string   `json:"templateId,omitempty"`
}

// GetDQRule fetches a single rule (monitor) on a job by name —
// GET /rest/dq/internal/v1/jobs/{jobName}/monitors/rules/{monitorName}.
func GetDQRule(ctx context.Context, client *http.Client, jobName, monitorName string) (*DQRule, error) {
	path := "/rest/dq/internal/v1/jobs/" + url.PathEscape(jobName) + "/monitors/rules/" + url.PathEscape(monitorName)
	respBody, status, err := dqDo(ctx, client, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("getting dq rule: %w", err)
	}
	if status != http.StatusOK {
		if status == http.StatusNotFound {
			return nil, fmt.Errorf("getting dq rule: rule %q not found on job %q: %s", monitorName, jobName, string(respBody))
		}
		return nil, fmt.Errorf("getting dq rule: unexpected status %d: %s", status, string(respBody))
	}
	var result DQRule
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("getting dq rule: decoding response: %w", err)
	}
	return &result, nil
}

// EditDQRuleRequest is the request body for PUT /rest/dq/internal/v1/monitors/rules.
// Its shape mirrors the DQ `Monitor` DTO (the same body as create).
type EditDQRuleRequest struct {
	JobName      string   `json:"jobName"`
	MonitorName  string   `json:"monitorName"`
	MonitorType  string   `json:"monitorType"`
	MonitorValue string   `json:"monitorValue"`
	FilterQuery  string   `json:"filterQuery,omitempty"`
	ColumnName   string   `json:"columnName,omitempty"`
	Description  string   `json:"description,omitempty"`
	Dimensions   []string `json:"dimensions,omitempty"`
	Tolerance    int      `json:"tolerance"`
	IsActive     int      `json:"isActive"`
	IsSuppressed bool     `json:"isSuppressed"`
	TemplateID   string   `json:"templateId,omitempty"`
}

// EditDQRule updates an existing rule (monitor). newRuleName, when non-empty,
// renames the rule — PUT /rest/dq/internal/v1/monitors/rules?newRuleName=.
func EditDQRule(ctx context.Context, client *http.Client, request EditDQRuleRequest, newRuleName string) (*CreateDQRuleResponse, error) {
	path := "/rest/dq/internal/v1/monitors/rules"
	if newRuleName != "" {
		path += "?" + url.Values{"newRuleName": {newRuleName}}.Encode()
	}
	respBody, status, err := dqDo(ctx, client, http.MethodPut, path, request)
	if err != nil {
		return nil, fmt.Errorf("editing dq rule: %w", err)
	}
	if status != http.StatusOK {
		switch status {
		case http.StatusBadRequest:
			return nil, fmt.Errorf("editing dq rule: bad request (invalid rule definition): %s", string(respBody))
		case http.StatusForbidden:
			return nil, fmt.Errorf("editing dq rule: missing permission to edit rules on this job: %s", string(respBody))
		case http.StatusNotFound:
			return nil, fmt.Errorf("editing dq rule: rule %q not found on job %q: %s", request.MonitorName, request.JobName, string(respBody))
		case http.StatusUnprocessableEntity:
			return nil, fmt.Errorf("editing dq rule: rule edit not allowed for this job (e.g. dataset is not of type PUSHDOWN): %s", string(respBody))
		default:
			return nil, fmt.Errorf("editing dq rule: unexpected status %d: %s", status, string(respBody))
		}
	}
	var result CreateDQRuleResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("editing dq rule: decoding response: %w", err)
	}
	return &result, nil
}

// DeleteDQRule deletes a rule (monitor) on a job —
// DELETE /rest/dq/internal/v1/jobs/{jobName}/monitors/rules/{monitorName}.
func DeleteDQRule(ctx context.Context, client *http.Client, jobName, monitorName string) error {
	path := "/rest/dq/internal/v1/jobs/" + url.PathEscape(jobName) + "/monitors/rules/" + url.PathEscape(monitorName)
	respBody, status, err := dqDo(ctx, client, http.MethodDelete, path, nil)
	if err != nil {
		return fmt.Errorf("deleting dq rule: %w", err)
	}
	if status != http.StatusNoContent {
		switch status {
		case http.StatusForbidden:
			return fmt.Errorf("deleting dq rule: missing permission to delete rules on this job: %s", string(respBody))
		case http.StatusNotFound:
			return fmt.Errorf("deleting dq rule: rule %q not found on job %q: %s", monitorName, jobName, string(respBody))
		default:
			return fmt.Errorf("deleting dq rule: unexpected status %d: %s", status, string(respBody))
		}
	}
	return nil
}

// DQRuleResultEntry is one per-run result for a rule (a RuleDetailsEntry): the
// score, break counts and status for a single job run.
type DQRuleResultEntry struct {
	Exception       string  `json:"exception,omitempty"`
	RunDate         int64   `json:"runDate"`
	Score           int     `json:"score"`
	BreakMsg        string  `json:"breakMsg,omitempty"`
	RuleCondition   string  `json:"ruleCondition,omitempty"`
	TotalCount      float64 `json:"totalCount"`
	BreakingRecords float64 `json:"breakingRecords"`
	PassingRecords  float64 `json:"passingRecords"`
	RuleStatus      string  `json:"ruleStatus,omitempty"`
	PassFail        bool    `json:"passFail"`
	BreakingPerc    float64 `json:"breakingPerc"`
	PassingPerc     float64 `json:"passingPerc"`
}

// DQRuleResults is the paginated result set for a rule (a RuleDetails response):
// the rule's definition summary plus its per-run result entries.
type DQRuleResults struct {
	Dataset          string              `json:"dataset"`
	RuleName         string              `json:"ruleName"`
	RuleType         string              `json:"ruleType"`
	RuleValue        string              `json:"ruleValue"`
	RuleValueBuilder string              `json:"ruleValueBuilder,omitempty"`
	FilterQuery      string              `json:"filterQuery,omitempty"`
	Tolerance        int                 `json:"tolerance"`
	IsActive         int                 `json:"isActive"`
	Results          []DQRuleResultEntry `json:"results"`
	Total            int64               `json:"total"`
	Offset           int64               `json:"offset"`
	Limit            int64               `json:"limit"`
}

// GetDQRuleResults reads a rule's per-run results / breaking records —
// GET /rest/dq/internal/v1/monitoring/rules/{jobName}/{ruleName}. offset/limit
// paginate; sortOrder is "ASC" or "DESC" (defaults to DESC when empty).
func GetDQRuleResults(ctx context.Context, client *http.Client, jobName, ruleName string, offset, limit int, sortOrder string) (*DQRuleResults, error) {
	q := url.Values{
		"offset": {fmt.Sprintf("%d", offset)},
		"limit":  {fmt.Sprintf("%d", limit)},
	}
	if sortOrder != "" {
		q.Set("sortOrder", sortOrder)
	}
	path := "/rest/dq/internal/v1/monitoring/rules/" + url.PathEscape(jobName) + "/" + url.PathEscape(ruleName) + "?" + q.Encode()
	respBody, status, err := dqDo(ctx, client, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("getting dq rule results: %w", err)
	}
	if status != http.StatusOK {
		if status == http.StatusNotFound {
			return nil, fmt.Errorf("getting dq rule results: rule %q not found on job %q: %s", ruleName, jobName, string(respBody))
		}
		return nil, fmt.Errorf("getting dq rule results: unexpected status %d: %s", status, string(respBody))
	}
	var result DQRuleResults
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("getting dq rule results: decoding response: %w", err)
	}
	return &result, nil
}

// PreviewRuleRequest is the request body for both rule validation and SQL preview
// (POST /rest/dq/internal/v1/rules/validate and .../rules/previewRule). It mirrors
// the DQ `PreviewRuleRequest` DTO.
type PreviewRuleRequest struct {
	EdgeSiteID   string `json:"edgeSiteId"`
	ConnectionID string `json:"connectionId"`
	SchemaName   string `json:"schemaName"`
	JobName      string `json:"jobName"`
	PreviewRule  string `json:"previewRule"`
	FilterQuery  string `json:"filterQuery,omitempty"`
	RowLimit     int    `json:"rowLimit"`
}

// ValidateDQRuleResponse is the validation verdict from
// POST /rest/dq/internal/v1/rules/validate. A rule that fails validation still
// returns HTTP 200 with IsValid=false and a Message explaining why.
type ValidateDQRuleResponse struct {
	IsValid bool   `json:"isValid"`
	Message string `json:"message"`
}

// ValidateDQRule checks that a rule's SQL/definition is valid before it is saved
// or run — POST /rest/dq/internal/v1/rules/validate.
func ValidateDQRule(ctx context.Context, client *http.Client, request PreviewRuleRequest) (*ValidateDQRuleResponse, error) {
	respBody, status, err := dqDo(ctx, client, http.MethodPost, "/rest/dq/internal/v1/rules/validate", request)
	if err != nil {
		return nil, fmt.Errorf("validating dq rule: %w", err)
	}
	if status != http.StatusOK {
		switch status {
		case http.StatusForbidden:
			return nil, fmt.Errorf("validating dq rule: missing permission: %s", string(respBody))
		case http.StatusNotFound:
			return nil, fmt.Errorf("validating dq rule: connection or job not found: %s", string(respBody))
		default:
			return nil, fmt.Errorf("validating dq rule: unexpected status %d: %s", status, string(respBody))
		}
	}
	var result ValidateDQRuleResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("validating dq rule: decoding response: %w", err)
	}
	return &result, nil
}
