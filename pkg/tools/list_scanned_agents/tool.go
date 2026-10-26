// Package list_scanned_agents implements the list_scanned_agents MCP tool: it
// lists the agents Collibra has imported from one source system, with the AI
// Agent asset id each was imported as, so a caller holding only an external id
// can find the asset id the other agent scan reads ask for.
package list_scanned_agents

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	pageSize = 500
	// maxPages bounds the walk done for an exact externalId so a huge system cannot stall the call.
	maxPages = 10
)

// OutputStatus is the outcome of a list_scanned_agents call.
type OutputStatus string

const (
	StatusSuccess         OutputStatus = "success"
	StatusValidationError OutputStatus = "validation_error"
	StatusError           OutputStatus = "error"
)

// Input is the tool's typed input.
type Input struct {
	SourceSystem string `json:"sourceSystem,omitempty" jsonschema:"Required unless agentNameContains is set. A prefix of the agents' external ids, matched against the start of the ids, so every imported agent whose id starts with it is selected, also when it ends inside a segment (the tool handles that; a trailing ':' is ignored). For an id shaped '<route>:<instance>:<native id>' use its first two segments, e.g. 'databricks:1234567890123456'. For an id without a route, e.g. '<repository address>#<agent name>', use the part before the agent's own name (the repository address), or the whole id to select just that agent."`
	ExternalID   string `json:"externalId,omitempty" jsonschema:"Optional. Return only the agent with exactly this external id, e.g. 'databricks:1234567890123456:example_catalog.example_schema.example_agent'. It must start with sourceSystem. The tool pages through the matching agents for you, so use this to find one agent's asset id without reading the whole list."`
	NameContains string `json:"agentNameContains,omitempty" jsonschema:"Optional. Find agents by a part of the agent's name in the catalog (the name of the AI Agent asset, for example 'call-analysis' finds an agent called 'call-analysis-orchestrator'), whatever its external id looks like. It searches the catalog's AI Agent assets, returns their asset ids, and ignores externalId and externalIdContains. If sourceSystem is also given, only agents whose external id starts with it are kept."`
	Contains     string `json:"externalIdContains,omitempty" jsonschema:"Optional. Return only the agents whose external id contains this text (case-sensitive), for example a model name such as 'example_schema.example_agent' to find where a model was imported without knowing its workspace. The tool pages through the matching agents for you and returns at most limit of them. Ignored when externalId is set."`
	Limit        int    `json:"limit,omitempty" jsonschema:"Optional. Page size when listing without externalId or externalIdContains; with externalIdContains, the most matches to return. Default 100, maximum 500."`
	Cursor       string `json:"cursor,omitempty" jsonschema:"Optional. The nextCursor of an earlier page, to read the next one. Ignored when externalId is set."`
}

// Agent is one imported agent.
type Agent struct {
	ExternalID       string `json:"externalId" jsonschema:"The agent's identifier in its source system."`
	AIAgentID        string `json:"aiAgentId" jsonschema:"The AI Agent asset this agent was imported as. Pass it to get_latest_agent_scan."`
	AIAgentVersionID string `json:"aiAgentVersionId,omitempty" jsonschema:"The AI Agent Version asset its latest state was imported as; absent until that state has been versioned."`
	AgentName        string `json:"agentName,omitempty" jsonschema:"The name of the AI Agent asset, when the agent was found by name."`
	Domain           string `json:"domain,omitempty" jsonschema:"The domain the AI Agent asset sits in, when the agent was found by name."`
}

// Output is the typed response.
type Output struct {
	Status     OutputStatus `json:"status" jsonschema:"'success' (the agents field may be empty: that is an answer, not a fault); 'validation_error' for bad inputs; 'error' when Collibra could not answer."`
	Message    string       `json:"message" jsonschema:"Plain-language summary that can be relayed to the customer."`
	Guidance   string       `json:"guidance,omitempty" jsonschema:"What to tell or ask the customer next, on error."`
	Agents     []Agent      `json:"agents,omitempty" jsonschema:"The imported agents found, newest-imported first. Only agents Collibra has imported appear: an agent that was scanned but not yet imported has no asset id and is not listed."`
	NextCursor string       `json:"nextCursor,omitempty" jsonschema:"Pass as cursor to read the next page. Absent on the last page and when externalId is set."`
	Truncated  bool         `json:"truncated,omitempty" jsonschema:"True when an externalId search stopped at its page limit before finding the agent, so the agent may exist further on."`
	HTTPStatus int          `json:"httpStatus,omitempty" jsonschema:"HTTP status of the rejection, when Collibra answered with one."`
}

// NewTool returns the registered tool.
func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:  "list_scanned_agents",
		Title: "List Scanned Agents",
		Description: "List the agents Collibra has imported from one source system (for example one Databricks workspace), each with the AI Agent asset id it was imported as. " +
			"Use it before get_latest_agent_scan to turn an agent's external id (shaped '<route>:<instance>:<native id>', e.g. 'databricks:1234567890123456:catalog.schema.model') into the asset id that tool needs, or to see which agents of a system Collibra already knows. " +
			"If you do not know the external id, pass agentNameContains instead: a part of the agent's name (for example 'call-analysis'), searched among the catalog's AI Agent assets, which works whatever the id looks like. " +
			"Otherwise pass sourceSystem (a prefix of the external ids: the first two segments for a route-shaped id such as 'databricks:1234567890123456', the route alone such as 'databricks:' to search every instance, the repository address for an id like '<repository>#<name>', or the whole id) and, to find one agent, its full externalId, or externalIdContains to find agents by a part of the id such as a model name. " +
			"Only agents Collibra has imported are listed; an agent that was scanned but not yet imported is absent, and an empty result is a valid answer. It only reads. " +
			"Note: requires permission to read agent scans (scope aigovernance.guardian.fingerprint-read).",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: chip.Ptr(false), IdempotentHint: true, OpenWorldHint: chip.Ptr(false)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if message := validate(input); message != "" {
			return Output{Status: StatusValidationError, Message: message}, nil
		}
		if input.NameContains != "" {
			return findByName(ctx, collibraClient, input), nil
		}
		if input.ExternalID != "" {
			return findOne(ctx, collibraClient, input), nil
		}
		if input.Contains != "" {
			return findContaining(ctx, collibraClient, input), nil
		}
		return listPage(ctx, collibraClient, input), nil
	}
}

func validate(input Input) string {
	system := input.SourceSystem
	switch {
	case input.NameContains != "":
		if input.NameContains != strings.TrimSpace(input.NameContains) || strings.Contains(input.NameContains, ">") {
			return fmt.Sprintf("agentNameContains %q must not have leading or trailing whitespace or contain '>'.", input.NameContains)
		}
		if system != strings.TrimSpace(system) || strings.Contains(system, ">") {
			return fmt.Sprintf("sourceSystem %q must not have leading or trailing whitespace or contain '>'.", system)
		}
		return ""
	case strings.TrimSpace(system) == "":
		return "sourceSystem is required: a prefix of the agents' external ids, e.g. 'databricks:1234567890123456' (the first two segments of a route-shaped id), a repository address, or a whole id."
	case system != strings.TrimSpace(system):
		return fmt.Sprintf("sourceSystem %q has leading or trailing whitespace. Remove it and call again.", system)
	case strings.Contains(system, ">"):
		return fmt.Sprintf("sourceSystem %q contains '>', which an external id never does. Pass a prefix of the id, e.g. 'databricks:1234567890123456'.", system)
	case input.Contains != strings.TrimSpace(input.Contains) || strings.Contains(input.Contains, ">"):
		return fmt.Sprintf("externalIdContains %q must not have leading or trailing whitespace or contain '>'.", input.Contains)
	case input.ExternalID != "" && !strings.HasPrefix(input.ExternalID, system):
		return fmt.Sprintf("externalId %q does not start with sourceSystem %q. sourceSystem is a prefix of the external ids.", input.ExternalID, system)
	case input.Limit < 0 || input.Limit > pageSize:
		return fmt.Sprintf("limit must be between 1 and %d.", pageSize)
	}
	return ""
}

// findByName searches the catalog's AI Agent assets by name, so an agent can be
// found by what it is called, whatever its external id looks like.
func findByName(ctx context.Context, collibraClient *http.Client, input Input) Output {
	assets, err := clients.FindAgentAssetsByName(ctx, collibraClient, input.NameContains, 0)
	if err != nil {
		return failure(err)
	}
	want := input.Limit
	if want == 0 {
		want = 100
	}
	prefix := normalize(input.SourceSystem)
	var agents []Agent
	for _, asset := range assets {
		if prefix != "" && !strings.HasPrefix(asset.ExternalID, prefix) {
			continue
		}
		agents = append(agents, Agent{ExternalID: asset.ExternalID, AIAgentID: asset.AssetID, AgentName: asset.AgentName, Domain: asset.DomainName})
		if len(agents) == want {
			break
		}
	}
	return Output{
		Status:  StatusSuccess,
		Agents:  agents,
		Message: fmt.Sprintf("The catalog has %d AI Agent asset(s) whose name contains %q.", len(agents), input.NameContains),
	}
}

// listPage reads one page for a sourceSystem that names whole leading
// segments. A prefix that ends inside a segment (a repository address, a part
// of an id) selects nothing server-side, so an empty first page falls back to
// walking the provider's agents and applying the prefix here.
func listPage(ctx context.Context, collibraClient *http.Client, input Input) Output {
	system := normalize(input.SourceSystem)
	page, err := clients.ListImportedAgents(ctx, collibraClient, system, input.Cursor, input.Limit)
	if err != nil {
		return failure(err)
	}
	if len(page.Results) == 0 && input.Cursor == "" && strings.Contains(system, ":") {
		return listByPrefix(ctx, collibraClient, input, system)
	}
	agents := toAgents(page.Results)
	return Output{
		Status:     StatusSuccess,
		Message:    fmt.Sprintf("Collibra lists %d imported agent(s) for %s on this page.", len(agents), system),
		Agents:     agents,
		NextCursor: page.NextCursor,
	}
}

func listByPrefix(ctx context.Context, collibraClient *http.Client, input Input, system string) Output {
	want := input.Limit
	if want == 0 {
		want = 100
	}
	var matches []clients.ImportedAgent
	complete, err := scan(ctx, collibraClient, system, func(agent clients.ImportedAgent) bool {
		matches = append(matches, agent)
		return len(matches) == want
	})
	if err != nil {
		return failure(err)
	}
	return Output{
		Status:    StatusSuccess,
		Agents:    toAgents(matches),
		Truncated: !complete,
		Message:   fmt.Sprintf("Collibra lists %d imported agent(s) whose external id starts with %q.", len(matches), system),
	}
}

// findOne walks the matching agents for the one external id, so the caller
// does not have to read every agent just to learn one asset id.
func findOne(ctx context.Context, collibraClient *http.Client, input Input) Output {
	var found *clients.ImportedAgent
	complete, err := scan(ctx, collibraClient, input.SourceSystem, func(agent clients.ImportedAgent) bool {
		if agent.ExternalID == input.ExternalID {
			found = &agent
		}
		return found != nil
	})
	if err != nil {
		return failure(err)
	}
	switch {
	case found != nil:
		return Output{
			Status:  StatusSuccess,
			Message: fmt.Sprintf("Collibra has imported %s as AI Agent asset %s.", found.ExternalID, found.AIAgentID),
			Agents:  toAgents([]clients.ImportedAgent{*found}),
		}
	case complete:
		return Output{Status: StatusSuccess, Message: fmt.Sprintf("Collibra has not imported an agent with external id %q (it may have been scanned but not yet imported).", input.ExternalID)}
	}
	return Output{
		Status:    StatusSuccess,
		Truncated: true,
		Message:   fmt.Sprintf("No agent with external id %q was found in the first %d agents searched; the search stopped there, so it may exist further on.", input.ExternalID, maxPages*pageSize),
	}
}

// findContaining keeps the agents whose external id contains the text, so the
// caller never reads agents it does not need.
func findContaining(ctx context.Context, collibraClient *http.Client, input Input) Output {
	want := input.Limit
	if want == 0 {
		want = 100
	}
	var matches []clients.ImportedAgent
	complete, err := scan(ctx, collibraClient, input.SourceSystem, func(agent clients.ImportedAgent) bool {
		if strings.Contains(agent.ExternalID, input.Contains) {
			matches = append(matches, agent)
		}
		return len(matches) == want
	})
	if err != nil {
		return failure(err)
	}
	message := fmt.Sprintf("Collibra has imported %d agent(s) containing %q under %s.", len(matches), input.Contains, normalize(input.SourceSystem))
	if !complete {
		message = fmt.Sprintf("Found %d agent(s) containing %q under %s; the search stopped before the end, so there may be more.", len(matches), input.Contains, normalize(input.SourceSystem))
	}
	return Output{Status: StatusSuccess, Agents: toAgents(matches), Truncated: !complete, Message: message}
}

// scan visits the imported agents whose external id starts with prefix, until
// visit returns true or the agents run out; complete says whether it reached
// the end.
//
// The service narrows by whole leading segments: it matches "<sourceSystem>:"
// against the start of the id. A prefix that ends inside a segment, such as a
// repository address followed by '#<name>', therefore finds nothing there. When
// the prefix as given selects no agent at all, scan asks for the provider (the
// part before the first ':') and applies the prefix itself.
func scan(ctx context.Context, collibraClient *http.Client, prefix string, visit func(clients.ImportedAgent) bool) (complete bool, err error) {
	prefix = normalize(prefix)
	seen := false
	complete, err = walk(ctx, collibraClient, prefix, func(agent clients.ImportedAgent) bool {
		seen = true
		return visit(agent)
	})
	provider, _, hasMore := strings.Cut(prefix, ":")
	if err != nil || seen || !complete || !hasMore {
		return complete, err
	}
	return walk(ctx, collibraClient, provider, func(agent clients.ImportedAgent) bool {
		return strings.HasPrefix(agent.ExternalID, prefix) && visit(agent)
	})
}

func walk(ctx context.Context, collibraClient *http.Client, system string, visit func(clients.ImportedAgent) bool) (complete bool, err error) {
	cursor := ""
	for range maxPages {
		page, err := clients.ListImportedAgents(ctx, collibraClient, system, cursor, pageSize)
		if err != nil {
			return false, err
		}
		for _, agent := range page.Results {
			if visit(agent) {
				return false, nil
			}
		}
		if page.NextCursor == "" {
			return true, nil
		}
		cursor = page.NextCursor
	}
	return false, nil
}

// normalize drops trailing colons: "databricks:" means the provider alone.
func normalize(system string) string {
	return strings.TrimRight(system, ":")
}

func toAgents(in []clients.ImportedAgent) []Agent {
	out := make([]Agent, 0, len(in))
	for _, a := range in {
		out = append(out, Agent{ExternalID: a.ExternalID, AIAgentID: a.AIAgentID, AIAgentVersionID: a.AIAgentVersionID})
	}
	return out
}

func failure(err error) Output {
	var apiErr *clients.AgentScanAPIError
	if !errors.As(err, &apiErr) {
		return Output{
			Status:   StatusError,
			Message:  fmt.Sprintf("The list of imported agents could not be read from Collibra: %v.", err),
			Guidance: "This looks like a network problem. Tell the customer the lookup did not work and carry on without it; trying once more later is fine.",
		}
	}
	message, guidance := apiErr.ReadFailure()
	return Output{Status: StatusError, Message: message, Guidance: guidance, HTTPStatus: apiErr.StatusCode}
}
