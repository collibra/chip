package finalize_agent_scan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"
)

// The finalize procedure turns an agent-scanning skill's extraction into the
// complete agent scan document: normalise, reject, repair references, hash,
// assemble. It follows references/finalize-procedure.md of the agent-scanner
// skill step by step; the step numbers in comments refer to that file.

const llmFallback = "LLM_FALLBACK"

type object = map[string]any

// result is a finished agent scan document and what the caller reports about it.
type result struct {
	document         object
	contentHash      string
	agentCount       int
	toolCount        int
	warnings         []string
	suggestedAgentID string
}

// finalize runs the whole procedure. It returns either a result or the list of
// every problem found, never both.
func finalize(extraction []byte, now time.Time) (*result, []string) {
	s, problems := decodeExtraction(extraction)
	if len(problems) > 0 {
		return nil, problems
	}
	s.normalise()
	s.cleanDeployments()
	s.validate()
	if len(s.problems) > 0 {
		return nil, s.problems
	}
	s.repairReferences()
	contentHash := s.hash()
	return &result{
		document:         s.assemble(contentHash, now),
		contentHash:      contentHash,
		agentCount:       len(s.agents),
		toolCount:        len(s.tools),
		warnings:         s.warnings,
		suggestedAgentID: s.suggestedAgentID(),
	}, nil
}

// scan holds the extraction while it is worked on.
type scan struct {
	agents   []object
	tools    []object
	warnings []string
	platform any
	source   any
	problems []string
}

func (s *scan) problem(format string, args ...any) {
	s.problems = append(s.problems, fmt.Sprintf(format, args...))
}

func (s *scan) warn(format string, args ...any) {
	s.warnings = append(s.warnings, fmt.Sprintf(format, args...))
}

var extractionKeys = []string{"agents", "tools", "warnings", "platform", "source"}

// decodeExtraction reads the extraction, keeping numbers as written.
func decodeExtraction(extraction []byte) (*scan, []string) {
	decoder := json.NewDecoder(bytes.NewReader(extraction))
	decoder.UseNumber()
	var top object
	if err := decoder.Decode(&top); err != nil || top == nil {
		return nil, []string{"extraction must be a JSON object with 'agents', 'tools' and 'warnings'."}
	}
	if decoder.More() {
		return nil, []string{"extraction must be one JSON object, with nothing after it."}
	}

	s := &scan{platform: top["platform"], source: top["source"]}
	for key := range top {
		if !slices.Contains(extractionKeys, key) {
			s.problem("extraction has an unknown field '%s'. Allowed: %s.", key, strings.Join(extractionKeys, ", "))
		}
	}
	s.agents = s.objectList(top, "agents", "extraction")
	s.tools = s.objectList(top, "tools", "extraction")
	s.warnings = s.stringList(top["warnings"], "extraction.warnings")
	return s, s.problems
}

// objectList reads a required list of objects (Step 2: agents and tools must be arrays).
func (s *scan) objectList(parent object, key, owner string) []object {
	items, ok := parent[key].([]any)
	if !ok {
		s.problem("%s.%s must be a list.", owner, key)
		return nil
	}
	objects := make([]object, 0, len(items))
	for i, item := range items {
		o, ok := item.(object)
		if !ok {
			s.problem("%s.%s[%d] must be an object.", owner, key, i)
			continue
		}
		objects = append(objects, o)
	}
	return objects
}

// stringList reads an optional list of strings; absent or null is empty.
func (s *scan) stringList(value any, owner string) []string {
	if value == nil {
		return []string{}
	}
	items, ok := value.([]any)
	if !ok {
		s.problem("%s must be a list of strings.", owner)
		return []string{}
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			s.problem("%s must contain only strings.", owner)
			continue
		}
		out = append(out, text)
	}
	return out
}

// normalise is Step 1: drop evidence, set via, lowercase, and make the lists lists.
func (s *scan) normalise() {
	for _, agent := range s.agents {
		owner := "agent '" + name(agent) + "'"
		delete(agent, "evidence")
		dropNulls(agent, "framework", "systemPrompt", "crossSessionPersistence", "deployment")
		agent["via"] = llmFallback
		if framework, ok := agent["framework"].(object); ok {
			s.lowercase(framework, "name", owner+" framework")
		}
		for _, model := range s.objects(agent, "models", owner) {
			model["via"] = llmFallback
			s.lowercase(model, "provider", owner+" model")
		}
		if prompt, ok := agent["systemPrompt"].(object); ok {
			prompt["via"] = llmFallback
		}
		for _, middleware := range s.objects(agent, "middleware", owner) {
			middleware["via"] = llmFallback
			s.lowercase(middleware, "hook", owner+" middleware")
		}
		s.ensureList(agent, "tools", owner)
		s.ensureList(agent, "subagents", owner)
	}
	for _, tool := range s.tools {
		owner := "tool '" + name(tool) + "'"
		delete(tool, "evidence")
		dropNulls(tool, "description", "returns")
		tool["via"] = llmFallback
		s.ensureList(tool, "args", owner)
		s.ensureList(tool, "usedByAgents", owner)
		for _, guardrail := range s.objects(tool, "guardrails", owner) {
			guardrail["via"] = llmFallback
			s.lowercase(guardrail, "kind", owner+" guardrail")
			s.lowercase(guardrail, "behavior", owner+" guardrail")
			s.ensureList(guardrail, "argsInvolved", owner+" guardrail")
		}
	}
}

// objects makes parent[key] a list and returns its objects; a non-object entry is a problem.
func (s *scan) objects(parent object, key, owner string) []object {
	items := s.ensureList(parent, key, owner)
	out := make([]object, 0, len(items))
	for _, item := range items {
		o, ok := item.(object)
		if !ok {
			s.problem("%s: every entry of '%s' must be an object.", owner, key)
			continue
		}
		out = append(out, o)
	}
	return out
}

// ensureList makes parent[key] an empty list when it is missing or null.
func (s *scan) ensureList(parent object, key, owner string) []any {
	value, present := parent[key]
	if !present || value == nil {
		parent[key] = []any{}
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		s.problem("%s: '%s' must be a list.", owner, key)
		return nil
	}
	return items
}

func (s *scan) lowercase(parent object, key, owner string) {
	value, present := parent[key]
	if !present || value == nil {
		delete(parent, key)
		return
	}
	text, ok := value.(string)
	if !ok {
		s.problem("%s: '%s' must be a string.", owner, key)
		return
	}
	parent[key] = strings.ToLower(text)
}

func dropNulls(parent object, keys ...string) {
	for _, key := range keys {
		if value, present := parent[key]; present && value == nil {
			delete(parent, key)
		}
	}
}

// cleanDeployments runs before the Step 2 checks: blank values count as absent,
// an endpoint with no url is dropped with a warning, and an empty deployment is removed.
func (s *scan) cleanDeployments() {
	for _, agent := range s.agents {
		deployment, ok := agent["deployment"].(object)
		if !ok {
			continue
		}
		dropBlank(deployment, "version")
		if endpoint, ok := deployment["endpoint"].(object); ok {
			dropBlank(endpoint, "name")
			dropBlank(endpoint, "url")
			if _, hasURL := endpoint["url"]; !hasURL {
				delete(deployment, "endpoint")
				s.warn("deployment.endpoint had no url and was dropped")
			}
		} else if value, present := deployment["endpoint"]; present && value == nil {
			delete(deployment, "endpoint")
		}
		if len(deployment) == 0 {
			delete(agent, "deployment")
		}
	}
}

func dropBlank(parent object, key string) {
	value, present := parent[key]
	if !present {
		return
	}
	if text, ok := value.(string); value == nil || (ok && strings.TrimSpace(text) == "") {
		delete(parent, key)
	}
}

func name(o object) string {
	text, _ := o["name"].(string)
	return text
}

// suggestedAgentID is Step 8: "<repoUrl>#<ROOT name>", or the ROOT name alone.
func (s *scan) suggestedAgentID() string {
	root := name(s.root())
	if source, ok := s.source.(object); ok {
		if repoURL, _ := source["repoUrl"].(string); repoURL != "" {
			return repoURL + "#" + root
		}
	}
	return root
}

func (s *scan) root() object {
	for _, agent := range s.agents {
		if agent["role"] == "ROOT" {
			return agent
		}
	}
	return nil
}

// assemble is Step 7: the complete document.
func (s *scan) assemble(contentHash string, now time.Time) object {
	document := object{
		"fingerprintVersion": "1.0",
		"parser":             object{"version": "skill-0.1", "llmProvider": "CUSTOMER"},
		"generatedOn":        now.UTC().Format("2006-01-02T15:04:05Z"),
		"contentHash":        contentHash,
		"agents":             asList(s.agents),
		"tools":              asList(s.tools),
		"warnings":           s.warnings,
	}
	if platform, ok := s.platform.(object); ok {
		out := object{"name": platform["name"]}
		if metadata, ok := platform["metadata"].(object); ok && len(metadata) > 0 {
			out["metadata"] = metadata
		}
		document["platform"] = out
	}
	if source, ok := s.source.(object); ok {
		out := object{}
		for _, key := range []string{"repoUrl", "commitSha"} {
			if text, _ := source[key].(string); text != "" {
				out[key] = text
			}
		}
		if len(out) > 0 {
			document["source"] = out
		}
	}
	return document
}

func asList(objects []object) []any {
	out := make([]any, len(objects))
	for i, o := range objects {
		out[i] = o
	}
	return out
}

var upperSnakeCase = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

func validURLWithoutCredentials(raw string) bool {
	parsed, err := url.Parse(raw)
	return err == nil && parsed.User == nil
}
