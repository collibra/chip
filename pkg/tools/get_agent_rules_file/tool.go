// Package get_agent_rules_file implements the get_agent_rules_file MCP tool:
// it reads the metadata of an AI agent's latest rules file (enforcement
// bundle) and a short-lived download link for it, so the assistant can fetch
// the file on the customer's machine and check it. The tool does not download
// the file itself: chip may run remotely, and the file belongs with the
// customer. The link is a secret: it is never logged.
package get_agent_rules_file

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the outcome of a get_agent_rules_file call.
type OutputStatus string

const (
	StatusSuccess         OutputStatus = "success"
	StatusNotFound        OutputStatus = "not_found"
	StatusValidationError OutputStatus = "validation_error"
	StatusError           OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	AIAgentID string `json:"aiAgentId" jsonschema:"Required. UUID of the AI Agent asset (the agent's entry in the Collibra catalog), not the agent's external id. Get it from list_scanned_agents."`
}

// Output is the typed response.
type Output struct {
	Status         OutputStatus `json:"status" jsonschema:"'success'; 'not_found' when the agent has no rules file yet (errorCode noBundleAvailable) or Guardian has no record of the agent (aiAgentNotFound); 'validation_error' for a bad input (nothing called); 'error' when Collibra could not answer."`
	Message        string       `json:"message" jsonschema:"Plain-language summary that can be relayed to the customer. It never contains the download link."`
	Guidance       string       `json:"guidance,omitempty" jsonschema:"What to tell or ask the customer next, on error or not_found."`
	ErrorCode      string       `json:"errorCode,omitempty" jsonschema:"Machine-readable error code from Collibra (e.g. 'noBundleAvailable'), when the request was rejected."`
	HTTPStatus     int          `json:"httpStatus,omitempty" jsonschema:"HTTP status of the rejection, when Collibra answered with one."`
	BundleID       string       `json:"bundleId,omitempty" jsonschema:"Id of the rules file (enforcement bundle)."`
	Version        int64        `json:"version,omitempty" jsonschema:"The rules file's version, one more for each rules file of the same agent version."`
	AgentVersionID string       `json:"agentVersionId,omitempty" jsonschema:"The AI Agent Version the rules file was compiled for."`
	SizeInBytes    int64        `json:"sizeInBytes,omitempty" jsonschema:"Size of the rules file (a ZIP) in bytes. Check the downloaded file against it."`
	ContentHash    string       `json:"contentHash,omitempty" jsonschema:"SHA-256 of the rules file, hex-encoded. Check the downloaded file against it."`
	CompiledAt     string       `json:"compiledAt,omitempty" jsonschema:"When the rules file was compiled (UTC)."`
	DownloadURL    string       `json:"downloadUrl,omitempty" jsonschema:"SENSITIVE. Short-lived signed link to the rules file. Use it once, straight away, to download the file on the customer's machine. Never show it to the customer, never write it to a file, and never log it or repeat it in a message."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "get_agent_rules_file",
		Title: "Get Agent Rules File",
		Description: "Get what is needed to download and check an AI agent's latest rules file (an enforcement bundle: a ZIP holding the agent contract rules compiled for the agent to apply at runtime). " +
			"Returns the rules file's bundleId, version, agentVersionId, sizeInBytes, contentHash (SHA-256, hex) and compiledAt, and downloadUrl: a short-lived signed link to the file. " +
			"The tool does not download the file: download it yourself on the customer's machine, then check its size and SHA-256 against sizeInBytes and contentHash. " +
			"downloadUrl is SENSITIVE: use it once, straight away; never show it to the customer, never write it to a file, never log it or repeat it in a message. If it expires, call this tool again for a fresh one. " +
			"Use it after get_agent_guardian_status shows a rules file (compilation.latestBundle) for the agent, for example to install the rules file with the agent's deployment. It does not start a run (apply_latest_rules does). " +
			"aiAgentId is the AI Agent asset id, not the external id: get it from list_scanned_agents. " +
			"A not_found status with errorCode noBundleAvailable is an answer: the agent has no rules file yet; call get_agent_guardian_status to see why. errorCode aiAgentNotFound means Guardian has no record of the agent. Permission errors (401, 403) are not transient: do not retry them. It only reads. " +
			"Example requests: 'download my agent's rules file', 'get the enforcement bundle for my agent', 'install the latest rules on my agent'. " +
			"Note: requires permission to read Guardian enforcement (scope aigovernance.guardian.enforcement-read).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
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

		bundle, err := clients.GetLatestAgentRulesBundle(ctx, collibraClient, aiAgentID)
		if err != nil {
			return failure(err, "The rules file could not be found in Collibra"), nil
		}
		downloadURL, err := clients.GetAgentRulesBundleDownloadURL(ctx, collibraClient, bundle.ID)
		if err != nil {
			return failure(err, "The rules file's download link could not be read from Collibra"), nil
		}
		return Output{
			Status: StatusSuccess,
			Message: fmt.Sprintf("The agent's latest rules file is version %d (%d bytes), compiled %s. "+
				"downloadUrl is a short-lived secret link: download the file once on the customer's machine and check it against sizeInBytes and contentHash; never show or store the link.",
				bundle.Version, bundle.SizeInBytes, bundle.CompiledAt),
			BundleID:       bundle.ID,
			Version:        bundle.Version,
			AgentVersionID: bundle.AgentVersionID,
			SizeInBytes:    bundle.SizeInBytes,
			ContentHash:    bundle.ContentHash,
			CompiledAt:     bundle.CompiledAt,
			DownloadURL:    downloadURL,
		}, nil
	}
}

func failure(err error, what string) Output {
	message, guidance, errorCode, httpStatus, ok := clients.DescribeGuardianError(err)
	if !ok {
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("%s: %v.", what, err),
			Guidance: "This looks like a network problem. Tell the customer the lookup did not work; trying once more later is fine.",
		}
	}
	status := StatusError
	if httpStatus == http.StatusNotFound {
		status = StatusNotFound
	}
	return Output{Status: status, Message: message, Guidance: guidance, ErrorCode: errorCode, HTTPStatus: httpStatus}
}
