package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Job is a DGC job as returned by the Jobs API — GET /rest/jobs/v1/jobs and
// GET /rest/jobs/v1/jobs/{jobId}. Its fields mirror the Job schema in job-api.yaml.
// This is the DGC-wide job resource you poll for status (e.g. after triggering a
// catalog database sync, whose CatalogJob response carries the id to poll here). It is
// distinct from edge-management's per-Edge-site jobs (see GetJobStatusLog /
// get_job_status), which track Edge capability runs and connection tests instead.
type Job struct {
	ID                 string `json:"id"`
	Name               string `json:"name,omitempty"`
	Type               string `json:"type,omitempty"`
	State              string `json:"state,omitempty"`
	Result             string `json:"result,omitempty"`
	Message            string `json:"message,omitempty"`
	ProgressPercentage int    `json:"progressPercentage,omitempty"`
	StartDate          string `json:"startDate,omitempty"`
	EndDate            string `json:"endDate,omitempty"`
	CreatedBy          string `json:"createdBy,omitempty"`
	CreatedOn          string `json:"createdOn,omitempty"`
	LastModifiedBy     string `json:"lastModifiedBy,omitempty"`
	LastModifiedOn     string `json:"lastModifiedOn,omitempty"`
	User               string `json:"user,omitempty"`
	SelfManaged        bool   `json:"selfManaged,omitempty"`
}

// JobPagedResponse is the paged response returned by GET /rest/jobs/v1/jobs.
type JobPagedResponse struct {
	Results    []Job  `json:"results"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type jobFindParams struct {
	Name          string   `url:"name,omitempty"`
	NameMatchMode string   `url:"nameMatchMode,omitempty"`
	Result        []string `url:"result,omitempty"`
	State         []string `url:"state,omitempty"`
	Type          []string `url:"type,omitempty"`
	User          string   `url:"user,omitempty"`
	SortField     string   `url:"sortField,omitempty"`
	SortOrder     string   `url:"sortOrder,omitempty"`
	Cursor        string   `url:"cursor,omitempty"`
	PageSize      int      `url:"pageSize,omitempty"`
}

// GetJob fetches a DGC job's status via GET /rest/jobs/v1/jobs/{jobId}. This is the
// current, non-deprecated DGC job resource (the older /rest/2.0/jobs/{jobId} is
// deprecated for removal, per its own javadoc, in favor of this one).
func GetJob(ctx context.Context, client *http.Client, jobID string) (*Job, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/rest/jobs/v1/jobs/"+jobID, nil)
	if err != nil {
		return nil, fmt.Errorf("getting job %q: building request: %w", jobID, err)
	}

	body, err := executeRequest(client, req)
	if err != nil {
		return nil, fmt.Errorf("getting job %q: %w", jobID, err)
	}

	var result Job
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("getting job %q: decoding response: %w", jobID, err)
	}
	return &result, nil
}

// FindJobs searches DGC jobs via GET /rest/jobs/v1/jobs, applying the given filters
// and pagination. All filters are optional.
func FindJobs(
	ctx context.Context,
	client *http.Client,
	name string,
	nameMatchMode string,
	result []string,
	state []string,
	jobType []string,
	user string,
	sortField string,
	sortOrder string,
	cursor string,
	pageSize int,
) (*JobPagedResponse, error) {
	params := jobFindParams{
		Name:          name,
		NameMatchMode: nameMatchMode,
		Result:        result,
		State:         state,
		Type:          jobType,
		User:          user,
		SortField:     sortField,
		SortOrder:     sortOrder,
		Cursor:        cursor,
		PageSize:      pageSize,
	}

	endpoint, err := buildUrl("/rest/jobs/v1/jobs", params)
	if err != nil {
		return nil, fmt.Errorf("finding jobs: building endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("finding jobs: building request: %w", err)
	}

	body, err := executeRequest(client, req)
	if err != nil {
		return nil, fmt.Errorf("finding jobs: %w", err)
	}

	var resp JobPagedResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("finding jobs: decoding response: %w", err)
	}
	return &resp, nil
}
