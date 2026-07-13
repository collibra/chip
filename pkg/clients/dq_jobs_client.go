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

// DQRunDateValue is the discriminated runDate value on a job run ({kind, value}).
type DQRunDateValue struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// DQJobRun is the status of a single job run, from
// GET /rest/dq/1.0/jobRuns/{jobRunId}. status is the JobRunStatus enum
// (WAITING, DISPATCHED, SETUP, RUNNING, SENDING, FINISHED, CANCELLED, FAILED,
// UNKNOWN — the field is open, so additional values may appear). exception is
// only populated on FAILED runs.
type DQJobRun struct {
	JobRunID             string          `json:"jobRunId"`
	JobName              string          `json:"jobName"`
	RunDate              *DQRunDateValue `json:"runDate,omitempty"`
	Status               string          `json:"status"`
	Activity             string          `json:"activity,omitempty"`
	Exception            string          `json:"exception,omitempty"`
	StartTime            string          `json:"startTime,omitempty"`
	EndTime              string          `json:"endTime,omitempty"`
	ExecutionTimeSeconds *int64          `json:"executionTimeSeconds,omitempty"`
	Score                *float64        `json:"score,omitempty"`
	ActiveMonitors       *int            `json:"activeMonitors,omitempty"`
	BreakingMonitors     *int            `json:"breakingMonitors,omitempty"`
	RowCount             *int64          `json:"rowCount,omitempty"`
	ExecutedQuery        string          `json:"executedQuery,omitempty"`
}

// GetDQJobRunStatus fetches the status of a job run by its run id (the jobRunId
// returned when the job was run) — GET /rest/dq/1.0/jobRuns/{jobRunId}.
func GetDQJobRunStatus(ctx context.Context, client *http.Client, jobRunID string) (*DQJobRun, error) {
	path := "/rest/dq/1.0/jobRuns/" + url.PathEscape(jobRunID)
	respBody, status, err := dqDo(ctx, client, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("getting dq job status: %w", err)
	}
	if status != http.StatusOK {
		if status == http.StatusNotFound {
			return nil, fmt.Errorf("getting dq job status: job run %q not found: %s", jobRunID, string(respBody))
		}
		return nil, fmt.Errorf("getting dq job status: unexpected status %d: %s", status, string(respBody))
	}
	var result DQJobRun
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("getting dq job status: decoding response: %w", err)
	}
	return &result, nil
}

// DQJobLogEntry is one entry in a job run's execution log (a JobLog): the stage,
// activity and a human-readable description with an optional hint.
type DQJobLogEntry struct {
	LogID           int64  `json:"logId"`
	Activity        string `json:"activity,omitempty"`
	Stage           string `json:"stage,omitempty"`
	LogDesc         string `json:"logDesc,omitempty"`
	LogHint         string `json:"logHint,omitempty"`
	StageTime       int64  `json:"stageTime,omitempty"`
	PrettyStageTime string `json:"prettyStageTime,omitempty"`
}

// GetDQJobLog fetches the execution log for a job run by its run id. NOTE: there
// is no public log endpoint, so this uses the internal UI surface —
// GET /rest/dq/internal/v1/job/logs?jobUUID={jobRunId}.
func GetDQJobLog(ctx context.Context, client *http.Client, jobRunID string) ([]DQJobLogEntry, error) {
	path := "/rest/dq/internal/v1/job/logs?" + url.Values{"jobUUID": {jobRunID}}.Encode()
	respBody, status, err := dqDo(ctx, client, http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("getting dq job log: %w", err)
	}
	if status != http.StatusOK {
		if status == http.StatusNotFound {
			return nil, fmt.Errorf("getting dq job log: job run %q not found: %s", jobRunID, string(respBody))
		}
		return nil, fmt.Errorf("getting dq job log: unexpected status %d: %s", status, string(respBody))
	}
	var result []DQJobLogEntry
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("getting dq job log: decoding response: %w", err)
	}
	return result, nil
}
