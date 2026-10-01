// Package get_dq_job_run_logs implements the dq_get_job_run_logs MCP tool — explain why a Collibra
// data-quality job run was slow or failed, from its Edge stage logs, in one call.
//
// The flow combines public and private GETs:
//
//	Run  : GET /rest/dq/1.0/jobRuns/{jobRunId} -> status, exception, executed query.
//	Job  : GET /rest/dq/1.0/jobs/{jobName}     -> job type, only when the run does not carry it.
//	Logs : GET /rest/dq/internal/v1/job/{jobRunId}/edge/platformLogs?type=SUBMIT|DRIVER
//	       -> the full stored "Stage 2" (Spark submit) and "Stage 3" (Spark driver) logs shown under
//	       the UI's "Stage logs" button. Pushdown runs only have a SUBMIT log.
//
// The logs are large (the submit log repeats a pod-status line every second), so by default the tool
// returns parsed structure instead of text: sparkConfig (from SUBMIT), jobSummary (from DRIVER),
// rule-based diagnostics, and distinct error lines. Raw text is opt-in via head/tail/grep/offset
// parameters, and the whole response is kept under responseBudget characters. Every log-derived string
// is redacted before it is returned.
//
// Logs are only stored when Edge debug logging is enabled on the job's Edge capability, and they are
// purged after the platform log retention period; that case is reported as status "not_available".
package get_dq_job_run_logs

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Status string

const (
	StatusSuccess      Status = "success"
	StatusNotAvailable Status = "not_available"
	StatusNeedsInput   Status = "needs_input"
	StatusError        Status = "error"
)

const (
	responseBudget        = 15_000
	defaultTailLines      = 200
	defaultHeadLines      = 100
	maxRawLines           = 2000
	defaultGrepContext    = 2
	maxGrepContext        = 10
	defaultGrepMaxMatches = 50
	maxGrepMatches        = 200
)

const (
	includeSummary = "summary"
	includeConfig  = "config"
	includeErrors  = "errors"
	includeHead    = "head"
	includeTail    = "tail"
	includeGrep    = "grep"
	includeRange   = "range"
)

var (
	validIncludes   = []string{includeSummary, includeConfig, includeErrors, includeHead, includeTail, includeGrep, includeRange}
	defaultIncludes = []string{includeSummary, includeConfig, includeErrors}
)

// Input is the tool's typed input.
type Input struct {
	RunID           string   `json:"run_id" jsonschema:"Required. The id (jobRunId) of the job run."`
	Stage           string   `json:"stage,omitempty" jsonschema:"Optional. SUBMIT (Stage 2, Spark submit log) or DRIVER (Stage 3, Spark driver log; pullup only). Omit for every stage that applies."`
	Include         []string `json:"include,omitempty" jsonschema:"Optional sections: summary, config, errors, head, tail, grep, range. Default [summary, config, errors]. Setting head_lines/tail_lines/grep/offset/limit adds the matching section automatically."`
	CollapseRepeats *bool    `json:"collapse_repeats,omitempty" jsonschema:"Optional, default true. Fold consecutive lines that differ only by timestamp into one '[repeated Nx, first–last]' line."`
	HeadLines       int      `json:"head_lines,omitempty" jsonschema:"Optional. First N (collapsed) lines of each log, max 2000."`
	TailLines       int      `json:"tail_lines,omitempty" jsonschema:"Optional. Last N (collapsed) lines of each log, max 2000 (200 when 'tail' is included without a count)."`
	Grep            string   `json:"grep,omitempty" jsonschema:"Optional. RE2 regex to search each log for (case-sensitive; prefix (?i) to ignore case)."`
	GrepContext     *int     `json:"grep_context,omitempty" jsonschema:"Optional. Lines of context around each grep match (default 2, max 10)."`
	GrepMaxMatches  int      `json:"grep_max_matches,omitempty" jsonschema:"Optional. Max grep matches per log (default 50, max 200)."`
	Offset          int      `json:"offset,omitempty" jsonschema:"Optional. With limit: skip this many raw lines. Line numbers in the output are raw line numbers."`
	Limit           int      `json:"limit,omitempty" jsonschema:"Optional. With offset: return raw lines offset+1..offset+limit (max 2000)."`
}

// StageLog is one stage's log: size, error lines, and any requested raw sections.
type StageLog struct {
	Stage            string   `json:"stage" jsonschema:"SUBMIT or DRIVER."`
	Label            string   `json:"label" jsonschema:"The name used in the Collibra UI's Stage logs menu."`
	Available        bool     `json:"available" jsonschema:"False when no log was stored for this stage."`
	TotalLines       int      `json:"totalLines"`
	TotalBytes       int      `json:"totalBytes"`
	CollapsedLines   int      `json:"collapsedLines" jsonschema:"Raw lines folded away by collapse_repeats."`
	ErrorLines       []string `json:"errorLines,omitempty" jsonschema:"Distinct ERROR/FATAL/Exception/Caused by lines as '<line>: <text> [count]' (DEBUG/TRACE excluded, capped)."`
	Head             []string `json:"head,omitempty"`
	Tail             []string `json:"tail,omitempty"`
	Range            []string `json:"range,omitempty"`
	Grep             []string `json:"grep,omitempty" jsonschema:"One block per match; '>' marks matched lines, '|' context."`
	GrepTotalMatches *int     `json:"grepTotalMatches,omitempty"`
	Truncated        bool     `json:"truncated,omitempty"`
	Hint             string   `json:"hint,omitempty" jsonschema:"When truncated: which parameter to use next."`
}

// Output is the typed response.
type Output struct {
	Status         Status       `json:"status" jsonschema:"'success' when at least one stage log was found; 'not_available' when none were stored; 'needs_input' for bad inputs; 'error' for downstream DQ failures."`
	Message        string       `json:"message"`
	JobRunID       string       `json:"jobRunId,omitempty"`
	JobName        string       `json:"jobName,omitempty"`
	JobType        string       `json:"jobType,omitempty" jsonschema:"PULLUP or PUSHDOWN."`
	RunStatus      string       `json:"runStatus,omitempty"`
	ExecutionSec   *int64       `json:"executionTimeSeconds,omitempty"`
	Exception      string       `json:"exception,omitempty" jsonschema:"The run's failure message, when status is FAILED."`
	Diagnostics    []Diagnostic `json:"diagnostics,omitempty" jsonschema:"Rule-based hints about slowness/failure, each with evidence lines."`
	SparkConfig    *SparkConfig `json:"sparkConfig,omitempty" jsonschema:"Spark/Kubernetes sizing parsed from the SUBMIT log."`
	JobSummary     *JobSummary  `json:"jobSummary,omitempty" jsonschema:"Phases, Spark stages, partitions, executors, rows, exit status parsed from the DRIVER log (pullup only)."`
	Logs           []StageLog   `json:"logs,omitempty"`
	Truncated      bool         `json:"truncated,omitempty"`
	Hint           string       `json:"hint,omitempty"`
	JobDetailsLink string       `json:"jobDetailsLink,omitempty"`
	Guidance       string       `json:"guidance,omitempty" jsonschema:"What to do next, especially when logs are not available."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "dq_get_job_run_logs",
		Title: "Diagnose Data Quality Job Run from Stage Logs",
		Description: "Explains why a Collibra data-quality job run was slow or failed, from its Edge stage logs " +
			"(Spark submit log = Stage 2, Spark driver log = Stage 3, pullup only). Use after dq_get_job_run " +
			"when a run FAILED or took unexpectedly long.\n\n" +
			"The default call (just run_id) returns a small, parsed view: diagnostics (rule-based hints with " +
			"evidence), sparkConfig (executor/driver sizing, k8s requests/limits), jobSummary (phase durations, " +
			"slowest Spark stages, partitions, executors, rows loaded, exit status) and distinct error lines. " +
			"Raw log text is not returned unless asked: for follow-up digging use grep (+grep_context), " +
			"head_lines, tail_lines, or offset/limit (line numbers in the output are raw log line numbers).\n\n" +
			"Logs exist only when Edge debug logging is enabled on the job's Edge capability; otherwise status " +
			"is 'not_available' with guidance.\n\n" +
			"Example user requests: \"Why did DQ run <id> fail?\"; \"Why did job run <id> take so long?\"; " +
			"\"Show me the Spark logs for run <id>\"",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		opts, bad := parseOptions(input)
		if bad != nil {
			return *bad, nil
		}

		run, code, err := clients.GetDqJobRun(ctx, collibraClient, opts.runID)
		if err != nil {
			return lookupError(code, err, opts.runID, "look up run"), nil
		}
		out := Output{
			JobRunID:     opts.runID,
			JobName:      run.JobName,
			JobType:      resolveJobType(ctx, collibraClient, run),
			RunStatus:    run.Status,
			ExecutionSec: run.ExecutionTimeSeconds,
			Exception:    run.Exception,
		}
		if run.JobName != "" {
			out.JobDetailsLink = clients.DqJobDetailsPath(run.JobName)
		}

		isPushdown := strings.EqualFold(out.JobType, "PUSHDOWN")
		if opts.stage == clients.DqPlatformLogDriver && isPushdown {
			out.Status = StatusNeedsInput
			out.Message = fmt.Sprintf("Run %q is a pushdown job; pushdown jobs have no driver (Stage 3) log.", opts.runID)
			out.Guidance = "Omit stage, or use SUBMIT to read the Stage 2 log."
			return out, nil
		}

		texts := map[clients.DqPlatformLogType]string{}
		for _, s := range stagesToRead(opts.stage, isPushdown) {
			text, code, err := clients.GetDqJobRunPlatformLog(ctx, collibraClient, opts.runID, s)
			if err != nil {
				failure := lookupError(code, err, opts.runID, fmt.Sprintf("read the %s log of run", s))
				out.Status, out.Message, out.Guidance = failure.Status, failure.Message, failure.Guidance
				return out, nil
			}
			texts[s] = text
			out.Logs = append(out.Logs, StageLog{Stage: string(s), Label: stageLabel(s)})
		}

		if !anyText(texts) {
			out.Status = StatusNotAvailable
			out.Message = fmt.Sprintf("No stage logs are stored for run %q (status %s).", opts.runID, run.Status)
			out.Guidance = notAvailableGuidance(run.Status)
			return out, nil
		}

		query := run.ExecutedQuery
		if query == "" {
			query = run.SourceQuery
		}
		analyze(&out, texts, opts, query)
		fitBudget(&out)

		out.Status = StatusSuccess
		out.Message = fmt.Sprintf("Analyzed stage logs for run %q (status %s): %d diagnostic(s).", opts.runID, run.Status, len(out.Diagnostics))
		if out.Hint == "" && !opts.anyRaw() {
			out.Hint = "Raw log text omitted. Follow up with grep (+grep_context), head_lines, tail_lines, or offset/limit on the line numbers shown."
		}
		return out, nil
	}
}

// options is the validated form of Input.
type options struct {
	runID          string
	stage          clients.DqPlatformLogType
	include        map[string]bool
	collapse       bool
	headLines      int
	tailLines      int
	grep           *regexp.Regexp
	grepContext    int
	grepMaxMatches int
	offset         int
	limit          int
}

func (o options) anyRaw() bool {
	return o.include[includeHead] || o.include[includeTail] || o.include[includeGrep] || o.include[includeRange]
}

func parseOptions(in Input) (options, *Output) {
	needs := func(msg, guidance string) (options, *Output) {
		return options{}, &Output{Status: StatusNeedsInput, Message: msg, Guidance: guidance}
	}
	o := options{runID: strings.TrimSpace(in.RunID), collapse: in.CollapseRepeats == nil || *in.CollapseRepeats}
	if o.runID == "" {
		return needs("Provide the run whose logs to read.",
			"Supply run_id — the id (jobRunId) of the job run. Use dq_search_job_runs to find it by job name.")
	}
	o.stage = clients.DqPlatformLogType(strings.ToUpper(strings.TrimSpace(in.Stage)))
	if o.stage != "" && o.stage != clients.DqPlatformLogSubmit && o.stage != clients.DqPlatformLogDriver {
		return needs(fmt.Sprintf("Unknown stage %q.", in.Stage), "Use SUBMIT (Stage 2) or DRIVER (Stage 3), or omit stage.")
	}

	o.include = map[string]bool{}
	includes := in.Include
	if len(includes) == 0 {
		includes = defaultIncludes
	}
	for _, name := range includes {
		name = strings.ToLower(strings.TrimSpace(name))
		if !slices.Contains(validIncludes, name) {
			return needs(fmt.Sprintf("Unknown include section %q.", name), "Valid sections: "+strings.Join(validIncludes, ", ")+".")
		}
		o.include[name] = true
	}

	o.headLines = clamp(in.HeadLines, 0, maxRawLines)
	o.tailLines = clamp(in.TailLines, 0, maxRawLines)
	o.offset = max(in.Offset, 0)
	o.limit = clamp(in.Limit, 0, maxRawLines)
	o.include[includeHead] = o.include[includeHead] || o.headLines > 0
	o.include[includeTail] = o.include[includeTail] || o.tailLines > 0
	o.include[includeRange] = o.include[includeRange] || o.limit > 0 || o.offset > 0
	if o.include[includeHead] && o.headLines == 0 {
		o.headLines = defaultHeadLines
	}
	if o.include[includeTail] && o.tailLines == 0 {
		o.tailLines = defaultTailLines
	}
	if o.include[includeRange] && o.limit == 0 {
		o.limit = defaultHeadLines
	}

	if in.Grep != "" {
		re, err := regexp.Compile(in.Grep)
		if err != nil {
			return needs(fmt.Sprintf("Invalid grep regex: %v", err), "grep uses RE2 syntax; escape special characters or simplify the pattern.")
		}
		o.grep = re
		o.include[includeGrep] = true
	} else if o.include[includeGrep] {
		return needs("include has 'grep' but no grep pattern was given.", "Set grep to an RE2 regex.")
	}
	o.grepContext = defaultGrepContext
	if in.GrepContext != nil {
		o.grepContext = clamp(*in.GrepContext, 0, maxGrepContext)
	}
	o.grepMaxMatches = defaultGrepMaxMatches
	if in.GrepMaxMatches > 0 {
		o.grepMaxMatches = min(in.GrepMaxMatches, maxGrepMatches)
	}
	return o, nil
}

// resolveJobType prefers the run's jobType; the public run resource often omits it, so fall back to
// the job definition. Unknown is fine — both stages are then read.
func resolveJobType(ctx context.Context, collibraClient *http.Client, run *clients.DqJobRun) string {
	if run.JobType != "" || run.JobName == "" {
		return run.JobType
	}
	job, _, err := clients.GetDqJob(ctx, collibraClient, run.JobName)
	if err != nil || job == nil {
		return ""
	}
	return job.JobType
}

func stagesToRead(requested clients.DqPlatformLogType, isPushdown bool) []clients.DqPlatformLogType {
	if requested != "" {
		return []clients.DqPlatformLogType{requested}
	}
	if isPushdown {
		return []clients.DqPlatformLogType{clients.DqPlatformLogSubmit}
	}
	return []clients.DqPlatformLogType{clients.DqPlatformLogSubmit, clients.DqPlatformLogDriver}
}

// analyze fills the parsed blocks and per-log sections.
func analyze(out *Output, texts map[clients.DqPlatformLogType]string, opts options, query string) {
	lines := map[clients.DqPlatformLogType][]string{}
	for i := range out.Logs {
		sl := &out.Logs[i]
		text := texts[clients.DqPlatformLogType(sl.Stage)]
		if strings.TrimSpace(text) == "" {
			continue
		}
		raw := splitLines(text)
		lines[clients.DqPlatformLogType(sl.Stage)] = raw
		fillStageLog(sl, raw, len(text), opts)
	}

	if opts.include[includeConfig] {
		if submit, ok := lines[clients.DqPlatformLogSubmit]; ok {
			out.SparkConfig = parseSparkConfig(submit)
		}
	}
	if opts.include[includeSummary] {
		driver := lines[clients.DqPlatformLogDriver]
		var ev summaryEvidence
		if driver != nil {
			out.JobSummary, ev = parseJobSummary(driver)
		}
		cfg := out.SparkConfig
		if cfg == nil {
			cfg = parseSparkConfig(lines[clients.DqPlatformLogSubmit])
		}
		out.Diagnostics = diagnose(out.JobSummary, ev, cfg, driver, query)
	}
	for _, sl := range out.Logs {
		out.Truncated = out.Truncated || sl.Truncated
	}
}

func fillStageLog(sl *StageLog, raw []string, bytes int, opts options) {
	entries := collapse(raw, opts.collapse)
	sl.Available = true
	sl.TotalLines = len(raw)
	sl.TotalBytes = bytes
	sl.CollapsedLines = len(raw) - len(entries)

	var hints []string
	if opts.include[includeErrors] {
		var more bool
		sl.ErrorLines, more = collectErrors(entries)
		if more {
			hints = append(hints, fmt.Sprintf("More than %d distinct error lines; use grep to search for specific errors.", maxErrorLines))
		}
	}
	if opts.include[includeHead] {
		sl.Head = headOf(entries, opts.headLines)
	}
	if opts.include[includeTail] {
		sl.Tail = tailOf(entries, opts.tailLines)
	}
	if opts.include[includeRange] {
		sl.Range = rangeOf(entries, opts.offset, opts.limit)
		if opts.offset+opts.limit < len(raw) {
			hints = append(hints, fmt.Sprintf("Range ends at line %d of %d; continue with offset=%d.", opts.offset+opts.limit, len(raw), opts.offset+opts.limit))
		}
	}
	if opts.include[includeGrep] {
		blocks, total := grepOf(entries, opts.grep, opts.grepContext, opts.grepMaxMatches)
		sl.Grep, sl.GrepTotalMatches = blocks, &total
		if total > opts.grepMaxMatches {
			hints = append(hints, fmt.Sprintf("grep matched %d lines, showing %d; tighten the regex or raise grep_max_matches.", total, opts.grepMaxMatches))
		}
	}
	if len(hints) > 0 {
		sl.Truncated = true
		sl.Hint = strings.Join(hints, " ")
	}
}

// ---- response budget ----

var rawHints = map[string]string{
	includeHead:  "head cut to fit the response budget; page on with offset/limit.",
	includeTail:  "tail cut to fit the response budget; page earlier lines with offset/limit.",
	includeRange: "range cut to fit the response budget; use a smaller limit and page with offset.",
	includeGrep:  "grep output cut to fit the response budget; tighten the regex or lower grep_context/grep_max_matches.",
}

// fitBudget trims raw sections (largest first), then secondary detail, until the JSON response fits.
func fitBudget(out *Output) {
	for range 50 {
		excess := jsonSize(out) - responseBudget
		if excess <= 0 {
			return
		}
		if !trimLargestRaw(out, excess) {
			break
		}
	}
	if jsonSize(out) <= responseBudget {
		return
	}
	for i := range out.Logs {
		if len(out.Logs[i].ErrorLines) > 10 {
			out.Logs[i].ErrorLines = out.Logs[i].ErrorLines[:10]
			markTruncated(out, &out.Logs[i], "errorLines cut to 10 to fit the response budget; use grep for the rest.")
		}
	}
	if jsonSize(out) > responseBudget && out.SparkConfig != nil && out.SparkConfig.Conf != nil {
		out.SparkConfig.Conf = nil
		out.Truncated = true
		out.Hint = "sparkConfig.conf omitted to fit the response budget; request include=[\"config\"] alone to see it."
	}
}

type rawSection struct {
	log   *StageLog
	kind  string
	lines *[]string
}

func trimLargestRaw(out *Output, excess int) bool {
	var best *rawSection
	bestSize := 0
	for i := range out.Logs {
		sl := &out.Logs[i]
		for kind, lines := range map[string]*[]string{includeHead: &sl.Head, includeTail: &sl.Tail, includeRange: &sl.Range, includeGrep: &sl.Grep} {
			if size := sizeOf(*lines); size > bestSize {
				best, bestSize = &rawSection{log: sl, kind: kind, lines: lines}, size
			}
		}
	}
	if best == nil {
		return false
	}
	lines := *best.lines
	removed, target := 0, excess+200
	for len(lines) > 0 && removed < target {
		if best.kind == includeTail {
			removed += len(lines[0]) + 4
			lines = lines[1:]
		} else {
			removed += len(lines[len(lines)-1]) + 4
			lines = lines[:len(lines)-1]
		}
	}
	*best.lines = lines
	markTruncated(out, best.log, rawHints[best.kind])
	return true
}

func markTruncated(out *Output, sl *StageLog, hint string) {
	sl.Truncated, out.Truncated = true, true
	if !strings.Contains(sl.Hint, hint) {
		sl.Hint = strings.TrimSpace(sl.Hint + " " + hint)
	}
}

func sizeOf(lines []string) int {
	n := 0
	for _, l := range lines {
		n += len(l) + 4
	}
	return n
}

func jsonSize(out *Output) int {
	b, _ := json.Marshal(out)
	return len(b)
}

// ---- small helpers ----

func stageLabel(stage clients.DqPlatformLogType) string {
	if stage == clients.DqPlatformLogDriver {
		return "Stage 3 logs (Spark driver)"
	}
	return "Stage 2 logs (Spark submit)"
}

func anyText(texts map[clients.DqPlatformLogType]string) bool {
	for _, t := range texts {
		if strings.TrimSpace(t) != "" {
			return true
		}
	}
	return false
}

func clamp(n, lo, hi int) int {
	return min(max(n, lo), hi)
}

func notAvailableGuidance(runStatus string) string {
	if clients.IsCancellableDqRunState(runStatus) {
		return "The run is still in progress; stage logs are collected from Edge as the job reports status and " +
			"may not exist yet. Retry once the run finishes. If none appear then, Edge debug logging is likely off " +
			"for the job's Edge capability."
	}
	return "The most likely cause is that Edge debug logging is not enabled on the job's Pullup/Pushdown Edge " +
		"capability — enable it and rerun the job to capture stage logs. Otherwise the logs may have been purged " +
		"after the platform log retention period. The run's exception field (above) and dq_get_job_run may " +
		"still explain a failure."
}

func lookupError(code int, err error, runID, action string) Output {
	out := Output{Status: StatusError, JobRunID: runID}
	switch code {
	case http.StatusNotFound:
		out.Message = fmt.Sprintf("Could not %s %q: not found (HTTP 404).", action, runID)
		out.Guidance = "Verify the run_id — it may be wrong, or the run has since been deleted."
	case http.StatusUnauthorized:
		out.Message = "Not authenticated to the data-quality API (HTTP 401)."
		out.Guidance = "Your Collibra session/token is missing or expired — re-authenticate and retry."
	case http.StatusForbidden:
		out.Message = fmt.Sprintf("You do not have permission to %s %q (HTTP 403).", action, runID)
		out.Guidance = "Reading job logs requires access to the job's logs on its connection. Ask an administrator for the Data Quality Editor/Manager role, then retry."
	case http.StatusBadRequest:
		out.Message = fmt.Sprintf("The data-quality API rejected the request to %s %q (HTTP 400): %v", action, runID, err)
		out.Guidance = "Check that run_id is a well-formed jobRunId (UUID), then retry."
	case 0:
		out.Message = fmt.Sprintf("Failed to %s %q: %v", action, runID, err)
		out.Guidance = "A network/transport error occurred contacting the data-quality API. Retry."
	default:
		out.Message = fmt.Sprintf("Failed to %s %q (HTTP %d): %v", action, runID, code, err)
		out.Guidance = "This is likely a server-side error. Retry shortly; if it persists, contact your Collibra administrator."
	}
	return out
}
