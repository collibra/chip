package main

import (
	"fmt"
	"log/slog"
	"sort"
	"strings"

	"github.com/collibra/chip/pkg/skills"
	"github.com/collibra/chip/pkg/tools"
)

// knownExperimentalFeatures lists every experimental feature chip knows
// about, keyed by the name accepted in `--experimental`, the
// `COLLIBRA_MCP_EXPERIMENTAL` env var, and the `mcp.experimental` YAML
// field. Add an entry here when introducing a new experimental feature;
// the help text, example YAML, and validation warning all read from this
// map so no other code needs to change.
var knownExperimentalFeatures = map[string]string{
	skills.FeatureName:                 "Embedded skill catalog served via list_collibra_skills and load_collibra_skill.",
	tools.ContextSpecificationsFeature: "Context specification tools: list_context_specifications, get_context_specification, and contextSpecificationId parameter on get_asset_details.",
	tools.DataQualityFeatureName:       "Data quality tools: create data quality jobs (create_data_quality_job: discovery + preview + create in one); read a job's full definition (dq_get_job) or a run's full details and per-monitor results (dq_get_job_run); cancel an in-progress job run (dq_cancel_job_run); delete a completed job run and its results (dq_delete_job_run); delete a job definition and everything attached to it (dq_delete_job); partially update an existing job's configuration — schedule, monitors, notifications, scan SQL, compute/sizing and data location (dq_update_job); create, validate, read and search rules; per-run rule results; rule templates (list, read, deploy); plain-language (Text2SQL) rule generation; and catalog column search.",
}

// validateExperimental warns (without exiting) when the user enabled an
// unknown experimental feature. Stale configs from retired or renamed
// features should not break server startup.
func validateExperimental(enabled []string) {
	for _, name := range enabled {
		if _, ok := knownExperimentalFeatures[name]; !ok {
			slog.Warn(fmt.Sprintf(
				"unknown experimental feature %q; known features: %s",
				name, knownExperimentalFeaturesList(),
			))
		}
	}
}

func knownExperimentalFeaturesList() string {
	return strings.Join(sortedKnownExperimentalNames(), ", ")
}

// formatExperimentalForHelp renders the EXPERIMENTAL FEATURES block of
// --help output. Each known feature gets one line: name padded out, then
// its description.
func formatExperimentalForHelp() string {
	var b strings.Builder
	for _, name := range sortedKnownExperimentalNames() {
		fmt.Fprintf(&b, "  %-28s%s\n", name, knownExperimentalFeatures[name])
	}
	return b.String()
}

func sortedKnownExperimentalNames() []string {
	names := make([]string, 0, len(knownExperimentalFeatures))
	for name := range knownExperimentalFeatures {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
