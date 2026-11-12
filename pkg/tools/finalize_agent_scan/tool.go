// Package finalize_agent_scan implements the finalize_agent_scan MCP tool: it
// turns the details an agent-scanning skill extracted from an AI agent's code
// into the complete agent scan document, with the hashes Collibra compares
// scans by. It runs locally and calls nothing: sending the document is
// publish_agent_scan's job.
package finalize_agent_scan

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/collibra/chip/pkg/chip"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// OutputStatus is the outcome of a finalize_agent_scan call.
type OutputStatus string

const (
	StatusSuccess         OutputStatus = "success"
	StatusValidationError OutputStatus = "validation_error"
)

// Input is the tool's typed input.
type Input struct {
	Extraction json.RawMessage `json:"extraction" jsonschema:"Required. The extraction as a JSON object: 'agents', 'tools' and 'warnings', optionally 'platform' and 'source', and on the ROOT agent optionally 'deployment' (see the agent-scanner skill's agent-scan-fields reference). Pass the object itself, not an encoded string."`
}

// Output is the typed response.
type Output struct {
	Status           OutputStatus    `json:"status" jsonschema:"'success' when the document was built; 'validation_error' when the extraction has problems (nothing was built: fix every problem listed and call again)."`
	Message          string          `json:"message" jsonschema:"Plain-language summary."`
	Problems         []string        `json:"problems,omitempty" jsonschema:"Every problem found in the extraction, on validation_error. Fix them from the agent's code, not by loosening the facts."`
	Document         json.RawMessage `json:"document,omitempty" jsonschema:"The complete agent scan document, on success. Save it unchanged as agent_scan.json; this is what publish_agent_scan sends."`
	ContentHash      string          `json:"contentHash,omitempty" jsonschema:"The document's content hash, on success."`
	AgentCount       int             `json:"agentCount,omitempty" jsonschema:"Number of agents in the document."`
	ToolCount        int             `json:"toolCount,omitempty" jsonschema:"Number of tools in the document."`
	Warnings         []string        `json:"warnings,omitempty" jsonschema:"Every warning in the document: the extraction's own, then those added here (a dropped reference, a defaulted confidence, a dropped endpoint). A dropped reference usually means something was missed in the code."`
	SuggestedAgentID string          `json:"suggestedAgentId,omitempty" jsonschema:"'<repoUrl>#<ROOT agent name>', or the ROOT agent's name when the extraction has no source. A suggestion only: the agent id sent to Collibra carries its telemetry route in front."`
}

// now is the clock for generatedOn; tests replace it.
var now = time.Now

// NewTool returns the registered tool.
func NewTool() *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "finalize_agent_scan",
		Title: "Finalize Agent Scan",
		Description: "Build the complete agent scan document from the details an agent-scanning skill extracted from an AI agent's code. An agent scan is a JSON description of an AI agent — its sub-agents, tools, models, system prompts and guardrails. " +
			"This tool checks the extraction, lists every problem, repairs references that match nothing (with a warning), computes the content hashes Collibra uses to tell whether an agent changed, and returns the document. " +
			"It runs locally: it calls nothing, sends nothing and changes nothing, so it needs no confirmation. Send the result with publish_agent_scan after the user agrees. " +
			"Required: extraction (a JSON object with 'agents', 'tools' and 'warnings', and optionally 'platform', 'source' and a ROOT-only 'deployment'). " +
			"Returns status 'success' with the document, its contentHash, counts, warnings and a suggested agent id; or 'validation_error' with every problem, so the extraction can be fixed in one go. " +
			"Also use it to check a stored scan: rebuild the extraction from it and compare the hashes. " +
			"Example requests: 'finalize the agent scan', 'compute the agent scan hashes', 'check this stored agent scan'.",
		Handler:     handler,
		Permissions: []string{},
		TypeSchemas: typeSchemas(),
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(_ context.Context, input Input) (Output, error) {
	extraction := bytes.TrimSpace(input.Extraction)
	if len(extraction) == 0 || bytes.Equal(extraction, []byte("null")) || extraction[0] != '{' {
		return validationError([]string{"extraction is required and must be a JSON object with 'agents', 'tools' and 'warnings'. Pass the object itself, not an encoded string."}), nil
	}

	result, problems := finalize(extraction, now())
	if len(problems) > 0 {
		return validationError(problems), nil
	}

	document, err := encode(result.document)
	if err != nil {
		return Output{}, fmt.Errorf("encoding the agent scan document: %w", err)
	}
	return Output{
		Status: StatusSuccess,
		Message: fmt.Sprintf("Built the agent scan: %d agents, %d tools, %d warnings, content hash %s. Nothing was sent; read the warnings, then show the user the scan before publish_agent_scan.",
			result.agentCount, result.toolCount, len(result.warnings), result.contentHash),
		Document:         document,
		ContentHash:      result.contentHash,
		AgentCount:       result.agentCount,
		ToolCount:        result.toolCount,
		Warnings:         result.warnings,
		SuggestedAgentID: result.suggestedAgentID,
	}, nil
}

// encode writes the document with two-space indentation, without escaping
// HTML characters or non-ASCII text, and with numbers exactly as received.
func encode(document object) (json.RawMessage, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(document); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buffer.Bytes(), "\n"), nil
}

func validationError(problems []string) Output {
	return Output{
		Status:   StatusValidationError,
		Message:  fmt.Sprintf("The extraction has %d problem(s), so no document was built: %s Fix them from the agent's code and call again.", len(problems), strings.Join(problems, " ")),
		Problems: problems,
	}
}

// typeSchemas advertises json.RawMessage as an object instead of an integer array.
func typeSchemas() map[reflect.Type]*jsonschema.Schema {
	return map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[json.RawMessage](): {
			Type:        "object",
			Description: "A JSON object.",
		},
	}
}
