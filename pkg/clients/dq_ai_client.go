package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Text2SQLRequest is the request body for POST /rest/dq/internal/v1/ai/text2sql.
// It turns a natural-language rule description into rule SQL. The table is
// identified by jobName; columns give the relevant column context. This endpoint
// is internal-only (not part of the public DQ API) and is not in the OAS spec.
type Text2SQLRequest struct {
	EdgeSiteID   string   `json:"edgeSiteId"`
	ConnectionID string   `json:"connectionId"`
	Query        string   `json:"query"`
	JobName      string   `json:"jobName"`
	Columns      []string `json:"columns"`
}

// Text2SQLResponse is the response: a single generated SQL string. There is no
// separate filter/WHERE-predicate field — the engine returns one query.
type Text2SQLResponse struct {
	SQLQuery string `json:"sqlQuery"`
}

// GenerateDQRuleSQL turns a natural-language description into rule SQL —
// POST /rest/dq/internal/v1/ai/text2sql.
func GenerateDQRuleSQL(ctx context.Context, client *http.Client, request Text2SQLRequest) (*Text2SQLResponse, error) {
	respBody, status, err := dqDo(ctx, client, http.MethodPost, "/rest/dq/internal/v1/ai/text2sql", request)
	if err != nil {
		return nil, fmt.Errorf("generating dq rule sql: %w", err)
	}
	if status != http.StatusOK {
		switch status {
		case http.StatusBadRequest:
			return nil, fmt.Errorf("generating dq rule sql: bad request (the description could not be turned into valid SQL): %s", string(respBody))
		case http.StatusForbidden:
			return nil, fmt.Errorf("generating dq rule sql: missing permission to use DQ AI: %s", string(respBody))
		default:
			return nil, fmt.Errorf("generating dq rule sql: unexpected status %d: %s", status, string(respBody))
		}
	}
	var result Text2SQLResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("generating dq rule sql: decoding response: %w", err)
	}
	return &result, nil
}
