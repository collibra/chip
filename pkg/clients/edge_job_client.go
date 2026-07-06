package clients

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
)

// TestConnectionResponse is the response shape returned by
// GET /edge/api/rest/v2/connections/{connectionId}/test.
type TestConnectionResponse struct {
	JobID   string `json:"jobId"`
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// TestConnection tests an Edge connection via
// GET /edge/api/rest/v2/connections/{connectionId}/test?timeoutSec=. When timeoutSec
// is nil (or <= 0), the call returns immediately with a jobId while the test runs in
// the background — poll its result with GetJobStatusLog. When timeoutSec is positive,
// the call blocks up to that many seconds and returns the final result directly.
func TestConnection(ctx context.Context, client *http.Client, connectionID string, timeoutSec *int) (*TestConnectionResponse, error) {
	endpoint := "/edge/api/rest/v2/connections/" + connectionID + "/test"
	if timeoutSec != nil && *timeoutSec > 0 {
		endpoint += "?timeoutSec=" + strconv.Itoa(*timeoutSec)
	}

	var result TestConnectionResponse
	if err := getJSON(ctx, client, endpoint, "testing connection", &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// JobStatusLog is the response shape returned by GET /edge/api/rest/v2/jobs/{id}/statusLog.
type JobStatusLog struct {
	JobID               string `json:"jobId"`
	Status              string `json:"status"`
	Message             string `json:"message,omitempty"`
	LastUpdatedDateTime string `json:"lastUpdatedDateTime,omitempty"`
}

// GetJobStatusLog fetches the current status of an Edge job (e.g. one started by
// TestConnection or start_ingestion's SynchronizeDatabaseMetadata) via
// GET /edge/api/rest/v2/jobs/{id}/statusLog. Terminal statuses include SUCCEEDED,
// FAILED, CANCELLED, CAPABILITY_SUCCEEDED, and CAPABILITY_FAILED.
func GetJobStatusLog(ctx context.Context, client *http.Client, jobID string) (*JobStatusLog, error) {
	endpoint := "/edge/api/rest/v2/jobs/" + jobID + "/statusLog"
	var result JobStatusLog
	if err := getJSON(ctx, client, endpoint, "getting job status", &result); err != nil {
		return nil, err
	}
	if result.JobID == "" {
		return nil, fmt.Errorf("getting job status: empty response for job %s", jobID)
	}
	return &result, nil
}
