package get_dq_job_run_logs

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const (
	manyRows            = 1_000_000
	dominantPhaseShare  = 0.5
	resourceWaitPeriodS = 15 // Spark repeats the "not accepted any resources" warning every 15s
)

// Diagnostic is a rule-based hint with the log lines that triggered it.
type Diagnostic struct {
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	Evidence []string `json:"evidence,omitempty"`
}

var whereClause = regexp.MustCompile(`(?i)\bwhere\b`)

// diagnose applies simple rules to the parsed summary/config. It never calls a model.
func diagnose(s *JobSummary, ev summaryEvidence, cfg *SparkConfig, driver []string, query string) []Diagnostic {
	var out []Diagnostic
	add := func(d *Diagnostic) {
		if d != nil {
			out = append(out, *d)
		}
	}
	if s != nil {
		add(singlePartitionRead(s, ev, driver))
		add(singleExecutor(s, cfg, driver))
		add(noParallelism(s, ev, driver))
		add(resourceWait(s, ev, driver))
		add(dominantPhase(s, driver))
	}
	add(fullTableScan(query))
	return out
}

func singlePartitionRead(s *JobSummary, ev summaryEvidence, driver []string) *Diagnostic {
	if s.PartitionNum == nil || *s.PartitionNum != 1 || s.RowsLoaded == nil || *s.RowsLoaded <= manyRows {
		return nil
	}
	return &Diagnostic{
		Code: "single_partition_read",
		Message: fmt.Sprintf("Single-partition read of %s rows: the JDBC load ran as one task. Configure a partition column / numPartitions (parallel JDBC) on the job.",
			commas(*s.RowsLoaded)),
		Evidence: lineEvidence(driver, ev.partitionLine, ev.rowsLine),
	}
}

func singleExecutor(s *JobSummary, cfg *SparkConfig, driver []string) *Diagnostic {
	if len(s.ExecutorsRegistered) != 1 {
		return nil
	}
	msg := "Only one executor registered, so at most one executor's cores did the work."
	if cfg != nil && cfg.ExecutorInstances != nil {
		msg += fmt.Sprintf(" The job requested spark.executor.instances=%d.", *cfg.ExecutorInstances)
	}
	return &Diagnostic{Code: "single_executor", Message: msg, Evidence: lineEvidence(driver, s.ExecutorsRegistered[0].line)}
}

func noParallelism(s *JobSummary, ev summaryEvidence, driver []string) *Diagnostic {
	if s.MaxTasksPerStage == nil || *s.MaxTasksPerStage != 1 || s.TaskSetCount == 0 {
		return nil
	}
	return &Diagnostic{
		Code:     "no_parallelism",
		Message:  fmt.Sprintf("No parallelism: all %d Spark task sets ran with a single task.", s.TaskSetCount),
		Evidence: lineEvidence(driver, ev.firstTaskSetLine),
	}
}

// resourceWait measures from the first task set to the first executor registration when both are
// logged, and otherwise estimates from the warning count.
func resourceWait(s *JobSummary, ev summaryEvidence, driver []string) *Diagnostic {
	w := s.ResourceWaitWarnings
	if w == nil || w.Count == 0 {
		return nil
	}
	waitSec := int64(w.Count * resourceWaitPeriodS)
	if len(s.ExecutorsRegistered) > 0 && !ev.firstTaskSetTime.IsZero() {
		if reg := s.ExecutorsRegistered[0].registeredAt; reg.After(ev.firstTaskSetTime) {
			waitSec = int64(reg.Sub(ev.firstTaskSetTime).Seconds())
		}
	}
	return &Diagnostic{
		Code: "resource_wait",
		Message: fmt.Sprintf("Waited ~%ds for executor resources (%d 'not accepted any resources' warnings) before tasks could run; the cluster lacked capacity for the requested executor.",
			waitSec, w.Count),
		Evidence: lineEvidence(driver, w.firstLine, w.lastLine),
	}
}

func dominantPhase(s *JobSummary, driver []string) *Diagnostic {
	if s.TotalPhaseSec == nil || *s.TotalPhaseSec <= 0 {
		return nil
	}
	total := *s.TotalPhaseSec
	for _, p := range s.Phases {
		if p.DurationSec == nil || float64(*p.DurationSec) <= dominantPhaseShare*float64(total) {
			continue
		}
		msg := fmt.Sprintf("%s took %s (%d%% of %s across all phases).", p.Name, humanSec(*p.DurationSec),
			*p.DurationSec*100/total, humanSec(total))
		lines := []int{p.startLine, p.endLine}
		if st := longestStageIn(s.SparkStages, p); st != nil {
			msg += fmt.Sprintf(" Most of it was Spark stage %d (%s, %s).", st.ID, st.Description, humanSec(st.DurationMs/1000))
			lines = append(lines, st.line)
		}
		return &Diagnostic{Code: "dominant_phase", Message: msg, Evidence: lineEvidence(driver, lines...)}
	}
	return nil
}

func longestStageIn(stages []SparkStage, p Phase) *SparkStage {
	var best *SparkStage
	for i := range stages {
		st := &stages[i]
		if st.endTime.IsZero() || st.endTime.Before(p.startTime) || (!p.endTime.IsZero() && st.endTime.After(p.endTime)) {
			continue
		}
		if best == nil || st.DurationMs > best.DurationMs {
			best = st
		}
	}
	return best
}

func fullTableScan(query string) *Diagnostic {
	query = strings.TrimSpace(query)
	if query == "" || whereClause.MatchString(query) {
		return nil
	}
	return &Diagnostic{
		Code:     "full_table_scan",
		Message:  "The source query has no WHERE clause, so every run scans the full table. Consider filtering on a date column with ${rd}.",
		Evidence: []string{"query: " + truncateLine(redact(query))},
	}
}

func lineEvidence(lines []string, lineNos ...int) []string {
	var out []string
	seen := map[int]bool{}
	for _, n := range lineNos {
		if n <= 0 || n > len(lines) || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, evidence("DRIVER", n, lines[n-1]))
	}
	return out
}

func humanSec(sec int64) string {
	return (time.Duration(sec) * time.Second).String()
}
