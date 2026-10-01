package get_dq_job_run_logs

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const maxRawSubmitStatement = 1500

// SparkConfig is the Spark/Kubernetes sizing the job was submitted with, parsed from the SUBMIT log.
// Fields are null when the log does not state them; nothing is inferred.
type SparkConfig struct {
	Source                   string            `json:"source" jsonschema:"Where the values came from (submit_log)."`
	DriverCores              *int              `json:"driverCores"`
	DriverMemory             *string           `json:"driverMemory"`
	ExecutorCores            *int              `json:"executorCores"`
	ExecutorMemory           *string           `json:"executorMemory"`
	ExecutorInstances        *int              `json:"executorInstances"`
	ExecutorMemoryOverhead   *string           `json:"executorMemoryOverhead"`
	DynamicAllocationEnabled *bool             `json:"dynamicAllocationEnabled"`
	DynamicAllocationMin     *int              `json:"dynamicAllocationMin"`
	DynamicAllocationMax     *int              `json:"dynamicAllocationMax"`
	K8sResources             K8sResources      `json:"k8sResources"`
	Conf                     map[string]string `json:"conf,omitempty" jsonschema:"Effective spark.* settings (sensitive values masked)."`
	RejectedConfKeys         []string          `json:"rejectedConfKeys,omitempty" jsonschema:"sparkConf keys the Edge launcher dropped because they are not allowlisted."`
	RawSubmitStatement       *string           `json:"rawSubmitStatement" jsonschema:"The Edge launch parameters line (redacted, truncated)."`
}

type K8sResources struct {
	Driver   K8sPodResources `json:"driver"`
	Executor K8sPodResources `json:"executor"`
}

type K8sPodResources struct {
	Requests map[string]string `json:"requests"`
	Limits   map[string]string `json:"limits"`
}

var (
	// The Edge launcher prints "ClusterSparkLauncher - Spark settings:" followed by one key=value per line.
	sparkSettingsHeader = regexp.MustCompile(`Spark settings:\s*$`)
	sparkSettingLine    = regexp.MustCompile(`^\s*(spark\.[\w.\-]+)=(.*)$`)
	// spark-submit --verbose "Parsed arguments:" rows, e.g. "spark-submit:   executorMemory          1024M".
	parsedArgsHeader = regexp.MustCompile(`Parsed arguments:\s*$`)
	parsedArgLine    = regexp.MustCompile(`^(?:spark-submit:)?\s+(\w+)\s+(\S.*)$`)
	sparkTuple       = regexp.MustCompile(`\((spark\.[\w.\-]+),([^)]*)\)`)
	confFlag         = regexp.MustCompile(`--conf\s+(spark\.[\w.\-]+)=(\S+)`)
	rejectedConfKey  = regexp.MustCompile(`Rejecting sparkConf key '([^']+)'`)
	edgeParameters   = regexp.MustCompile(`Edge parameters:\s*(.*)$`)
)

// parsedArgToConf maps spark-submit "Parsed arguments" names to the conf keys they set.
var parsedArgToConf = map[string]string{
	"executorMemory": "spark.executor.memory",
	"executorCores":  "spark.executor.cores",
	"driverMemory":   "spark.driver.memory",
	"driverCores":    "spark.driver.cores",
	"numExecutors":   "spark.executor.instances",
}

// parseSparkConfig returns nil when the log contains no recognizable submit configuration.
func parseSparkConfig(lines []string) *SparkConfig {
	conf := collectConf(lines)
	cfg := &SparkConfig{Source: "submit_log", Conf: map[string]string{}}
	for k, v := range conf {
		cfg.Conf[k] = redactValue(k, v)
	}
	cfg.RejectedConfKeys = rejectedKeys(lines)
	cfg.RawSubmitStatement = rawSubmitStatement(lines)
	if len(conf) == 0 && cfg.RawSubmitStatement == nil {
		return nil
	}

	cfg.DriverCores = intConf(conf, "spark.driver.cores")
	cfg.DriverMemory = strConf(conf, "spark.driver.memory")
	cfg.ExecutorCores = intConf(conf, "spark.executor.cores")
	cfg.ExecutorMemory = strConf(conf, "spark.executor.memory")
	cfg.ExecutorInstances = intConf(conf, "spark.executor.instances")
	cfg.ExecutorMemoryOverhead = strConf(conf, "spark.executor.memoryOverhead")
	cfg.DynamicAllocationEnabled = boolConf(conf, "spark.dynamicAllocation.enabled")
	cfg.DynamicAllocationMin = intConf(conf, "spark.dynamicAllocation.minExecutors")
	cfg.DynamicAllocationMax = intConf(conf, "spark.dynamicAllocation.maxExecutors")
	cfg.K8sResources = K8sResources{Driver: podResources(conf, "driver"), Executor: podResources(conf, "executor")}
	return cfg
}

// collectConf merges every source of spark.* settings. The Edge "Spark settings" block wins because it
// has real values; spark-submit's own output is usually masked by spark.redaction.regex.
func collectConf(lines []string) map[string]string {
	conf := map[string]string{}
	setIfAbsent := func(k, v string) {
		v = strings.TrimSpace(v)
		if _, ok := conf[k]; !ok && v != "" && !strings.Contains(v, "(redacted") {
			conf[k] = v
		}
	}
	for i := 0; i < len(lines); i++ {
		switch {
		case sparkSettingsHeader.MatchString(lines[i]):
			for i+1 < len(lines) {
				m := sparkSettingLine.FindStringSubmatch(lines[i+1])
				if m == nil {
					break
				}
				conf[m[1]] = strings.TrimSpace(m[2])
				i++
			}
		case parsedArgsHeader.MatchString(lines[i]):
			for i+1 < len(lines) {
				m := parsedArgLine.FindStringSubmatch(lines[i+1])
				if m == nil {
					break
				}
				if key, ok := parsedArgToConf[m[1]]; ok && m[2] != "null" {
					setIfAbsent(key, m[2])
				}
				i++
			}
		default:
			for _, m := range sparkTuple.FindAllStringSubmatch(lines[i], -1) {
				setIfAbsent(m[1], m[2])
			}
			for _, m := range confFlag.FindAllStringSubmatch(lines[i], -1) {
				setIfAbsent(m[1], m[2])
			}
		}
	}
	return conf
}

func rejectedKeys(lines []string) []string {
	seen := map[string]bool{}
	var keys []string
	for _, l := range lines {
		if m := rejectedConfKey.FindStringSubmatch(l); m != nil && !seen[m[1]] {
			seen[m[1]] = true
			keys = append(keys, m[1])
		}
	}
	sort.Strings(keys)
	return keys
}

func rawSubmitStatement(lines []string) *string {
	for _, l := range lines {
		if m := edgeParameters.FindStringSubmatch(l); m != nil {
			s := redact(m[1])
			if len(s) > maxRawSubmitStatement {
				s = s[:maxRawSubmitStatement] + "…"
			}
			return &s
		}
	}
	return nil
}

func podResources(conf map[string]string, role string) K8sPodResources {
	res := K8sPodResources{Requests: map[string]string{}, Limits: map[string]string{}}
	prefix := "spark.kubernetes." + role + "."
	for kind, target := range map[string]map[string]string{"request": res.Requests, "limit": res.Limits} {
		if v, ok := conf[prefix+kind+".cores"]; ok {
			target["cpu"] = v
		}
		if v, ok := conf[prefix+kind+".memory"]; ok {
			target["memory"] = v
		}
	}
	return res
}

func strConf(conf map[string]string, key string) *string {
	if v, ok := conf[key]; ok {
		return &v
	}
	return nil
}

func intConf(conf map[string]string, key string) *int {
	if n, err := strconv.Atoi(conf[key]); err == nil {
		return &n
	}
	return nil
}

func boolConf(conf map[string]string, key string) *bool {
	if b, err := strconv.ParseBool(conf[key]); err == nil {
		return &b
	}
	return nil
}
