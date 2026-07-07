package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const genericIntegrationBasePath = "/rest/catalog/1.0/genericIntegration"

// --- Types ---

type GenericConfiguration struct {
	Id          string `json:"id,omitempty"`
	IngestibleId string `json:"ingestibleId,omitempty"`
	Value       string `json:"value,omitempty"`
}

type SaveGenericConfigRequest struct {
	Configuration string `json:"configuration"`
}

type GenericSchedule struct {
	Id                 int64  `json:"id,omitempty"`
	CronExpression     string `json:"cronExpression,omitempty"`
	CronTimeZone       string `json:"cronTimeZone,omitempty"`
	LastRunTimeStamp   int64  `json:"lastRunTimeStamp,omitempty"`
	NextRunDateLongValue int64 `json:"nextRunDateLongValue,omitempty"`
	CronJson           string `json:"cronJson,omitempty"`
	Workflow           string `json:"workflow,omitempty"`
}

type AddGenericScheduleRequest struct {
	CronExpression string `json:"cronExpression"`
	CronTimeZone   string `json:"cronTimeZone"`
	CronJson       string `json:"cronJson,omitempty"`
}

type ChangeGenericScheduleRequest struct {
	CronExpression string `json:"cronExpression"`
	CronTimeZone   string `json:"cronTimeZone"`
	CronJson       string `json:"cronJson,omitempty"`
}

type GenericJob struct {
	Id                string  `json:"id,omitempty"`
	Name              string  `json:"name,omitempty"`
	Type              string  `json:"type,omitempty"`
	State             string  `json:"state,omitempty"`
	Result            string  `json:"result,omitempty"`
	Message           string  `json:"message,omitempty"`
	ProgressPercentage float64 `json:"progressPercentage,omitempty"`
	Cancelable        bool    `json:"cancelable,omitempty"`
	StartDate         int64   `json:"startDate,omitempty"`
	EndDate           int64   `json:"endDate,omitempty"`
	CreatedBy         string  `json:"createdBy,omitempty"`
	CreatedOn         int64   `json:"createdOn,omitempty"`
	UserId            string  `json:"userId,omitempty"`
}

type genericScheduleParams struct {
	Workflow string `url:"workflow,omitempty"`
}

type startJobParams struct {
	CloudIngestionJobId string `url:"cloudIngestionJobId,omitempty"`
	Workflow            string `url:"workflow,omitempty"`
}

// --- Configuration functions ---

func GetGenericConfig(ctx context.Context, client *http.Client, ingestibleId string) (*GenericConfiguration, error) {
	endpoint := genericIntegrationBasePath + "/" + ingestibleId + "/configuration"
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	body, err := executeRequest(client, req)
	if err != nil {
		return nil, err
	}
	var config GenericConfiguration
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config response: %w", err)
	}
	return &config, nil
}

func SaveGenericConfig(ctx context.Context, client *http.Client, ingestibleId string, reqBody SaveGenericConfigRequest) (*GenericConfiguration, error) {
	endpoint := genericIntegrationBasePath + "/" + ingestibleId + "/configuration"
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "PUT", endpoint, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	body, err := executeRequest(client, req)
	if err != nil {
		return nil, err
	}
	var config GenericConfiguration
	if err := json.Unmarshal(body, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config response: %w", err)
	}
	return &config, nil
}

func DeleteGenericConfig(ctx context.Context, client *http.Client, ingestibleId string) error {
	endpoint := genericIntegrationBasePath + "/" + ingestibleId + "/configuration"
	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	_, err = executeRequest(client, req)
	return err
}

func GetGenericSchema(ctx context.Context, client *http.Client, ingestibleId string) (string, error) {
	endpoint := genericIntegrationBasePath + "/" + ingestibleId + "/configuration/schema"
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	body, err := executeRequest(client, req)
	if err != nil {
		return "", err
	}
	// Response is a JSON string value
	var schema string
	if err := json.Unmarshal(body, &schema); err != nil {
		// If not a quoted string, return raw body
		return string(body), nil
	}
	return schema, nil
}

// --- Schedule functions ---

func GetGenericSchedule(ctx context.Context, client *http.Client, ingestibleId string) (*GenericSchedule, error) {
	endpoint := genericIntegrationBasePath + "/" + ingestibleId + "/schedule"
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	body, err := executeRequest(client, req)
	if err != nil {
		return nil, err
	}
	var schedule GenericSchedule
	if err := json.Unmarshal(body, &schedule); err != nil {
		return nil, fmt.Errorf("failed to parse schedule response: %w", err)
	}
	return &schedule, nil
}

func AddGenericSchedule(ctx context.Context, client *http.Client, ingestibleId string, workflow string, reqBody AddGenericScheduleRequest) (*GenericSchedule, error) {
	params := genericScheduleParams{Workflow: workflow}
	endpoint, err := buildUrl(genericIntegrationBasePath+"/"+ingestibleId+"/schedule", params)
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint: %w", err)
	}
	return doGenericScheduleRequest(ctx, client, "POST", endpoint, reqBody)
}

func UpdateGenericSchedule(ctx context.Context, client *http.Client, ingestibleId string, workflow string, reqBody ChangeGenericScheduleRequest) (*GenericSchedule, error) {
	params := genericScheduleParams{Workflow: workflow}
	endpoint, err := buildUrl(genericIntegrationBasePath+"/"+ingestibleId+"/schedule", params)
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint: %w", err)
	}
	return doGenericScheduleRequest(ctx, client, "PUT", endpoint, reqBody)
}

func DeleteGenericSchedule(ctx context.Context, client *http.Client, ingestibleId string, workflow string) error {
	params := genericScheduleParams{Workflow: workflow}
	endpoint, err := buildUrl(genericIntegrationBasePath+"/"+ingestibleId+"/schedule", params)
	if err != nil {
		return fmt.Errorf("failed to build endpoint: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	_, err = executeRequest(client, req)
	return err
}

func GetAllGenericSchedules(ctx context.Context, client *http.Client, ingestibleId string) (*GenericSchedule, error) {
	endpoint := genericIntegrationBasePath + "/" + ingestibleId + "/schedules"
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	body, err := executeRequest(client, req)
	if err != nil {
		return nil, err
	}
	var schedule GenericSchedule
	if err := json.Unmarshal(body, &schedule); err != nil {
		return nil, fmt.Errorf("failed to parse schedules response: %w", err)
	}
	return &schedule, nil
}

func doGenericScheduleRequest(ctx context.Context, client *http.Client, method, endpoint string, reqBody any) (*GenericSchedule, error) {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	body, err := executeRequest(client, req)
	if err != nil {
		return nil, err
	}
	var schedule GenericSchedule
	if err := json.Unmarshal(body, &schedule); err != nil {
		return nil, fmt.Errorf("failed to parse schedule response: %w", err)
	}
	return &schedule, nil
}

// --- Job functions ---

func CancelGenericJob(ctx context.Context, client *http.Client, ingestibleId string, workflow string) error {
	params := genericScheduleParams{Workflow: workflow}
	endpoint, err := buildUrl(genericIntegrationBasePath+"/"+ingestibleId+"/cancel", params)
	if err != nil {
		return fmt.Errorf("failed to build endpoint: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, "DELETE", endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	_, err = executeRequest(client, req)
	return err
}

func StartGenericJob(ctx context.Context, client *http.Client, ingestibleId string, cloudIngestionJobId string, workflow string, runtimeArguments string) (*GenericJob, error) {
	params := startJobParams{
		CloudIngestionJobId: cloudIngestionJobId,
		Workflow:            workflow,
	}
	endpoint, err := buildUrl(genericIntegrationBasePath+"/"+ingestibleId+"/run", params)
	if err != nil {
		return nil, fmt.Errorf("failed to build endpoint: %w", err)
	}

	var bodyReader *bytes.Reader
	if runtimeArguments != "" {
		data, err := json.Marshal(runtimeArguments)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal runtime arguments: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	} else {
		bodyReader = bytes.NewReader([]byte{})
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	body, err := executeRequest(client, req)
	if err != nil {
		return nil, err
	}
	var job GenericJob
	if err := json.Unmarshal(body, &job); err != nil {
		return nil, fmt.Errorf("failed to parse job response: %w", err)
	}
	return &job, nil
}