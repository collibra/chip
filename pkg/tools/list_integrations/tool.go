package list_integrations

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Platform     string `json:"platform,omitempty" jsonschema:"optional platform filter: databricks, dataplex, etc. Matches type.Id substring. If omitted, all capabilities are returned."`
	NameContains string `json:"nameContains,omitempty" jsonschema:"optional case-insensitive substring filter on capability name"`
}

type Integration struct {
	IngestibleId   string `json:"ingestibleId" jsonschema:"UUID of the integration instance, use this for all subsequent calls"`
	Name           string `json:"name" jsonschema:"capability name"`
	TypeId         string `json:"typeId" jsonschema:"capability type, e.g. databricks-edge-capability"`
	EdgeSiteId     string `json:"edgeSiteId" jsonschema:"Edge site this capability runs on"`
	HasSchedule    bool   `json:"hasSchedule" jsonschema:"whether a sync schedule is configured"`
	CronExpression string `json:"cronExpression,omitempty" jsonschema:"cron expression for the schedule"`
	CronTimeZone   string `json:"cronTimeZone,omitempty" jsonschema:"timezone for the cron schedule"`
	LastRun        string `json:"lastRun,omitempty" jsonschema:"relative time of last sync run, e.g. 6h ago"`
	LastRunAt      string `json:"lastRunAt,omitempty" jsonschema:"ISO 8601 timestamp of last sync run, use this for precise time comparisons e.g. ran in last 24h"`
	LastRunState   string `json:"lastRunState,omitempty" jsonschema:"state of last job: COMPLETED, FAILED, RUNNING, etc."`
	LastRunResult  string `json:"lastRunResult,omitempty" jsonschema:"result of last job: SUCCESS, FAILURE, COMPLETED_WITH_ERROR, etc."`
	NextRun        string `json:"nextRun,omitempty" jsonschema:"relative time of next scheduled run, e.g. in 18h"`
}

type Output struct {
	Integrations []Integration `json:"integrations" jsonschema:"list of integrations with schedule info"`
	Total        int           `json:"total" jsonschema:"total number of integrations returned"`
	Error        string        `json:"error,omitempty" jsonschema:"error message if the request failed"`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name: "list_integrations",
		Description: `Lists all integration capabilities with their sync schedule and last/next run times.
Use this as the entry point for any integration question: "show me all integrations", "show Databricks integrations", "what's scheduled for the Finance integration?".
Optionally filter by platform substring (e.g. "databricks", "dataplex") and/or name. Returns ingestibleId for each — pass it to catalog_etl_get_schedule, catalog_etl_start_job, or catalog_etl_cancel_job.
For time-based queries like "which integrations ran in the last 24h", use jobs_find sorted by startDate to get recent jobs, then cross-reference with integration names from this tool.`,
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		caps, err := clients.ListCapabilities(ctx, collibraClient)
		if err != nil {
			return Output{Error: fmt.Sprintf("failed to list capabilities: %s", err.Error())}, nil
		}

		filtered := filterCapabilities(caps, input)
		integrations := enrichWithSchedules(ctx, collibraClient, filtered)
		return Output{Integrations: integrations, Total: len(integrations)}, nil
	}
}

func filterCapabilities(caps []clients.EdgeCapability, input Input) []clients.EdgeCapability {
	platform := strings.ToLower(input.Platform)
	name := strings.ToLower(input.NameContains)

	result := make([]clients.EdgeCapability, 0, len(caps))
	for _, c := range caps {
		if platform != "" && platform != "all" {
			typeId := ""
			if c.Type != nil {
				typeId = strings.ToLower(c.Type.Id)
			}
			if !strings.Contains(typeId, platform) {
				continue
			}
		}

		if name != "" && !strings.Contains(strings.ToLower(c.Name), name) {
			continue
		}

		result = append(result, c)
	}
	return result
}

func enrichWithSchedules(ctx context.Context, collibraClient *http.Client, caps []clients.EdgeCapability) []Integration {
	integrations := make([]Integration, len(caps))
	now := time.Now()

	var wg sync.WaitGroup
	for i, c := range caps {
		wg.Add(1)
		go func(idx int, cap clients.EdgeCapability) {
			defer wg.Done()
			integrations[idx] = buildIntegration(ctx, collibraClient, cap, now)
		}(i, c)
	}
	wg.Wait()
	return integrations
}

func buildIntegration(ctx context.Context, collibraClient *http.Client, cap clients.EdgeCapability, now time.Time) Integration {
	typeId := ""
	if cap.Type != nil {
		typeId = cap.Type.Id
	}

	integration := Integration{
		IngestibleId: cap.Id,
		Name:         cap.Name,
		TypeId:       typeId,
		EdgeSiteId:   cap.EdgeSiteId,
	}

	schedule, err := clients.GetGenericSchedule(ctx, collibraClient, cap.Id)
	if err == nil && schedule != nil {
		integration.HasSchedule = true
		integration.CronExpression = schedule.CronExpression
		integration.CronTimeZone = schedule.CronTimeZone
		integration.NextRun = futureRelativeTime(schedule.NextRunDateLongValue, now)
		if schedule.LastRunTimeStamp > 0 {
			t := time.UnixMilli(schedule.LastRunTimeStamp)
			integration.LastRun = relativeTime(t, now)
			integration.LastRunAt = t.UTC().Format(time.RFC3339)
		}
	}

	// fall back to Jobs API for last run when schedule has no timestamp
	if integration.LastRun == "" {
		fillLastRunFromJobs(ctx, collibraClient, &integration, cap.Name, now)
	}

	return integration
}

func fillLastRunFromJobs(ctx context.Context, collibraClient *http.Client, integration *Integration, capName string, now time.Time) {
	resp, err := clients.FindJobs(ctx, collibraClient, capName, "ANYWHERE", nil, nil, nil, "", "START_DATE", "DESC", "", 1)
	if err != nil || len(resp.Results) == 0 {
		return
	}
	job := resp.Results[0]
	integration.LastRunState = job.State
	integration.LastRunResult = job.Result

	// prefer EndDate for completed jobs, fall back to StartDate
	dateStr := job.EndDate
	if dateStr == "" {
		dateStr = job.StartDate
	}
	if t := parseJobDate(dateStr); !t.IsZero() {
		integration.LastRun = relativeTime(t, now)
		integration.LastRunAt = t.UTC().Format(time.RFC3339)
	}
}

func parseJobDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02T15:04:05.999Z07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func relativeTime(t time.Time, now time.Time) string {
	diff := now.Sub(t)
	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(diff.Hours()/24))
	}
}

func futureRelativeTime(epochMs int64, now time.Time) string {
	if epochMs == 0 {
		return ""
	}
	diff := time.UnixMilli(epochMs).Sub(now)
	if diff <= 0 {
		return "overdue"
	}
	switch {
	case diff < time.Hour:
		return fmt.Sprintf("in %dm", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("in %dh", int(diff.Hours()))
	default:
		return fmt.Sprintf("in %dd", int(diff.Hours()/24))
	}
}
