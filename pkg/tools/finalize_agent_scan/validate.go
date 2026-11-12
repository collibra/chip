package finalize_agent_scan

import (
	"slices"
	"strings"
)

// validate is Step 2 plus the shape check of Step 7: every problem is listed,
// not only the first, so the extraction can be fixed in one go.
func (s *scan) validate() {
	s.validateNames()
	s.validateRoles()
	for _, agent := range s.agents {
		s.validateAgent(agent)
	}
	for _, tool := range s.tools {
		s.validateTool(tool)
	}
	s.validatePlatform()
	s.validateSource()
}

func (s *scan) validateNames() {
	for _, group := range []struct {
		kind  string
		items []object
	}{{"agent", s.agents}, {"tool", s.tools}} {
		kind, items := group.kind, group.items
		seen := map[string]bool{}
		for _, item := range items {
			value, present := item["name"]
			text, isString := value.(string)
			switch {
			case present && !isString:
				s.problem("every %s name must be a string.", kind)
				continue
			case len([]rune(text)) < 3:
				s.problem("%s name '%s' is shorter than 3 characters; a missing name counts as empty.", kind, text)
			case seen[text]:
				s.problem("two %ss share the name '%s'; names must be unique.", kind, text)
			}
			seen[text] = true
		}
	}
}

var roles = []string{"ROOT", "SUBAGENT", "ORPHAN"}

func (s *scan) validateRoles() {
	roots := 0
	for _, agent := range s.agents {
		role, _ := agent["role"].(string)
		if !slices.Contains(roles, role) {
			s.problem("agent '%s' has role '%v'; use ROOT, SUBAGENT or ORPHAN.", name(agent), agent["role"])
		}
		if role == "ROOT" {
			roots++
		}
	}
	if roots != 1 {
		s.problem("exactly one agent must have role ROOT; found %d.", roots)
	}
}

var agentKeys = []string{"name", "contentHash", "role", "pattern", "via", "modelsResolved", "models", "toolsResolved", "tools",
	"subagentsResolved", "subagents", "middlewareResolved", "middleware", "framework", "systemPrompt", "crossSessionPersistence", "deployment"}

func (s *scan) validateAgent(agent object) {
	owner := "agent '" + name(agent) + "'"
	for _, old := range []string{"deploymentVersion", "endpointUrl"} {
		if _, present := agent[old]; present {
			s.problem("%s carries the old field '%s', which Collibra refuses; use 'deployment' (version, endpoint.url) on the ROOT agent instead.", owner, old)
			delete(agent, old)
		}
	}
	s.checkKeys(agent, owner, agentKeys, "name", "role", "pattern")
	s.checkString(agent, "pattern", owner)
	for _, flag := range []string{"modelsResolved", "toolsResolved", "subagentsResolved", "middlewareResolved"} {
		if _, ok := agent[flag].(bool); !ok {
			s.problem("%s needs '%s' as true or false.", owner, flag)
		}
	}
	if value, present := agent["crossSessionPersistence"]; present {
		if _, ok := value.(bool); !ok {
			s.problem("%s: 'crossSessionPersistence' must be true or false.", owner)
		}
	}
	s.checkNames(agent, "tools", owner)
	s.checkNames(agent, "subagents", owner)
	s.validateFramework(agent, owner)
	s.validateModels(agent, owner)
	s.validateSystemPrompt(agent, owner)
	for _, middleware := range objectsIn(agent, "middleware") {
		s.checkKeys(middleware, owner+" middleware", []string{"class", "hook", "via"}, "class", "hook")
		s.checkString(middleware, "class", owner+" middleware")
		s.checkString(middleware, "hook", owner+" middleware")
	}
	s.validateDeployment(agent, owner)
}

func (s *scan) validateFramework(agent object, owner string) {
	value, present := agent["framework"]
	if !present {
		return
	}
	framework, ok := value.(object)
	if !ok {
		s.problem("%s: 'framework' must be an object with 'name' and 'version'.", owner)
		return
	}
	s.checkKeys(framework, owner+" framework", []string{"name", "version"})
	s.checkString(framework, "name", owner+" framework")
	s.checkString(framework, "version", owner+" framework")
}

func (s *scan) validateModels(agent object, owner string) {
	models := objectsIn(agent, "models")
	defaults := 0
	for _, model := range models {
		s.checkKeys(model, owner+" model", []string{"provider", "modelId", "isDefault", "via", "parameters"}, "provider", "modelId", "isDefault")
		s.checkString(model, "provider", owner+" model")
		s.checkString(model, "modelId", owner+" model")
		isDefault, ok := model["isDefault"].(bool)
		if _, present := model["isDefault"]; present && !ok {
			s.problem("%s model: 'isDefault' must be true or false.", owner)
		}
		if isDefault {
			defaults++
		}
		if value, present := model["parameters"]; present {
			if _, ok := value.(object); !ok {
				s.problem("%s model: 'parameters' must be an object.", owner)
			}
		}
	}
	if len(models) > 0 && defaults != 1 {
		s.problem("%s lists models but %d of them has isDefault true; exactly one must.", owner, defaults)
	}
}

func (s *scan) validateSystemPrompt(agent object, owner string) {
	value, present := agent["systemPrompt"]
	if !present {
		return
	}
	prompt, ok := value.(object)
	if !ok {
		s.problem("%s: 'systemPrompt' must be an object with 'text'.", owner)
		return
	}
	s.checkKeys(prompt, owner+" systemPrompt", []string{"text", "via", "templatingVars", "dynamic"}, "text")
	s.checkString(prompt, "text", owner+" systemPrompt")
}

// validateDeployment checks the ROOT-only deployment: a string version and an
// endpoint url without credentials.
func (s *scan) validateDeployment(agent object, owner string) {
	value, present := agent["deployment"]
	if !present {
		return
	}
	if agent["role"] != "ROOT" {
		s.problem("%s is not the ROOT agent but carries 'deployment'; only the ROOT agent may.", owner)
	}
	deployment, ok := value.(object)
	if !ok {
		s.problem("%s: 'deployment' must be an object with 'version' and 'endpoint'.", owner)
		return
	}
	s.checkKeys(deployment, owner+" deployment", []string{"version", "endpoint"})
	if version, present := deployment["version"]; present {
		if _, ok := version.(string); !ok {
			s.problem("%s: deployment.version must be a string, for example \"42\".", owner)
		}
	}
	endpoint, present := deployment["endpoint"]
	if !present {
		return
	}
	endpointObject, ok := endpoint.(object)
	if !ok {
		s.problem("%s: deployment.endpoint must be an object with 'url' and optionally 'name'.", owner)
		return
	}
	s.checkKeys(endpointObject, owner+" deployment.endpoint", []string{"url", "name"})
	s.checkString(endpointObject, "name", owner+" deployment.endpoint")
	address, ok := endpointObject["url"].(string)
	switch {
	case !ok:
		s.problem("%s: deployment.endpoint.url must be a string.", owner)
	case !validURLWithoutCredentials(address):
		s.problem("%s: deployment.endpoint.url must be an address without a user or password before '@'.", owner)
	}
}

var toolKeys = []string{"name", "contentHash", "definitionPattern", "origin", "args", "guardrails", "usedByAgents", "via", "description", "returns"}

func (s *scan) validateTool(tool object) {
	owner := "tool '" + name(tool) + "'"
	s.checkKeys(tool, owner, toolKeys, "name", "definitionPattern", "origin")
	s.checkString(tool, "definitionPattern", owner)
	s.checkString(tool, "description", owner)
	s.checkNames(tool, "usedByAgents", owner)
	for _, arg := range objectsIn(tool, "args") {
		s.checkString(arg, "name", owner+" argument")
		if value, present := arg["type"]; present && value != nil {
			s.checkString(arg, "type", owner+" argument")
		}
		if _, ok := arg["required"].(bool); !ok {
			s.problem("%s argument '%v' needs 'required' as true or false.", owner, arg["name"])
		}
	}
	if items, ok := tool["args"].([]any); ok {
		for _, item := range items {
			if _, ok := item.(object); !ok {
				s.problem("%s: every entry of 'args' must be an object.", owner)
			}
		}
	}
	if returns, present := tool["returns"]; present {
		if returnsObject, ok := returns.(object); !ok {
			s.problem("%s: 'returns' must be an object with 'type'.", owner)
		} else if value, present := returnsObject["type"]; present && value != nil {
			s.checkString(returnsObject, "type", owner+" returns")
		}
	}
	for _, guardrail := range objectsIn(tool, "guardrails") {
		guardOwner := owner + " guardrail"
		s.checkKeys(guardrail, guardOwner, []string{"kind", "description", "argsInvolved", "behavior", "confidence", "via"}, "kind", "description", "behavior")
		s.checkString(guardrail, "kind", guardOwner)
		s.checkString(guardrail, "behavior", guardOwner)
		s.checkString(guardrail, "description", guardOwner)
		s.checkNames(guardrail, "argsInvolved", guardOwner)
	}
}

func (s *scan) validatePlatform() {
	value, present := s.platform, s.platform != nil
	if !present {
		return
	}
	platform, ok := value.(object)
	if !ok {
		s.problem("platform must be an object with only 'name' and 'metadata'.")
		return
	}
	s.checkKeys(platform, "platform", []string{"name", "metadata"}, "name")
	if platformName, _ := platform["name"].(string); !upperSnakeCase.MatchString(platformName) {
		s.problem("platform.name '%v' must be UPPER_SNAKE_CASE, for example DATABRICKS.", platform["name"])
	}
	if metadata, present := platform["metadata"]; present && metadata != nil {
		if _, ok := metadata.(object); !ok {
			s.problem("platform.metadata must be an object.")
		}
	}
}

func (s *scan) validateSource() {
	if s.source == nil {
		return
	}
	source, ok := s.source.(object)
	if !ok {
		s.problem("source must be an object with 'repoUrl' and 'commitSha'.")
		return
	}
	s.checkKeys(source, "source", []string{"repoUrl", "commitSha"})
	s.checkString(source, "repoUrl", "source")
	s.checkString(source, "commitSha", "source")
}

// checkKeys reports unknown keys (the schema refuses them) and missing required ones.
func (s *scan) checkKeys(o object, owner string, allowed []string, required ...string) {
	var unknown []string
	for key := range o {
		if !slices.Contains(allowed, key) {
			unknown = append(unknown, key)
		}
	}
	slices.Sort(unknown)
	for _, key := range unknown {
		s.problem("%s has an unknown field '%s', which Collibra refuses. Allowed: %s.", owner, key, strings.Join(allowed, ", "))
	}
	for _, key := range required {
		if _, present := o[key]; !present {
			s.problem("%s is missing the required field '%s'.", owner, key)
		}
	}
}

// checkString reports a field that is present but not a string.
func (s *scan) checkString(o object, key, owner string) {
	if value, present := o[key]; present {
		if _, ok := value.(string); !ok {
			s.problem("%s: '%s' must be a string.", owner, key)
		}
	}
}

// checkNames reports a list of names with an entry that isn't a string.
func (s *scan) checkNames(o object, key, owner string) {
	items, _ := o[key].([]any)
	for _, item := range items {
		if _, ok := item.(string); !ok {
			s.problem("%s: every entry of '%s' must be a name (a string).", owner, key)
			return
		}
	}
}

// objectsIn returns the objects of a list that normalise already made a list.
func objectsIn(o object, key string) []object {
	items, _ := o[key].([]any)
	out := make([]object, 0, len(items))
	for _, item := range items {
		if entry, ok := item.(object); ok {
			out = append(out, entry)
		}
	}
	return out
}
