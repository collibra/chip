package get_dq_job_run_logs_test

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/get_dq_job_run_logs"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const runID = "a5c7e396-2ffe-42f8-864d-92dfdb35a83d"

// handlers configures the mocked endpoints. A nil handler means "must not be called".
type handlers struct {
	run  func(w http.ResponseWriter, r *http.Request)            // GET /rest/dq/1.0/jobRuns/{id}
	job  func(w http.ResponseWriter, r *http.Request)            // GET /rest/dq/1.0/jobs/{name}
	logs map[string]func(w http.ResponseWriter, r *http.Request) // GET .../edge/platformLogs?type=<key>
}

func newServer(t *testing.T, h handlers) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	route := func(name string, fn func(http.ResponseWriter, *http.Request)) func(http.ResponseWriter, *http.Request) {
		return func(w http.ResponseWriter, r *http.Request) {
			if fn == nil {
				t.Errorf("unexpected %s call: %s %s", name, r.Method, r.URL)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			fn(w, r)
		}
	}
	mux.HandleFunc("/rest/dq/1.0/jobRuns/", route("run", h.run))
	mux.HandleFunc("/rest/dq/1.0/jobs/", route("job", h.job))
	mux.HandleFunc("/rest/dq/internal/v1/job/", func(w http.ResponseWriter, r *http.Request) {
		logType := r.URL.Query().Get("type")
		route("platformLogs "+logType, h.logs[logType])(w, r)
	})
	return httptest.NewServer(mux)
}

func jsonHandler(code int, body any) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if body != nil {
			_ = json.NewEncoder(w).Encode(body)
		}
	}
}

func textHandler(body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(body))
	}
}

// jobRun mirrors the public JobRun, which does not carry jobType.
func jobRun(status string) map[string]any {
	return map[string]any{
		"jobRunId":             runID,
		"jobName":              "public.iowa_liquor_sales",
		"status":               status,
		"executionTimeSeconds": 4508,
		"executedQuery":        `SELECT * FROM "public"."iowa_liquor_sales"`,
	}
}

func jobDef(jobType string) func(http.ResponseWriter, *http.Request) {
	return jsonHandler(http.StatusOK, map[string]any{"jobName": "public.iowa_liquor_sales", "jobType": jobType})
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	f, err := os.Open("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// realRunServer serves the recorded logs of run a5c7e396 (public.iowa_liquor_sales, PULLUP, 75 min).
func realRunServer(t *testing.T) *httptest.Server {
	return newServer(t, handlers{
		run: jsonHandler(http.StatusOK, jobRun("FINISHED")),
		job: jobDef("PULLUP"),
		logs: map[string]func(http.ResponseWriter, *http.Request){
			"SUBMIT": textHandler(fixture(t, "a5c7e396_submit.log.gz")),
			"DRIVER": textHandler(fixture(t, "a5c7e396_driver.log.gz")),
		},
	})
}

func run(t *testing.T, server *httptest.Server, in tools.Input) tools.Output {
	t.Helper()
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("handler returned a Go error (should surface via Output): %v", err)
	}
	return out
}

func responseSize(t *testing.T, out tools.Output) int {
	t.Helper()
	b, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	return len(b)
}

func logFor(t *testing.T, out tools.Output, stage string) tools.StageLog {
	t.Helper()
	for _, l := range out.Logs {
		if l.Stage == stage {
			return l
		}
	}
	t.Fatalf("no %s log in output", stage)
	return tools.StageLog{}
}

func deref[T any](p *T) any {
	if p == nil {
		return nil
	}
	return *p
}

// ---- input validation ----

func TestInputValidation(t *testing.T) {
	cases := map[string]tools.Input{
		"missing run_id":         {},
		"unknown stage":          {RunID: runID, Stage: "executor"},
		"unknown include":        {RunID: runID, Include: []string{"everything"}},
		"invalid grep":           {RunID: runID, Grep: "("},
		"grep include, no regex": {RunID: runID, Include: []string{"grep"}},
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			server := newServer(t, handlers{})
			defer server.Close()
			if out := run(t, server, in); out.Status != tools.StatusNeedsInput {
				t.Fatalf("expected needs_input, got %q (%s)", out.Status, out.Message)
			}
		})
	}
}

// ---- real run fixtures ----

func TestRealRunDefaultCallIsSmallAndParsed(t *testing.T) {
	server := realRunServer(t)
	defer server.Close()

	out := run(t, server, tools.Input{RunID: runID})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("expected success, got %q (%s)", out.Status, out.Message)
	}
	if size := responseSize(t, out); size >= 15_000 {
		t.Errorf("default response is %d chars, want < 15000", size)
	}
	if out.JobType != "PULLUP" {
		t.Errorf("expected jobType resolved from the job definition, got %q", out.JobType)
	}
	if out.SparkConfig == nil || out.JobSummary == nil || len(out.Diagnostics) == 0 {
		t.Fatalf("expected sparkConfig, jobSummary and diagnostics by default, got %+v", out)
	}
	submit := logFor(t, out, "SUBMIT")
	if submit.TotalLines != 6429 || submit.TotalBytes < 1_000_000 {
		t.Errorf("expected full submit log size reported, got %d lines / %d bytes", submit.TotalLines, submit.TotalBytes)
	}
	if submit.CollapsedLines < 4000 {
		t.Errorf("expected the per-second pod status line to collapse, got collapsedLines=%d", submit.CollapsedLines)
	}
	if len(submit.Head)+len(submit.Tail)+len(submit.Grep)+len(submit.Range) != 0 {
		t.Error("expected no raw sections by default")
	}
	if out.Hint == "" {
		t.Error("expected a follow-up hint")
	}
}

func TestRealRunSparkConfig(t *testing.T) {
	server := realRunServer(t)
	defer server.Close()

	cfg := run(t, server, tools.Input{RunID: runID}).SparkConfig
	checks := map[string][2]any{
		"driverMemory":             {deref(cfg.DriverMemory), "1024M"},
		"executorMemory":           {deref(cfg.ExecutorMemory), "1024M"},
		"executorInstances":        {deref(cfg.ExecutorInstances), 1},
		"executorMemoryOverhead":   {deref(cfg.ExecutorMemoryOverhead), "1024M"},
		"driverCores":              {deref(cfg.DriverCores), nil},
		"executorCores":            {deref(cfg.ExecutorCores), nil},
		"dynamicAllocationEnabled": {deref(cfg.DynamicAllocationEnabled), nil},
		"driver request cpu":       {cfg.K8sResources.Driver.Requests["cpu"], "1000m"},
		"driver limit memory":      {cfg.K8sResources.Driver.Limits["memory"], "2548M"},
		"executor request memory":  {cfg.K8sResources.Executor.Requests["memory"], "2048M"},
		"executor limit cpu":       {cfg.K8sResources.Executor.Limits["cpu"], "1000m"},
		"pullSecrets masked":       {cfg.Conf["spark.kubernetes.container.image.pullSecrets"], "***"},
		"namespace kept":           {cfg.Conf["spark.kubernetes.namespace"], "collibra-edge"},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s: got %v, want %v", name, c[0], c[1])
		}
	}
	for k, v := range cfg.Conf {
		if strings.Contains(v, "redacted") {
			t.Errorf("conf %s carries spark-submit's masked placeholder %q instead of being omitted", k, v)
		}
	}
	if len(cfg.RejectedConfKeys) != 3 {
		t.Errorf("expected 3 rejected sparkConf keys, got %v", cfg.RejectedConfKeys)
	}
	if cfg.RawSubmitStatement == nil || !strings.Contains(*cfg.RawSubmitStatement, "sparkExecutorInstances=1") {
		t.Errorf("expected the Edge launch parameters as rawSubmitStatement, got %v", deref(cfg.RawSubmitStatement))
	}
}

func TestRealRunJobSummary(t *testing.T) {
	server := realRunServer(t)
	defer server.Close()

	s := run(t, server, tools.Input{RunID: runID}).JobSummary
	checks := map[string][2]any{
		"partitionNum":        {deref(s.PartitionNum), 1},
		"jdbcFetchSize":       {deref(s.JdbcFetchSize), 6000},
		"maxTasksPerStage":    {deref(s.MaxTasksPerStage), 1},
		"executors":           {len(s.ExecutorsRegistered), 1},
		"rowsLoaded":          {deref(s.RowsLoaded), int64(18_766_068)},
		"cellsLoaded":         {deref(s.CellsLoaded), int64(487_917_768)},
		"sparkUptimeMs":       {deref(s.SparkUptimeMs), int64(4_557_254)},
		"exitStatus":          {deref(s.ExitStatus), "0 (SUCCESS)"},
		"driverStorageMemory": {deref(s.DriverStorageMemory), "413.9 MiB"},
		"resource warnings":   {s.ResourceWaitWarnings.Count, 3},
		"longest stage":       {s.SparkStages[0].ID, 0},
		"longest stage ms":    {s.SparkStages[0].DurationMs, int64(2_306_556)},
		"longest job":         {s.SparkJobs[0].ID, 4},
	}
	for name, c := range checks {
		if c[0] != c[1] {
			t.Errorf("%s: got %v, want %v", name, c[0], c[1])
		}
	}
	ex := s.ExecutorsRegistered[0]
	if ex.ID != "1" || ex.Host != "172.20.62.97" || deref(ex.StorageMemory) != "413.9 MiB" {
		t.Errorf("unexpected executor: %+v (storage %v)", ex, deref(ex.StorageMemory))
	}

	want := map[string][2]int64{"Load": {2280, 2400}, "Schema": {1020, 1140}, "Profile": {1020, 1140}} // ≈39, 18, 18 min
	for _, p := range s.Phases {
		if r, ok := want[p.Name]; ok {
			if p.DurationSec == nil || *p.DurationSec < r[0] || *p.DurationSec > r[1] || p.Status != "completed" {
				t.Errorf("phase %s: got %v s (%s), want %d–%d s", p.Name, deref(p.DurationSec), p.Status, r[0], r[1])
			}
			delete(want, p.Name)
		}
	}
	if len(want) != 0 {
		t.Errorf("missing phases: %v", want)
	}
}

func TestRealRunDiagnostics(t *testing.T) {
	server := realRunServer(t)
	defer server.Close()

	got := map[string]tools.Diagnostic{}
	for _, d := range run(t, server, tools.Input{RunID: runID}).Diagnostics {
		got[d.Code] = d
	}
	for _, code := range []string{"single_partition_read", "single_executor", "no_parallelism", "resource_wait", "dominant_phase", "full_table_scan"} {
		d, ok := got[code]
		if !ok {
			t.Errorf("expected diagnostic %s", code)
			continue
		}
		if len(d.Evidence) == 0 {
			t.Errorf("diagnostic %s has no evidence", code)
		}
	}
	if d := got["dominant_phase"]; !strings.Contains(d.Message, "Load") || !strings.Contains(d.Message, "Load.scala:955") {
		t.Errorf("expected Load and its count stage named, got %q", d.Message)
	}
}

func TestRealRunTailIsCollapsedAndBudgeted(t *testing.T) {
	server := realRunServer(t)
	defer server.Close()

	out := run(t, server, tools.Input{RunID: runID, TailLines: 2000})
	if size := responseSize(t, out); size > 15_000 {
		t.Errorf("response is %d chars, want <= 15000", size)
	}
	submit := logFor(t, out, "SUBMIT")
	if !submit.Truncated || !strings.Contains(submit.Hint, "offset/limit") && !strings.Contains(logFor(t, out, "DRIVER").Hint, "offset/limit") {
		t.Errorf("expected truncation with an offset/limit hint, got %+v / %+v", submit.Hint, logFor(t, out, "DRIVER").Hint)
	}

	uncut := run(t, server, tools.Input{RunID: runID, Stage: "SUBMIT", Include: []string{"tail"}, TailLines: 30})
	if !strings.Contains(strings.Join(logFor(t, uncut, "SUBMIT").Tail, "\n"), "[repeated") {
		t.Error("expected the collapsed pod status line in the submit tail")
	}
}

func TestRealRunCollapseOff(t *testing.T) {
	server := realRunServer(t)
	defer server.Close()

	out := run(t, server, tools.Input{RunID: runID, Stage: "SUBMIT", Include: []string{"errors"}, CollapseRepeats: new(bool)})
	if got := logFor(t, out, "SUBMIT").CollapsedLines; got != 0 {
		t.Errorf("expected no collapsing with collapse_repeats=false, got %d", got)
	}
}

func TestRealRunGrepAndRange(t *testing.T) {
	server := realRunServer(t)
	defer server.Close()

	ctx := 1
	out := run(t, server, tools.Input{RunID: runID, Stage: "DRIVER", Include: []string{"errors"}, Grep: "Registered executor", GrepContext: &ctx})
	driver := logFor(t, out, "DRIVER")
	if deref(driver.GrepTotalMatches) != 1 || len(driver.Grep) != 1 {
		t.Fatalf("expected one grep match, got %v / %v", deref(driver.GrepTotalMatches), driver.Grep)
	}
	if lines := strings.Split(driver.Grep[0], "\n"); len(lines) != 3 || !strings.HasPrefix(lines[1], "1792> ") {
		t.Errorf("expected match at line 1792 with 1 line of context, got %q", driver.Grep[0])
	}

	out = run(t, server, tools.Input{RunID: runID, Stage: "DRIVER", Include: []string{"errors"}, Offset: 1755, Limit: 3})
	rng := logFor(t, out, "DRIVER").Range
	if len(rng) != 3 || !strings.HasPrefix(rng[0], "1756| ") || !strings.Contains(rng[0], "Load Activity started") {
		t.Errorf("expected lines 1756-1758, got %q", rng)
	}
}

// ---- synthetic multi-executor fixture ----

func TestMultiExecutorMultiPartition(t *testing.T) {
	driver := strings.Join([]string{
		"26/01/02 10:00:00 INFO PullupEdgeJob: Load Activity started",
		"26/01/02 10:00:01 INFO Load: Setting default fetchsize to 10000",
		"26/01/02 10:00:02 INFO Load: partitionNum sent from OwlOptions is : 8",
		"26/01/02 10:00:03 INFO TaskSchedulerImpl: Adding task set 0.0 with 8 tasks resource profile 0",
		"26/01/02 10:00:04 INFO KubernetesClusterSchedulerBackend$KubernetesDriverEndpoint: Registered executor NettyRpcEndpointRef(spark-client://Executor) (10.0.0.1:40000) with ID 1, ResourceProfileId 0",
		"26/01/02 10:00:04 INFO KubernetesClusterSchedulerBackend$KubernetesDriverEndpoint: Registered executor NettyRpcEndpointRef(spark-client://Executor) (10.0.0.2:40000) with ID 2, ResourceProfileId 0",
		"26/01/02 10:00:04 INFO KubernetesClusterSchedulerBackend$KubernetesDriverEndpoint: Registered executor NettyRpcEndpointRef(spark-client://Executor) (10.0.0.3:40000) with ID 3, ResourceProfileId 0",
		"26/01/02 10:00:05 INFO BlockManagerMasterEndpoint: Registering block manager 10.0.0.2:41000 with 2.1 GiB RAM, BlockManagerId(2, 10.0.0.2, 41000, None)",
		"26/01/02 10:05:00 INFO DAGScheduler: ResultStage 0 (count at Load.scala:955) finished in 295000 ms",
		"26/01/02 10:05:00 INFO Load: CACHING: Loaded, cached [ rows: 5,000,000 cells: 50,000,000 ]",
		"26/01/02 10:05:01 INFO PullupEdgeJob: LOAD Activity completed",
		"26/01/02 10:05:01 INFO PullupEdgeJob: Profile Activity started",
		"26/01/02 10:05:02 INFO TaskSchedulerImpl: Adding task set 1.0 with 8 tasks resource profile 0",
		"26/01/02 10:10:01 INFO PullupEdgeJob: PROFILE Activity completed",
	}, "\n")
	server := newServer(t, handlers{
		run: jsonHandler(http.StatusOK, map[string]any{"jobRunId": runID, "jobName": "j", "jobType": "PULLUP", "status": "FINISHED",
			"executedQuery": "SELECT * FROM t WHERE d = '2026-01-02'"}),
		logs: map[string]func(http.ResponseWriter, *http.Request){"SUBMIT": textHandler(""), "DRIVER": textHandler(driver)},
	})
	defer server.Close()

	out := run(t, server, tools.Input{RunID: runID})
	s := out.JobSummary
	if deref(s.PartitionNum) != 8 || deref(s.MaxTasksPerStage) != 8 || len(s.ExecutorsRegistered) != 3 || deref(s.JdbcFetchSize) != 10000 {
		t.Fatalf("unexpected summary: partitions=%v tasks=%v executors=%d fetch=%v",
			deref(s.PartitionNum), deref(s.MaxTasksPerStage), len(s.ExecutorsRegistered), deref(s.JdbcFetchSize))
	}
	if deref(s.ExecutorsRegistered[1].StorageMemory) != "2.1 GiB" || s.ExecutorsRegistered[0].StorageMemory != nil {
		t.Errorf("expected block manager storage mapped to executor 2 only, got %+v", s.ExecutorsRegistered)
	}
	for _, d := range out.Diagnostics {
		switch d.Code {
		case "single_partition_read", "single_executor", "no_parallelism", "resource_wait", "full_table_scan":
			t.Errorf("unexpected diagnostic %s: %s", d.Code, d.Message)
		}
	}
	if out.SparkConfig != nil {
		t.Errorf("expected no sparkConfig from an empty submit log, got %+v", out.SparkConfig)
	}
}

// ---- pushdown ----

func TestPushdownReadsOnlySubmitAndHasNoSummary(t *testing.T) {
	server := newServer(t, handlers{
		run:  jsonHandler(http.StatusOK, jobRun("FAILED")),
		job:  jobDef("PUSHDOWN"),
		logs: map[string]func(http.ResponseWriter, *http.Request){"SUBMIT": textHandler("2026-01-02 10:00:00 ERROR c.c.Pushdown - Query failed: timeout\n")},
	})
	defer server.Close()

	out := run(t, server, tools.Input{RunID: runID})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("expected success, got %q (%s)", out.Status, out.Message)
	}
	if len(out.Logs) != 1 || out.Logs[0].Stage != "SUBMIT" || out.JobSummary != nil {
		t.Errorf("expected only SUBMIT and no jobSummary, got logs=%+v summary=%+v", out.Logs, out.JobSummary)
	}
	if len(out.Logs[0].ErrorLines) != 1 {
		t.Errorf("expected the pushdown error line, got %v", out.Logs[0].ErrorLines)
	}
}

func TestDriverStageOnPushdownNeedsInput(t *testing.T) {
	server := newServer(t, handlers{run: jsonHandler(http.StatusOK, jobRun("FAILED")), job: jobDef("PUSHDOWN")})
	defer server.Close()

	if out := run(t, server, tools.Input{RunID: runID, Stage: "driver"}); out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input, got %q (%s)", out.Status, out.Message)
	}
}

// ---- logs not stored ----

func TestEmptyLogsNotAvailable(t *testing.T) {
	for status, want := range map[string]string{"FAILED": "debug logging", "RUNNING": "in progress"} {
		t.Run(status, func(t *testing.T) {
			server := newServer(t, handlers{
				run:  jsonHandler(http.StatusOK, jobRun(status)),
				job:  jobDef("PULLUP"),
				logs: map[string]func(http.ResponseWriter, *http.Request){"SUBMIT": textHandler(""), "DRIVER": textHandler("")},
			})
			defer server.Close()

			out := run(t, server, tools.Input{RunID: runID})
			if out.Status != tools.StatusNotAvailable || !strings.Contains(out.Guidance, want) {
				t.Fatalf("expected not_available mentioning %q, got %q (%s)", want, out.Status, out.Guidance)
			}
		})
	}
}

// ---- errors ----

func TestRunLookupErrorMapping(t *testing.T) {
	for _, code := range []int{http.StatusNotFound, http.StatusUnauthorized, http.StatusForbidden, http.StatusBadRequest, http.StatusInternalServerError} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			server := newServer(t, handlers{run: jsonHandler(code, map[string]any{"message": "boom"})})
			defer server.Close()

			out := run(t, server, tools.Input{RunID: runID})
			if out.Status != tools.StatusError || !strings.Contains(out.Message, fmt.Sprint(code)) {
				t.Errorf("expected error mentioning %d, got %q (%s)", code, out.Status, out.Message)
			}
		})
	}
}

func TestLogFetchForbidden(t *testing.T) {
	server := newServer(t, handlers{
		run:  jsonHandler(http.StatusOK, jobRun("FAILED")),
		job:  jobDef("PULLUP"),
		logs: map[string]func(http.ResponseWriter, *http.Request){"SUBMIT": jsonHandler(http.StatusForbidden, map[string]any{"message": "no"})},
	})
	defer server.Close()

	out := run(t, server, tools.Input{RunID: runID})
	if out.Status != tools.StatusError || !strings.Contains(out.Message, "403") || out.JobName == "" {
		t.Fatalf("expected 403 error with run context, got %q (%s) %+v", out.Status, out.Message, out)
	}
}

func TestJobLookupFailureStillReadsBothStages(t *testing.T) {
	server := newServer(t, handlers{
		run: jsonHandler(http.StatusOK, jobRun("FINISHED")),
		job: jsonHandler(http.StatusForbidden, nil),
		logs: map[string]func(http.ResponseWriter, *http.Request){
			"SUBMIT": textHandler("INFO a\n"),
			"DRIVER": textHandler("INFO b\n"),
		},
	})
	defer server.Close()

	if out := run(t, server, tools.Input{RunID: runID}); out.Status != tools.StatusSuccess || len(out.Logs) != 2 {
		t.Fatalf("expected both stages read when job type is unknown, got %q %+v", out.Status, out.Logs)
	}
}

func TestTransportError(t *testing.T) {
	server := newServer(t, handlers{})
	server.Close()

	if out := run(t, server, tools.Input{RunID: runID}); out.Status != tools.StatusError {
		t.Fatalf("expected error on transport failure, got %q (%s)", out.Status, out.Message)
	}
}
