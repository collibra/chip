package finalize_agent_scan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// repairReferences is Step 3: drop names that match nothing, and give a
// guardrail without a confidence 0.5. Each repair adds a warning.
func (s *scan) repairReferences() {
	agentNames, toolNames := map[string]bool{}, map[string]bool{}
	for _, agent := range s.agents {
		agentNames[name(agent)] = true
	}
	for _, tool := range s.tools {
		toolNames[name(tool)] = true
	}
	for _, agent := range s.agents {
		s.keepKnown(agent, "tools", toolNames)
		s.keepKnown(agent, "subagents", agentNames)
	}
	for _, tool := range s.tools {
		s.keepKnown(tool, "usedByAgents", agentNames)
	}
	for _, tool := range s.tools {
		for _, guardrail := range objectsIn(tool, "guardrails") {
			if value, present := guardrail["confidence"]; !present || value == nil {
				guardrail["confidence"] = json.Number("0.5")
				s.warn("guardrail '%s' on '%s' had no confidence; set to 0.5", guardrail["kind"], name(tool))
			}
		}
	}
}

func (s *scan) keepKnown(owner object, field string, known map[string]bool) {
	items, _ := owner[field].([]any)
	kept := make([]any, 0, len(items))
	for _, item := range items {
		reference, _ := item.(string)
		if !known[reference] {
			s.warn("%s.%s referenced '%s', which matches no known name; dropped", name(owner), field, reference)
			continue
		}
		kept = append(kept, reference)
	}
	owner[field] = kept
}

// hash runs Steps 4 to 6: it sets every tool's and agent's contentHash and
// returns the document hash.
func (s *scan) hash() string {
	toolHashes := map[string]string{}
	for _, tool := range s.tools {
		if tool["definitionPattern"] == "mcp_dynamic" {
			tool["contentHash"] = nil
			continue
		}
		toolHashes[name(tool)] = sha256Hex(s.toolText(tool))
		tool["contentHash"] = toolHashes[name(tool)]
	}
	agentHashes := s.hashAgents(toolHashes)
	return sha256Hex(s.documentText(agentHashes, toolHashes))
}

// toolText is Step 4: arguments by name, the return type, then guardrails.
func (s *scan) toolText(tool object) string {
	var text strings.Builder
	args := objectsIn(tool, "args")
	slices.SortStableFunc(args, func(a, b object) int { return strings.Compare(name(a), name(b)) })
	for _, arg := range args {
		argType, _ := arg["type"].(string)
		text.WriteString(name(arg) + argType + strconv.FormatBool(arg["required"] == true))
		if value, present := arg["default"]; present && value != nil {
			text.WriteString(s.formatValue(value, "tool '"+name(tool)+"' argument '"+name(arg)+"' default"))
		}
	}
	if returns, ok := tool["returns"].(object); ok {
		returnType, _ := returns["type"].(string)
		text.WriteString(returnType)
	}
	guardrails := objectsIn(tool, "guardrails")
	slices.SortStableFunc(guardrails, func(a, b object) int { return strings.Compare(guardrailKey(a), guardrailKey(b)) })
	for _, guardrail := range guardrails {
		text.WriteString(stringOf(guardrail["kind"]) + strings.Join(names(guardrail["argsInvolved"]), "") + stringOf(guardrail["behavior"]))
	}
	return text.String()
}

func guardrailKey(guardrail object) string {
	return stringOf(guardrail["kind"]) + "|" + stringOf(guardrail["behavior"]) + "|" + strings.Join(names(guardrail["argsInvolved"]), ",")
}

// hashAgents is Step 5. Agents that can all reach each other through their
// subagents form a loop group and are hashed as one unit; groups are worked
// through bottom-up, so every subagent outside a group is hashed first.
func (s *scan) hashAgents(toolHashes map[string]string) map[string]string {
	byName := map[string]object{}
	ownHashes := map[string]string{}
	for _, agent := range s.agents {
		byName[name(agent)] = agent
		ownHashes[name(agent)] = sha256Hex(s.agentText(agent, toolHashes, nil))
	}

	contentHashes := map[string]string{}
	for _, group := range loopGroups(s.agents) {
		groupHash := ""
		if len(group) > 1 {
			own := make([]string, len(group))
			for i, member := range group {
				own[i] = ownHashes[member]
			}
			slices.Sort(own)
			groupHash = sha256Hex(strings.Join(own, ""))
		}
		for _, member := range group {
			var term []string
			for _, subagent := range names(byName[member]["subagents"]) {
				if !slices.Contains(group, subagent) {
					term = append(term, contentHashes[subagent])
				}
			}
			if groupHash != "" {
				term = append(term, groupHash)
			}
			slices.Sort(term)
			contentHashes[member] = sha256Hex(s.agentText(byName[member], toolHashes, term))
		}
	}
	for _, agent := range s.agents {
		agent["contentHash"] = contentHashes[name(agent)]
	}
	return contentHashes
}

// agentText builds the Step 5 text; a nil subagent term gives the own-content text.
func (s *scan) agentText(agent object, toolHashes map[string]string, subagentTerm []string) string {
	var text strings.Builder
	owner := "agent '" + name(agent) + "'"
	framework, _ := agent["framework"].(object)
	text.WriteString(stringOf(agent["pattern"]) + stringOf(framework["name"]) + stringOf(framework["version"]))

	for _, model := range objectsIn(agent, "models") {
		if model["isDefault"] != true {
			continue
		}
		text.WriteString(stringOf(model["provider"]) + stringOf(model["modelId"]))
		parameters, _ := model["parameters"].(object)
		for _, key := range []string{"temperature", "top_p", "max_tokens"} {
			if value, present := parameters[key]; present && value != nil {
				text.WriteString(key + s.formatValue(value, owner+" model parameter '"+key+"'"))
			}
		}
		break
	}

	if prompt, ok := agent["systemPrompt"].(object); ok {
		text.WriteString(stringOf(prompt["text"]))
	}

	var tools []string
	for _, tool := range names(agent["tools"]) {
		if hash, ok := toolHashes[tool]; ok {
			tools = append(tools, hash)
		}
	}
	slices.Sort(tools)
	text.WriteString(strings.Join(tools, ""))
	text.WriteString(strings.Join(subagentTerm, ""))

	middleware := objectsIn(agent, "middleware")
	slices.SortStableFunc(middleware, func(a, b object) int {
		return strings.Compare(stringOf(a["class"])+stringOf(a["hook"]), stringOf(b["class"])+stringOf(b["hook"]))
	})
	for _, entry := range middleware {
		text.WriteString(stringOf(entry["class"]) + stringOf(entry["hook"]))
	}

	if persistence, ok := agent["crossSessionPersistence"].(bool); ok {
		text.WriteString(strconv.FormatBool(persistence))
	}
	return text.String()
}

// documentText is Step 6: sorted agent hashes, sorted tool hashes, then the
// ROOT agent's deployment version and endpoint url on their own lines.
func (s *scan) documentText(agentHashes, toolHashes map[string]string) string {
	agents := mapValues(agentHashes)
	tools := mapValues(toolHashes)
	slices.Sort(agents)
	slices.Sort(tools)

	version, address := "", ""
	if deployment, ok := s.root()["deployment"].(object); ok {
		version, _ = deployment["version"].(string)
		if endpoint, ok := deployment["endpoint"].(object); ok {
			address, _ = endpoint["url"].(string)
		}
	}
	return strings.Join(agents, "") + strings.Join(tools, "") + "\n" + version + "\n" + address
}

// loopGroups returns the agents grouped by delegation loop (strongly connected
// components of the subagents graph), each group after every group it delegates to.
func loopGroups(agents []object) [][]string {
	edges := map[string][]string{}
	for _, agent := range agents {
		edges[name(agent)] = names(agent["subagents"])
	}
	index, low := map[string]int{}, map[string]int{}
	onStack := map[string]bool{}
	var stack []string
	var groups [][]string
	next := 0

	var visit func(string)
	visit = func(agent string) {
		index[agent], low[agent] = next, next
		next++
		stack = append(stack, agent)
		onStack[agent] = true
		for _, subagent := range edges[agent] {
			if _, seen := index[subagent]; !seen {
				visit(subagent)
				low[agent] = min(low[agent], low[subagent])
			} else if onStack[subagent] {
				low[agent] = min(low[agent], index[subagent])
			}
		}
		if low[agent] == index[agent] {
			var group []string
			for {
				member := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				onStack[member] = false
				group = append(group, member)
				if member == agent {
					break
				}
			}
			groups = append(groups, group)
		}
	}
	for _, agent := range agents {
		if _, seen := index[name(agent)]; !seen {
			visit(name(agent))
		}
	}
	return groups
}

var integerLiteral = regexp.MustCompile(`^-?[0-9]+$`)

// formatValue writes a JSON value as text, as "Value formatting" in the
// procedure says. where names the value in a warning.
func (s *scan) formatValue(value any, where string) string {
	switch v := value.(type) {
	case string:
		return v
	case bool:
		return strconv.FormatBool(v)
	case json.Number:
		return s.formatNumber(v, where)
	case []any:
		items := make([]string, len(v))
		for i, item := range v {
			items[i] = s.formatValue(item, where)
		}
		return "[" + strings.Join(items, " ") + "]"
	case object:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		pairs := make([]string, len(keys))
		for i, key := range keys {
			pairs[i] = key + ":" + s.formatValue(v[key], where)
		}
		return "map[" + strings.Join(pairs, " ") + "]"
	}
	return ""
}

// formatNumber writes a whole number as digits only and any other number as the
// shortest decimal that reads back to it. A number that would need exponent
// notation is written as it arrived, with a warning: the assistant's client may
// have re-encoded it, so its source text can't be guaranteed.
func (s *scan) formatNumber(number json.Number, where string) string {
	literal := number.String()
	if integerLiteral.MatchString(literal) {
		return literal
	}
	f, err := number.Float64()
	if err != nil {
		return literal
	}
	magnitude := math.Abs(f)
	switch {
	case f == 0:
		return "0"
	case f == math.Trunc(f) && magnitude < 1e21:
		return strconv.FormatFloat(f, 'f', -1, 64)
	case magnitude < 1e-4 || magnitude >= 1e21:
		s.warn("the number %s in %s needs exponent notation; it was hashed as received, so its hash may not match one made from the source text", literal, where)
		return literal
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func sha256Hex(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func stringOf(value any) string {
	text, _ := value.(string)
	return text
}

func names(value any) []string {
	items, _ := value.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, stringOf(item))
	}
	return out
}

func mapValues(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for _, value := range m {
		out = append(out, value)
	}
	return out
}
