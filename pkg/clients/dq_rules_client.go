package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
