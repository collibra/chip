package finalize_agent_scan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// The reference examples V1 to V7 are copied from the agent-scanner skill's
// references/finalize-procedure.md, with the hashes it lists for each.
var referenceExamples = []struct {
	name   string
	agents map[string]string
	tools  map[string]string // "" for a tool with no hash (mcp_dynamic)
	doc    string
}{
	{"v1", map[string]string{"triage_agent": "006d726e0127bdfec3eecf519e3f17872720c20b6cdc69f6bad9ac960cb871c1"},
		map[string]string{"search_kb": "a5803c8312657afe9d0aab91c502ce8573e4d90b2ab2af9661e8f25e16c430a4"},
		"4453028e53aab59c475ed7ea85f6bfddd6c5fd7b0c9cf06f65e2e5fea5803eb4"},
	{"v2", map[string]string{"planner_agent": "74f91f4114364655ec52fd41e159b5753d0f65d38685b65b79b01a743a1e7370", "worker_agent": "6e8d9febeb18d4fece218ba2d53c7b5e558ffb4862f1b5bcb2dd381723120908"},
		nil, "d5058d00172ea6182f49f812adbcc66431a78ada30942f4ab582e1c5444fdcde"},
	{"v3", map[string]string{"alpha_agent": "45ee6259bb29b28527eb45ec4cf40329c5a207a9f6a316c942fa7947198a3111", "beta_agent": "f1eb32bdedf7862b974732f13084e78d0ebe6ba917d965aa8fe1abb26e1d4d5c"},
		nil, "171178c09fa2f9413350464cb1e94df27fbb0962f810fe3d4327f9350fb6be64"},
	{"v4", map[string]string{"ops_agent": "0e578797afe65a1ad3f1ff253bd1eaf60f05a9d0ece710bbed7b49c16f08fdc2"},
		map[string]string{"zip_files": "86fb14bbc79f488080641790e47b186a10d32758fd1c7799bbd46ff79a02b407", "list_files": ""},
		"c978f550b9892199d07d51fc14a323bc2d8c08ccb69d94314d7c264f9bfbf32f"},
	{"v5", map[string]string{"lead_agent": "63c39683a704ab0ff8feec4e54b90a99e9742e44aceac37010934f7444be9a90", "aide_agent": "657bdbcaf94453375a19579ffc2e29a480d3bb85ce1c050ade29cd08bcddc1d6"},
		nil, "7fcc9a5ac48310ba9940f4d410a0bd1a22e0e7736442c2b62c9c2806e64127f8"},
	{"v6", map[string]string{"alpha_agent": "45ee6259bb29b28527eb45ec4cf40329c5a207a9f6a316c942fa7947198a3111", "beta_agent": "32621094d46710cc5fc1ed8e169f99ea85ba5ba91740c53a7cce051022d83460", "gamma_agent": "dfc12ef4aa1d8f6bc82b23fe0341da989c3b05d1df9dab98adddeaa515fa48b2"},
		nil, "def5cc418a76377cd54c80f3da324bc2bcea05cd60c7b3da4ec404592b419928"},
	{"v7", map[string]string{"triage_agent": "006d726e0127bdfec3eecf519e3f17872720c20b6cdc69f6bad9ac960cb871c1"},
		map[string]string{"search_kb": "a5803c8312657afe9d0aab91c502ce8573e4d90b2ab2af9661e8f25e16c430a4"},
		"f6fa138efb728fc823820440ad621f6f571215ff9dfc32251ae50cb289c8cfdd"},
}

func TestReferenceExamplesHashAsTheProcedureSays(t *testing.T) {
	for _, example := range referenceExamples {
		t.Run(example.name, func(t *testing.T) {
			out := run(t, readTestdata(t, example.name+"-extraction.json"))
			if out.Status != StatusSuccess {
				t.Fatalf("status %s, problems %v", out.Status, out.Problems)
			}
			if out.ContentHash != example.doc {
				t.Errorf("document hash %s, want %s", out.ContentHash, example.doc)
			}
			document := decode(t, out.Document)
			for _, agent := range document["agents"].([]any) {
				a := agent.(map[string]any)
				if got, want := a["contentHash"], example.agents[a["name"].(string)]; got != want {
					t.Errorf("agent %s hash %v, want %s", a["name"], got, want)
				}
			}
			for _, tool := range document["tools"].([]any) {
				tl := tool.(map[string]any)
				want := example.tools[tl["name"].(string)]
				if got, _ := tl["contentHash"].(string); got != want {
					t.Errorf("tool %s hash %v, want %q", tl["name"], tl["contentHash"], want)
				}
			}
		})
	}
}

func TestReferenceDocumentsMatchWholly(t *testing.T) {
	for _, example := range []string{"v1", "v7"} {
		t.Run(example, func(t *testing.T) {
			out := run(t, readTestdata(t, example+"-extraction.json"))
			got := decode(t, out.Document)
			if got["generatedOn"] != "2026-11-12T10:20:30Z" {
				t.Errorf("generatedOn %v, want whole seconds in UTC", got["generatedOn"])
			}
			delete(got, "generatedOn")
			want := decode(t, readTestdata(t, example+"-document.json"))
			if !reflect.DeepEqual(got, want) {
				t.Errorf("document differs from the procedure's expected document\n got %s", out.Document)
			}
		})
	}
}

func TestSuggestedAgentID(t *testing.T) {
	if got := run(t, readTestdata(t, "v7-extraction.json")).SuggestedAgentID; got != "git@example.com:acme/agents.git#triage_agent" {
		t.Errorf("with a source: %q", got)
	}
	if got := run(t, readTestdata(t, "v1-extraction.json")).SuggestedAgentID; got != "triage_agent" {
		t.Errorf("without a source: %q", got)
	}
}

func TestNormaliseLowercasesDropsEvidenceAndFillsLists(t *testing.T) {
	out := run(t, []byte(`{"agents":[{"name":"main_agent","role":"ROOT","pattern":"create_agent","framework":{"name":"LangGraph"},
		"models":[{"provider":"OpenAI","modelId":"m","isDefault":true}],"middleware":[{"class":"C","hook":"WRAP"}],"evidence":["a.py:1"],
		"modelsResolved":true,"toolsResolved":true,"subagentsResolved":true,"middlewareResolved":true}],
		"tools":[{"name":"lookup","definitionPattern":"decorator_tool","origin":{"type":"decorator_tool"},"evidence":["a.py:2"],
		"guardrails":[{"kind":"Size_Limit","description":"d","behavior":"RAISES","confidence":0.9}]}],"warnings":[]}`))
	if out.Status != StatusSuccess {
		t.Fatalf("problems %v", out.Problems)
	}
	document := string(out.Document)
	for _, want := range []string{`"name": "langgraph"`, `"provider": "openai"`, `"hook": "wrap"`, `"kind": "size_limit"`, `"behavior": "raises"`, `"subagents": []`, `"args": []`, `"argsInvolved": []`, `"usedByAgents": []`} {
		if !strings.Contains(document, want) {
			t.Errorf("document lacks %s", want)
		}
	}
	if strings.Contains(document, "evidence") {
		t.Error("evidence was not removed")
	}
	if strings.Count(document, `"via": "LLM_FALLBACK"`) != 5 {
		t.Errorf("via should be on the agent, model, middleware, tool and guardrail:\n%s", document)
	}
}

func TestRepairsAddWarnings(t *testing.T) {
	out := run(t, []byte(`{"agents":[{"name":"main_agent","role":"ROOT","pattern":"p","tools":["lookup","ghost_tool"],"subagents":["ghost_agent"],
		"modelsResolved":true,"toolsResolved":true,"subagentsResolved":true,"middlewareResolved":true,
		"deployment":{"version":" ","endpoint":{"name":"ep"}}}],
		"tools":[{"name":"lookup","definitionPattern":"decorator_tool","origin":{},"usedByAgents":["main_agent","nobody"],
		"guardrails":[{"kind":"allowlist","description":"d","behavior":"raises"}]}],"warnings":["mine"]}`))
	if out.Status != StatusSuccess {
		t.Fatalf("problems %v", out.Problems)
	}
	want := []string{
		"mine",
		"deployment.endpoint had no url and was dropped",
		"main_agent.tools referenced 'ghost_tool', which matches no known name; dropped",
		"main_agent.subagents referenced 'ghost_agent', which matches no known name; dropped",
		"lookup.usedByAgents referenced 'nobody', which matches no known name; dropped",
		"guardrail 'allowlist' on 'lookup' had no confidence; set to 0.5",
	}
	if !slices.Equal(out.Warnings, want) {
		t.Errorf("warnings\n got %q\nwant %q", out.Warnings, want)
	}
	agent := decode(t, out.Document)["agents"].([]any)[0].(map[string]any)
	if _, present := agent["deployment"]; present {
		t.Error("an empty deployment should be removed")
	}
	if !strings.Contains(string(out.Document), `"confidence": 0.5`) {
		t.Error("confidence 0.5 not set")
	}
}

func TestRejectsListEveryProblem(t *testing.T) {
	out := run(t, []byte(`{"agents":[
		{"name":"ab","role":"BOSS","pattern":"p","models":[{"provider":"x","modelId":"m","isDefault":false}],"deploymentVersion":"1",
		 "modelsResolved":true,"toolsResolved":true,"subagentsResolved":true},
		{"name":"helper","role":"SUBAGENT","pattern":"p","deployment":{"version":2,"endpoint":{"url":"https://u:p@host/x"}},"oops":1,
		 "modelsResolved":true,"toolsResolved":true,"subagentsResolved":true,"middlewareResolved":true}],
		"tools":[{"name":"dup","definitionPattern":"d","origin":{}},{"name":"dup","definitionPattern":"d","origin":{}}],
		"warnings":[],"platform":{"name":"databricks"}}`))
	if out.Status != StatusValidationError {
		t.Fatalf("status %s, want validation_error", out.Status)
	}
	if out.Document != nil {
		t.Error("a rejected extraction must not produce a document")
	}
	for _, want := range []string{
		"agent name 'ab' is shorter than 3 characters",
		"two tools share the name 'dup'",
		"has role 'BOSS'",
		"exactly one agent must have role ROOT; found 0",
		"lists models but 0 of them has isDefault true",
		"needs 'middlewareResolved' as true or false",
		"old field 'deploymentVersion'",
		"is not the ROOT agent but carries 'deployment'",
		"deployment.version must be a string",
		"without a user or password",
		"unknown field 'oops'",
		"platform.name 'databricks' must be UPPER_SNAKE_CASE",
	} {
		if !containsAny(out.Problems, want) {
			t.Errorf("no problem mentions %q; got %q", want, out.Problems)
		}
	}
}

func TestRejectsNonObjectInput(t *testing.T) {
	for _, input := range []string{``, `null`, `[]`, `"{}"`, `{"agents":{},"tools":[]}`} {
		out := run(t, []byte(input))
		if out.Status != StatusValidationError {
			t.Errorf("%q: status %s, want validation_error", input, out.Status)
		}
	}
}

func TestFormatValue(t *testing.T) {
	s := &scan{}
	for _, tc := range []struct {
		json string
		want string
	}{
		{`5`, "5"}, {`5.0`, "5"}, {`0`, "0"}, {`-0.0`, "0"}, {`0.7`, "0.7"}, {`1024`, "1024"}, {`0.0001`, "0.0001"},
		{`"max"`, "max"}, {`false`, "false"}, {`["a","b"]`, "[a b]"}, {`[1,2.5]`, "[1 2.5]"},
		{`{"b":1,"a":"x"}`, "map[a:x b:1]"},
	} {
		var value any
		decoder := json.NewDecoder(strings.NewReader(tc.json))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			t.Fatal(err)
		}
		if got := s.formatValue(value, "test"); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.json, got, tc.want)
		}
	}
	if len(s.warnings) != 0 {
		t.Errorf("unexpected warnings %q", s.warnings)
	}
	s.formatValue(json.Number("1e-07"), "a default")
	if len(s.warnings) != 1 || !strings.Contains(s.warnings[0], "1e-07 in a default") {
		t.Errorf("exponent number should warn, got %q", s.warnings)
	}
}

func run(t *testing.T, extraction []byte) Output {
	t.Helper()
	original := now
	now = func() time.Time { return time.Date(2026, 11, 12, 11, 20, 30, 999, time.FixedZone("CET", 3600)) }
	t.Cleanup(func() { now = original })
	out, err := handler(t.Context(), Input{Extraction: extraction})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func readTestdata(t *testing.T, file string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", file))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func decode(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, data)
	}
	return out
}

func containsAny(items []string, part string) bool {
	return slices.ContainsFunc(items, func(item string) bool { return strings.Contains(item, part) })
}
