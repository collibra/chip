// Package get_latest_agent_scan implements the get_latest_agent_scan MCP tool:
// it reads the newest state Collibra holds for one AI Agent asset, so an
// agent-scanning skill can see what is already recorded before it scans again.
package get_latest_agent_scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the outcome of a get_latest_agent_scan call.
type OutputStatus string

const (
	StatusSuccess         OutputStatus = "success"
	StatusNotFound        OutputStatus = "not_found"
	StatusValidationError OutputStatus = "validation_error"
	StatusError           OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	AIAgentID       string `json:"aiAgentId" jsonschema:"Required. UUID of the AI Agent asset (not the agent's external id). Get it from list_scanned_agents."`
	IncludeDocument bool   `json:"includeDocument,omitempty" jsonschema:"Optional. false (default) returns only the summary fields, which is enough to decide whether the code changed since this scan. true also returns the full scan document, which can be large; ask for it only when you need to read or reuse it."`
}

// Source is where the scanned code came from, when the document records it.
type Source struct {
	RepoURL   string `json:"repoUrl,omitempty" jsonschema:"Repository address the scan was made from."`
	CommitSha string `json:"commitSha,omitempty" jsonschema:"Commit the scan was made from. Compare it with the current commit to see whether the agent's code changed."`
}

// Output is the typed response.
type Output struct {
	Status             OutputStatus    `json:"status" jsonschema:"'success'; 'not_found' when Collibra holds no scan for this asset (an answer: scan without a baseline); 'validation_error' for a bad input; 'error' when Collibra could not answer."`
	Message            string          `json:"message" jsonschema:"Plain-language summary that can be relayed to the customer."`
	Guidance           string          `json:"guidance,omitempty" jsonschema:"What to tell or ask the customer next, on error."`
	HTTPStatus         int             `json:"httpStatus,omitempty" jsonschema:"HTTP status of the rejection, when Collibra answered with one."`
	FingerprintID      string          `json:"fingerprintId,omitempty" jsonschema:"Id of this state of the agent."`
	ExternalID         string          `json:"externalId,omitempty" jsonschema:"The agent's identifier in its source system."`
	ContentHash        string          `json:"contentHash,omitempty" jsonschema:"Hash over the document's functional fields. Equal hashes mean a functionally identical agent."`
	FingerprintVersion string          `json:"fingerprintVersion,omitempty" jsonschema:"Schema version of the stored document."`
	ParserVersion      string          `json:"parserVersion,omitempty" jsonschema:"Version of the producer that generated the stored document."`
	ScanCount          int64           `json:"scanCount,omitempty" jsonschema:"How many scans reported this state."`
	FirstSeenOn        string          `json:"firstSeenOn,omitempty" jsonschema:"When the earliest scan of this state was recorded (UTC)."`
	LastSeenOn         string          `json:"lastSeenOn,omitempty" jsonschema:"When the latest scan of this state was recorded (UTC)."`
	Source             *Source         `json:"source,omitempty" jsonschema:"Where the scanned code came from, when the document records it."`
	RootAgent          string          `json:"rootAgent,omitempty" jsonschema:"Name of the root agent in the document."`
	DeploymentVersion  string          `json:"deploymentVersion,omitempty" jsonschema:"The root agent's deployment.version, when there is one. Compare it with what is deployed now to see whether the agent was redeployed."`
	EndpointURL        string          `json:"endpointUrl,omitempty" jsonschema:"The root agent's deployment.endpoint.url, when there is one."`
	EndpointName       string          `json:"endpointName,omitempty" jsonschema:"The root agent's deployment.endpoint.name (for Databricks the serving endpoint name), when there is one."`
	PlatformName       string          `json:"platformName,omitempty" jsonschema:"Platform recorded in the document (for example DATABRICKS), when there is one."`
	AgentCount         int             `json:"agentCount,omitempty" jsonschema:"Number of agents in the document."`
	ToolCount          int             `json:"toolCount,omitempty" jsonschema:"Number of tools in the document."`
	Document           json.RawMessage `json:"document,omitempty" jsonschema:"The full scan document, only when includeDocument was true."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "get_latest_agent_scan",
		Title: "Get Latest Agent Scan",
		Description: "Read the newest agent scan state Collibra holds for one AI Agent asset: its content hash, when it was last confirmed, the commit it was scanned from, and how many agents and tools it has, and where the root agent is deployed (version and endpoint). " +
			"Use it before scanning an agent again, to see what Collibra already has and whether the agent's code changed since (compare source.commitSha with the current commit). " +
			"aiAgentId is the AI Agent asset id, not the external id: get it from list_scanned_agents. " +
			"By default only a small summary comes back; set includeDocument=true to also get the full scan document, which can be large. " +
			"'Newest' means the most recently confirmed state; it is not necessarily the version currently deployed. " +
			"A not_found status is an answer (nothing recorded yet), not a fault. It only reads. " +
			"Note: requires permission to read agent scans (scope aigovernance.guardian.fingerprint-read).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		TypeSchemas: typeSchemas(),
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		aiAgentID := strings.TrimSpace(input.AIAgentID)
		if _, err := uuid.Parse(aiAgentID); err != nil {
			return Output{
				Status:  StatusValidationError,
				Message: fmt.Sprintf("aiAgentId %q is not a valid UUID. Pass the AI Agent asset's UUID (list_scanned_agents returns it), not the agent's external id.", input.AIAgentID),
			}, nil
		}

		fingerprint, err := clients.GetLatestAgentFingerprint(ctx, collibraClient, aiAgentID)
		if err != nil {
			return failure(err), nil
		}
		return success(fingerprint, input.IncludeDocument), nil
	}
}

func success(fingerprint *clients.AgentFingerprint, includeDocument bool) Output {
	out := Output{
		Status:             StatusSuccess,
		FingerprintID:      fingerprint.ID,
		ExternalID:         fingerprint.ExternalID,
		ContentHash:        fingerprint.ContentHash,
		FingerprintVersion: fingerprint.FingerprintVersion,
		ParserVersion:      fingerprint.ParserVersion,
		ScanCount:          fingerprint.ScanCount,
		FirstSeenOn:        fingerprint.FirstSeenOn,
		LastSeenOn:         fingerprint.LastSeenOn,
	}
	summarize(&out, fingerprint.Document)
	out.Message = fmt.Sprintf("Collibra's newest recorded state for this agent (content hash %s) was last confirmed on %s, after %d scan(s).", fingerprint.ContentHash, fingerprint.LastSeenOn, fingerprint.ScanCount)
	if includeDocument {
		out.Document = fingerprint.Document
	}
	return out
}

// summarize lifts the few document fields a caller needs to decide about
// re-scanning, so the document itself does not have to be returned.
func summarize(out *Output, document json.RawMessage) {
	var doc struct {
		Source   *Source `json:"source"`
		Platform *struct {
			Name string `json:"name"`
		} `json:"platform"`
		Agents []struct {
			Name       string `json:"name"`
			Role       string `json:"role"`
			Deployment *struct {
				Version  string `json:"version"`
				Endpoint *struct {
					URL  string `json:"url"`
					Name string `json:"name"`
				} `json:"endpoint"`
			} `json:"deployment"`
		} `json:"agents"`
		Tools []json.RawMessage `json:"tools"`
	}
	if json.Unmarshal(document, &doc) != nil {
		return
	}
	out.Source = doc.Source
	if doc.Platform != nil {
		out.PlatformName = doc.Platform.Name
	}
	out.AgentCount = len(doc.Agents)
	out.ToolCount = len(doc.Tools)
	for _, agent := range doc.Agents {
		if agent.Role == "ROOT" {
			out.RootAgent = agent.Name
			if agent.Deployment != nil {
				out.DeploymentVersion = agent.Deployment.Version
				if agent.Deployment.Endpoint != nil {
					out.EndpointURL = agent.Deployment.Endpoint.URL
					out.EndpointName = agent.Deployment.Endpoint.Name
				}
			}
		}
	}
}

func failure(err error) Output {
	var apiErr *clients.AgentScanAPIError
	if !errors.As(err, &apiErr) {
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("The earlier scan could not be read from Collibra: %v.", err),
			Guidance: "This looks like a network problem. Tell the customer the lookup did not work and carry on without it; trying once more later is fine.",
		}
	}
	message, guidance := apiErr.ReadFailure()
	status := StatusError
	if apiErr.StatusCode == http.StatusNotFound {
		status = StatusNotFound
	}
	return Output{Status: status, Message: message, Guidance: guidance, HTTPStatus: apiErr.StatusCode}
}

// typeSchemas advertises json.RawMessage as an object instead of an integer array.
func typeSchemas() map[reflect.Type]*jsonschema.Schema {
	return map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[json.RawMessage](): {
			Type:        "object",
			Description: "The agent scan document, a JSON object.",
		},
	}
}
