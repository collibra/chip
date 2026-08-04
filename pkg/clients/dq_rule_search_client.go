package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// DQMonitorFilter is one filter clause for the monitors dashboard query. Field
// is a MonitorFilterableField (e.g. JOB_NAME, COLUMN_NAME, MONITOR_NAME);
// Operator is a FilterOperator (e.g. EQUALS, CONTAINS).
type DQMonitorFilter struct {
	Field    string   `json:"field"`
	Operator string   `json:"operator"`
	Values   []string `json:"values"`
}

// dqDashboardRequest is the request body for the monitors dashboard query. When
// FilterFormula is empty the service ANDs all filters together.
type dqDashboardRequest struct {
	Filters   []DQMonitorFilter `json:"filters"`
	SortField string            `json:"sortField,omitempty"`
	SortOrder string            `json:"sortOrder,omitempty"`
	Offset    int               `json:"offset"`
	Limit     int               `json:"limit"`
}

// DQMonitorSummary is one rule (monitor) row from the dashboard query — enough to
// recognize an existing/duplicate rule on a column.
type DQMonitorSummary struct {
	MonitorName    string   `json:"monitorName"`
	MonitorType    string   `json:"monitorType"`
	MonitorStatus  string   `json:"monitorStatus"`
	ColumnName     string   `json:"columnName"`
	JobName        string   `json:"jobName"`
	ConnectionName string   `json:"connectionName"`
	SchemaName     string   `json:"schemaName"`
	TableName      string   `json:"tableName"`
	Dimensions     []string `json:"dimensions"`
	RuleQuery      string   `json:"ruleQuery"`
	FilterQuery    string   `json:"filterQuery"`
	Tolerance      string   `json:"tolerance"`
}

type dqDashboardResponse struct {
	Results []DQMonitorSummary `json:"results"`
	Total   int64              `json:"total"`
	Offset  int64              `json:"offset"`
	Limit   int64              `json:"limit"`
}

// DQMonitorSearchResult is the paginated set of matching rules.
type DQMonitorSearchResult struct {
	Results []DQMonitorSummary
	Total   int64
	Offset  int64
	Limit   int64
}

// FindDQRules searches existing rules (monitors) across jobs via the monitoring
// dashboard — POST /rest/dq/internal/v1/monitoring/monitors/dashboard. Pass
// filters (e.g. JOB_NAME + COLUMN_NAME EQUALS) to find rules on a specific
// column, for duplicate detection. Filters are ANDed together.
func FindDQRules(ctx context.Context, client *http.Client, filters []DQMonitorFilter, offset, limit int) (*DQMonitorSearchResult, error) {
	req := dqDashboardRequest{Filters: filters, Offset: offset, Limit: limit}
	respBody, status, err := dqDo(ctx, client, http.MethodPost, "/rest/dq/internal/v1/monitoring/monitors/dashboard", req)
	if err != nil {
		return nil, fmt.Errorf("finding dq rules: %w", err)
	}
	if status != http.StatusOK {
		if status == http.StatusBadRequest {
			return nil, fmt.Errorf("finding dq rules: bad request (invalid filter): %s", string(respBody))
		}
		return nil, fmt.Errorf("finding dq rules: unexpected status %d: %s", status, string(respBody))
	}
	var resp dqDashboardResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("finding dq rules: decoding response: %w", err)
	}
	return &DQMonitorSearchResult{Results: resp.Results, Total: resp.Total, Offset: resp.Offset, Limit: resp.Limit}, nil
}
