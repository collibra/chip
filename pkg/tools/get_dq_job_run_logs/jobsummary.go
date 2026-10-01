package get_dq_job_run_logs

import (
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxSparkJobs   = 10
	maxSparkStages = 10
	timeLayout     = "2006-01-02T15:04:05"
)

// JobSummary is what the DRIVER log says about how the pullup job ran. Fields are null when absent.
type JobSummary struct {
	Phases               []Phase       `json:"phases,omitempty" jsonschema:"DQ activities (Load, Schema, Profile, Rules...) with start/end and duration."`
	TotalPhaseSec        *int64        `json:"totalPhaseSec"`
	SparkStages          []SparkStage  `json:"sparkStages,omitempty" jsonschema:"Longest Spark stages (capped), with task counts."`
	SparkStageCount      int           `json:"sparkStageCount"`
	SparkJobs            []SparkJob    `json:"sparkJobs,omitempty" jsonschema:"Longest Spark jobs that logged a duration (capped)."`
	SparkJobCount        int           `json:"sparkJobCount"`
	PartitionNum         *int          `json:"partitionNum"`
	JdbcFetchSize        *int          `json:"jdbcFetchSize"`
	MaxTasksPerStage     *int          `json:"maxTasksPerStage"`
	TaskSetCount         int           `json:"taskSetCount"`
	ExecutorsRegistered  []Executor    `json:"executorsRegistered"`
	DriverStorageMemory  *string       `json:"driverStorageMemory" jsonschema:"Storage memory of the driver's own block manager."`
	ResourceWaitWarnings *ResourceWait `json:"resourceWaitWarnings"`
	RowsLoaded           *int64        `json:"rowsLoaded"`
	CellsLoaded          *int64        `json:"cellsLoaded"`
	SparkUptimeMs        *int64        `json:"sparkUptimeMs"`
	ExitStatus           *string       `json:"exitStatus"`
}

type Phase struct {
	Name        string `json:"name"`
	Status      string `json:"status" jsonschema:"completed | failed | running (no end line)."`
	Start       string `json:"start,omitempty"`
	End         string `json:"end,omitempty"`
	DurationSec *int64 `json:"durationSec"`

	startLine int
	endLine   int
	startTime time.Time
	endTime   time.Time
}

type SparkStage struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Status      string `json:"status"`
	DurationMs  int64  `json:"durationMs"`
	Tasks       *int   `json:"tasks"`
	End         string `json:"end,omitempty"`

	line    int
	endTime time.Time
}

type SparkJob struct {
	ID          int    `json:"id"`
	Description string `json:"description"`
	Status      string `json:"status"`
	TookMs      int64  `json:"tookMs"`
}

type Executor struct {
	ID            string  `json:"id"`
	Host          string  `json:"host"`
	RegisteredAt  string  `json:"registeredAt,omitempty"`
	StorageMemory *string `json:"storageMemory"`

	line         int
	registeredAt time.Time
}

type ResourceWait struct {
	Count   int    `json:"count"`
	FirstAt string `json:"firstAt,omitempty"`
	LastAt  string `json:"lastAt,omitempty"`

	firstLine, lastLine int
	firstTime           time.Time
}

// summaryEvidence keeps the line numbers diagnostics cite.
type summaryEvidence struct {
	partitionLine, rowsLine, firstTaskSetLine int
	firstTaskSetTime                          time.Time
}

var (
	activityLine      = regexp.MustCompile(`(?i)\b(\w+) Activity (started|completed|failed)\b`)
	jobFinishedLine   = regexp.MustCompile(`DAGScheduler: Job (\d+) (finished|failed): (.*), took ([\d.]+) (ms|s)\b`)
	stageFinishedLine = regexp.MustCompile(`DAGScheduler: (ResultStage|ShuffleMapStage) (\d+) \((.*)\) (finished|failed) in ([\d.]+) (ms|s)\b`)
	partitionNumLine  = regexp.MustCompile(`(?i)partitionNum\b.*?:\s*(\d+)`)
	fetchSizeLine     = regexp.MustCompile(`(?i)fetch\s*size to (\d+)`)
	taskSetLine       = regexp.MustCompile(`Adding task set (\d+)\.\d+ with (\d+) tasks`)
	executorLine      = regexp.MustCompile(`Registered executor .*\(([^()]+?)(?::\d+)?\) with ID (\w+)`)
	blockManagerLine  = regexp.MustCompile(`Registering block manager \S+ with ([\d.]+ \w+) RAM, BlockManagerId\(([^,]+),`)
	resourceWaitLine  = regexp.MustCompile(`Initial job has not accepted any resources`)
	cachedRowsLine    = regexp.MustCompile(`(?i)rows:\s*([\d,]+)\s+cells:\s*([\d,]+)`)
	uptimeLine        = regexp.MustCompile(`Successfully stopped SparkContext \(Uptime: (\d+) ms\)`)
	exitStatusLine    = regexp.MustCompile(`(?i)EdgeJob .*with status:\s*(.+?)\s*$`)
)

// parseJobSummary returns nil when nothing recognizable is in the log.
func parseJobSummary(lines []string) (*JobSummary, summaryEvidence) {
	s := &JobSummary{ExecutorsRegistered: []Executor{}}
	var ev summaryEvidence
	phases := map[string]*Phase{}
	var phaseOrder []string
	stageTasks := map[int]int{}
	var stages []SparkStage
	var jobs []SparkJob
	storage := map[string]string{}
	found := false

	for i, line := range lines {
		lineNo := i + 1
		ts, hasTS := parseTimestamp(line)
		switch {
		case activityLine.MatchString(line):
			m := activityLine.FindStringSubmatch(line)
			key := strings.ToLower(m[1])
			p, ok := phases[key]
			if !ok {
				p = &Phase{Name: titleCase(m[1]), Status: "running"}
				phases[key] = p
				phaseOrder = append(phaseOrder, key)
			}
			if strings.EqualFold(m[2], "started") {
				p.startLine, p.startTime = lineNo, ts
			} else {
				p.Status, p.endLine, p.endTime = strings.ToLower(m[2]), lineNo, ts
			}
		case stageFinishedLine.MatchString(line):
			m := stageFinishedLine.FindStringSubmatch(line)
			id, _ := strconv.Atoi(m[2])
			st := SparkStage{ID: id, Type: m[1], Description: redact(m[3]), Status: m[4], DurationMs: toMs(m[5], m[6]), line: lineNo}
			if hasTS {
				st.endTime, st.End = ts, ts.Format(timeLayout)
			}
			stages = append(stages, st)
		case jobFinishedLine.MatchString(line):
			m := jobFinishedLine.FindStringSubmatch(line)
			id, _ := strconv.Atoi(m[1])
			jobs = append(jobs, SparkJob{ID: id, Description: redact(m[3]), Status: m[2], TookMs: toMs(m[4], m[5])})
		case taskSetLine.MatchString(line):
			m := taskSetLine.FindStringSubmatch(line)
			stageID, _ := strconv.Atoi(m[1])
			n, _ := strconv.Atoi(m[2])
			stageTasks[stageID] = n
			s.TaskSetCount++
			if s.MaxTasksPerStage == nil || n > *s.MaxTasksPerStage {
				s.MaxTasksPerStage = &n
			}
			if ev.firstTaskSetLine == 0 {
				ev.firstTaskSetLine, ev.firstTaskSetTime = lineNo, ts
			}
		case executorLine.MatchString(line):
			m := executorLine.FindStringSubmatch(line)
			ex := Executor{ID: m[2], Host: m[1], line: lineNo}
			if hasTS {
				ex.registeredAt, ex.RegisteredAt = ts, ts.Format(timeLayout)
			}
			s.ExecutorsRegistered = append(s.ExecutorsRegistered, ex)
		case blockManagerLine.MatchString(line):
			m := blockManagerLine.FindStringSubmatch(line)
			storage[m[2]] = m[1]
		case resourceWaitLine.MatchString(line):
			if s.ResourceWaitWarnings == nil {
				s.ResourceWaitWarnings = &ResourceWait{firstLine: lineNo, firstTime: ts}
				if hasTS {
					s.ResourceWaitWarnings.FirstAt = ts.Format(timeLayout)
				}
			}
			w := s.ResourceWaitWarnings
			w.Count++
			w.lastLine = lineNo
			if hasTS {
				w.LastAt = ts.Format(timeLayout)
			}
		case partitionNumLine.MatchString(line) && s.PartitionNum == nil:
			s.PartitionNum = atoiPtr(partitionNumLine.FindStringSubmatch(line)[1])
			ev.partitionLine = lineNo
		case fetchSizeLine.MatchString(line) && s.JdbcFetchSize == nil:
			s.JdbcFetchSize = atoiPtr(fetchSizeLine.FindStringSubmatch(line)[1])
		case cachedRowsLine.MatchString(line):
			m := cachedRowsLine.FindStringSubmatch(line)
			s.RowsLoaded, s.CellsLoaded = atoi64Ptr(m[1]), atoi64Ptr(m[2])
			ev.rowsLine = lineNo
		case uptimeLine.MatchString(line):
			s.SparkUptimeMs = atoi64Ptr(uptimeLine.FindStringSubmatch(line)[1])
		case exitStatusLine.MatchString(line):
			v := redact(exitStatusLine.FindStringSubmatch(line)[1])
			s.ExitStatus = &v
		default:
			continue
		}
		found = true
	}
	if !found {
		return nil, ev
	}

	for _, key := range phaseOrder {
		s.Phases = append(s.Phases, finishPhase(*phases[key]))
	}
	s.TotalPhaseSec = totalPhaseSec(s.Phases)
	for i := range stages {
		if n, ok := stageTasks[stages[i].ID]; ok {
			stages[i].Tasks = &n
		}
	}
	s.SparkStageCount, s.SparkStages = len(stages), longestStages(stages)
	s.SparkJobCount, s.SparkJobs = len(jobs), longestJobs(jobs)
	for i := range s.ExecutorsRegistered {
		if mem, ok := storage[s.ExecutorsRegistered[i].ID]; ok {
			s.ExecutorsRegistered[i].StorageMemory = &mem
		}
	}
	if mem, ok := storage["driver"]; ok {
		s.DriverStorageMemory = &mem
	}
	return s, ev
}

func finishPhase(p Phase) Phase {
	if !p.startTime.IsZero() {
		p.Start = p.startTime.Format(timeLayout)
	}
	if !p.endTime.IsZero() {
		p.End = p.endTime.Format(timeLayout)
	}
	if !p.startTime.IsZero() && !p.endTime.IsZero() {
		d := int64(p.endTime.Sub(p.startTime).Seconds())
		p.DurationSec = &d
	}
	return p
}

func totalPhaseSec(phases []Phase) *int64 {
	var first, last time.Time
	for _, p := range phases {
		if !p.startTime.IsZero() && (first.IsZero() || p.startTime.Before(first)) {
			first = p.startTime
		}
		if p.endTime.After(last) {
			last = p.endTime
		}
	}
	if first.IsZero() || last.IsZero() {
		return nil
	}
	d := int64(last.Sub(first).Seconds())
	return &d
}

func longestStages(stages []SparkStage) []SparkStage {
	sorted := append([]SparkStage(nil), stages...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].DurationMs > sorted[j].DurationMs })
	return sorted[:min(len(sorted), maxSparkStages)]
}

func longestJobs(jobs []SparkJob) []SparkJob {
	sorted := append([]SparkJob(nil), jobs...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].TookMs > sorted[j].TookMs })
	return sorted[:min(len(sorted), maxSparkJobs)]
}

func toMs(value, unit string) int64 {
	f, _ := strconv.ParseFloat(value, 64)
	if unit == "s" {
		f *= 1000
	}
	return int64(math.Round(f))
}

func titleCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

func atoiPtr(s string) *int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return &n
}

func atoi64Ptr(s string) *int64 {
	n, err := strconv.ParseInt(strings.ReplaceAll(s, ",", ""), 10, 64)
	if err != nil {
		return nil
	}
	return &n
}
