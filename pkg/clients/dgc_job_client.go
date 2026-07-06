package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// CatalogJob is a DGC job (e.g. the job returned by
// POST /rest/catalogDatabase/v1/databases/{id}/synchronizeMetadata), as returned by
// GET /rest/jobs/v1/jobs/{jobId}. This is the DGC-wide job resource — distinct from
// edge-management's per-Edge-site jobs (see GetJobStatusLog / get_job_status), which
// track Edge capability runs and connection tests instead.
type CatalogJob struct {
	ID                 string  `json:"id"`
	Name               string  `json:"name,omitempty"`
	Type               string  `json:"type,omitempty"`
	State              string  `json:"state,omitempty"`
	Result             string  `json:"result,omitempty"`
	Message            string  `json:"message,omitempty"`
	ProgressPercentage float64 `json:"progressPercentage,omitempty"`
	StartDate          string  `json:"startDate,omitempty"`
	EndDate            string  `json:"endDate,omitempty"`
}

// GetCatalogJob fetches a DGC job's status via GET /rest/jobs/v1/jobs/{jobId}. This is
// the current, non-deprecated DGC job resource (the older /rest/2.0/jobs/{jobId} is
// deprecated for removal, per its own javadoc, in favor of this one).
func GetCatalogJob(ctx context.Context, client *http.Client, jobID string) (*CatalogJob, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/rest/jobs/v1/jobs/"+jobID, nil)
	if err != nil {
		return nil, fmt.Errorf("getting catalog job %q: building request: %w", jobID, err)
	}

	body, err := executeRequest(client, req)
	if err != nil {
		return nil, fmt.Errorf("getting catalog job %q: %w", jobID, err)
	}

	var result CatalogJob
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("getting catalog job %q: decoding response: %w", jobID, err)
	}
	return &result, nil
}
