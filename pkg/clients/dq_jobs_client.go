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

// RunDQJobRequest is the optional request body for
// POST /rest/dq/1.0/jobs/{jobName}/run. A nil request runs the job with the
// service defaults (current date/time, no backrun).
type RunDQJobRequest struct {
	RunDate    *DQRunDate `json:"runDate,omitempty"`
	RunDateEnd *DQRunDate `json:"runDateEnd,omitempty"`
	Backrun    *DQBackrun `json:"backrun,omitempty"`
}

// DQRunDate is the discriminated run-date value the DQ public API expects.
// Kind is "DATE" (value formatted yyyy-MM-dd) or "TIMESTAMP" (RFC 3339).
type DQRunDate struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// DQBackrun enables historical backfill runs relative to the run date.
type DQBackrun struct {
	TimeBin  string `json:"timeBin"`
	BinValue int    `json:"binValue"`
}

// RunDQJobResponse is the receipt returned from a job run trigger.
type RunDQJobResponse struct {
	JobRunID string `json:"jobRunId"`
}

// RunDQJob triggers a run of an existing DQ job via the public API. request may
// be nil to accept the service defaults.
func RunDQJob(ctx context.Context, client *http.Client, jobName string, request *RunDQJobRequest) (*RunDQJobResponse, error) {
	path := "/rest/dq/1.0/jobs/" + url.PathEscape(jobName) + "/run"

	var bodyReader io.Reader
	if request != nil {
		body, err := json.Marshal(request)
		if err != nil {
			return nil, fmt.Errorf("running dq job: marshaling request: %w", err)
		}
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("running dq job: building request: %w", err)
	}
	if request != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("running dq job: sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("running dq job: reading response: %w", err)
	}

	if resp.StatusCode != http.StatusAccepted {
		switch resp.StatusCode {
		case http.StatusBadRequest:
			return nil, fmt.Errorf("running dq job: bad request (invalid run parameters): %s", string(respBody))
		case http.StatusForbidden:
			return nil, fmt.Errorf("running dq job: missing permission to run this job: %s", string(respBody))
		case http.StatusNotFound:
			return nil, fmt.Errorf("running dq job: job %q not found: %s", jobName, string(respBody))
		default:
			return nil, fmt.Errorf("running dq job: unexpected status %d: %s", resp.StatusCode, string(respBody))
		}
	}

	var result RunDQJobResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("running dq job: decoding response: %w", err)
	}

	return &result, nil
}
