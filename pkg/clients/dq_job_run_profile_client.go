package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// DqColumnShape is one observed value shape for a column, using DQ's character-class
// encoding (`#` for a digit, `A` for a letter). Shapes are ordered by descending count.
type DqColumnShape struct {
	Pattern    string  `json:"pattern"`
	Count      int64   `json:"count"`
	Percentage float64 `json:"percentage"`
}

// DqColumnProfile is one column's profiling statistics from a job run
// (ColumnProfile in dq-v1-public-oas-spec.yaml). Min/Max/Mean/Median/Q1/Q3 are
// serialized as strings by the API and are null for columns the statistic does not
// apply to; TopShapes is null when shape analysis did not run.
type DqColumnProfile struct {
	ColumnName   string          `json:"columnName"`
	DefinedType  string          `json:"definedType,omitempty"`
	InferredType string          `json:"inferredType,omitempty"`
	ValueCount   int64           `json:"valueCount"`
	NullCount    int64           `json:"nullCount"`
	EmptyCount   int64           `json:"emptyCount"`
	UniqueCount  int64           `json:"uniqueCount"`
	Min          string          `json:"min,omitempty"`
	Max          string          `json:"max,omitempty"`
	Mean         string          `json:"mean,omitempty"`
	Median       string          `json:"median,omitempty"`
	Q1           string          `json:"q1,omitempty"`
	Q3           string          `json:"q3,omitempty"`
	TopShapes    []DqColumnShape `json:"topShapes,omitempty"`
}

// DqJobRunProfileResults is one page of a job run's per-column profiling results
// (JobRunProfileResults in dq-v1-public-oas-spec.yaml). Results are ordered
// ascending by column name. Total is only populated when the request asked for it.
type DqJobRunProfileResults struct {
	JobRunID string            `json:"jobRunId"`
	JobName  string            `json:"jobName,omitempty"`
	RunDate  *DqPublicRunDate  `json:"runDate,omitempty"`
	Offset   int64             `json:"offset"`
	Limit    int64             `json:"limit"`
	Total    *int64            `json:"total,omitempty"`
	Results  []DqColumnProfile `json:"results"`
}

// RunDateValue returns the run's date/timestamp, or "" when the page carries none.
func (r *DqJobRunProfileResults) RunDateValue() string {
	if r == nil || r.RunDate == nil {
		return ""
	}
	return r.RunDate.Value
}

// GetDqJobRunProfile reads one page of a job run's column-level profiling results via
// the PUBLIC GET /rest/dq/1.0/jobRuns/{jobRunId}/profile. limit/offset paginate (the
// API caps limit at 500); includeTotal asks the API for the full column count, which
// costs it an extra count query.
func GetDqJobRunProfile(ctx context.Context, collibraHttpClient *http.Client, jobRunID string, limit, offset int, includeTotal bool) (*DqJobRunProfileResults, int, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}
	if offset > 0 {
		params.Set("offset", strconv.Itoa(offset))
	}
	if includeTotal {
		params.Set("includeTotal", "true")
	}
	endpoint := "/rest/dq/1.0/jobRuns/" + url.PathEscape(jobRunID) + "/profile"
	if encoded := params.Encode(); encoded != "" {
		endpoint += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}
	body, code, err := executeRequestWithStatus(collibraHttpClient, req)
	if err != nil {
		return nil, code, err
	}
	var result DqJobRunProfileResults
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, code, fmt.Errorf("failed to parse job run profile response: %w", err)
	}
	return &result, code, nil
}
