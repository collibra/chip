package publish_agent_scan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// telemetryRoute describes one externalId prefix. The prefix is not a platform
// label: it tells Collibra where the agent's telemetry arrives, so the id has
// the shape <telemetry route>:<instance>:<native id>, route in lowercase. An
// agent built in a git repo and deployed on Kubernetes with OpenTelemetry uses
// "otel:" whatever its platform is. Supporting another route is one more entry
// in telemetryRoutes; the handler does not change.
//
// TODO: "otel:" is deliberately absent. The structure after it has not been
// decided, so ids with that prefix (or any other unknown one) pass through after
// the universal rules in validateExternalID. Add an entry once it is decided.
type telemetryRoute struct {
	prefix string
	// shape is the pattern an externalId with this prefix must match, and
	// shapeHint is the same shape written for a reader.
	shape     *regexp.Regexp
	shapeHint string
	// expectedFrom builds the externalId this route implies from the scan
	// document; false when the document does not carry what is needed.
	expectedFrom func(document json.RawMessage) (string, bool)
}

var telemetryRoutes = []telemetryRoute{
	{
		prefix:       "databricks",
		shape:        regexp.MustCompile(`^databricks:[0-9]+:[^.:\s]+\.[^.:\s]+\.[^.:\s]+$`),
		shapeHint:    "databricks:<workspace id, digits only>:<catalog>.<schema>.<model>",
		expectedFrom: databricksExpectedID,
	},
}

// checkRouteShape returns a message when a known route prefix has the wrong shape.
func checkRouteShape(externalID string) string {
	for _, route := range telemetryRoutes {
		if strings.HasPrefix(externalID, route.prefix+":") && !route.shape.MatchString(externalID) {
			return fmt.Sprintf("externalId %q starts with %q but is not shaped %s. Fix the id, or use a different prefix if the agent's telemetry does not arrive through %s.",
				externalID, route.prefix+":", route.shapeHint, route.prefix)
		}
	}
	return ""
}

// mismatchWarnings only ever warns: a customer may have a reason for a different id.
func mismatchWarnings(externalID string, document json.RawMessage) []string {
	for _, route := range telemetryRoutes {
		if !strings.HasPrefix(externalID, route.prefix+":") {
			continue
		}
		if expected, ok := route.expectedFrom(document); ok && expected != externalID {
			return []string{fmt.Sprintf("externalId %q does not match %q, the id implied by the scan's own %s details. "+
				"It will be sent as given; check with the user that this is intended.", externalID, expected, route.prefix)}
		}
	}
	return nil
}

func databricksExpectedID(document json.RawMessage) (string, bool) {
	name, metadata := documentPlatform(document)
	if name != "DATABRICKS" || metadata["workspaceId"] == "" || metadata["ucModelName"] == "" {
		return "", false
	}
	return "databricks:" + metadata["workspaceId"] + ":" + metadata["ucModelName"], true
}

func documentPlatform(document json.RawMessage) (name string, metadata map[string]string) {
	var doc struct {
		Platform *struct {
			Name     string         `json:"name"`
			Metadata map[string]any `json:"metadata"`
		} `json:"platform"`
	}
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	if decoder.Decode(&doc) != nil || doc.Platform == nil {
		return "", nil
	}
	metadata = map[string]string{}
	for key, value := range doc.Platform.Metadata {
		switch v := value.(type) {
		case string:
			metadata[key] = v
		case json.Number:
			metadata[key] = v.String()
		}
	}
	return doc.Platform.Name, metadata
}
