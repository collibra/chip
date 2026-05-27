package chip

import (
	"context"
	"log"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type toolInput struct {
	Input string `json:"input" jsonschema:"the input to tool"`
}

type toolOutput struct {
	Output string `json:"output" jsonschema:"the output from tool"`
}

func newTool() *Tool[toolInput, toolOutput] {
	return &Tool[toolInput, toolOutput]{
		Name:        "the_tool",
		Description: "The tool.",
		Handler:     handleTool(),
	}
}

func handleTool() ToolHandlerFunc[toolInput, toolOutput] {
	return func(ctx context.Context, input toolInput) (toolOutput, error) {
		return toolOutput{Output: input.Input}, nil
	}
}

func TestTool_SchemaValidationReturnsToolExecutionError(t *testing.T) {
	chipServer := NewServer()
	RegisterTool[toolInput, toolOutput](chipServer, newTool())
	chipSession := newChipSession(t.Context(), chipServer)
	defer closeSilently(chipSession)

	res, err := chipSession.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "the_tool",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("expected tool execution error, got protocol error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected isError: true for missing required field")
	}
	if len(res.Content) == 0 {
		t.Fatal("expected content describing the validation failure")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	if !strings.Contains(text.Text, "input") {
		t.Fatalf("expected error mentioning missing field 'input', got %q", text.Text)
	}
}

func TestTool_WrongTypeReturnsToolExecutionError(t *testing.T) {
	chipServer := NewServer()
	RegisterTool[toolInput, toolOutput](chipServer, newTool())
	chipSession := newChipSession(t.Context(), chipServer)
	defer closeSilently(chipSession)

	res, err := chipSession.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "the_tool",
		Arguments: map[string]any{"input": 123},
	})
	if err != nil {
		t.Fatalf("expected tool execution error, got protocol error: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected isError: true for wrong-typed field")
	}
}

func TestServer_InitializeResponseIncludesInstructions(t *testing.T) {
	chipServer := NewServer()
	chipSession := newChipSession(t.Context(), chipServer)
	defer closeSilently(chipSession)

	init := chipSession.InitializeResult()
	if init == nil {
		t.Fatal("expected non-nil InitializeResult")
	}
	if init.Instructions == "" {
		t.Fatal("expected non-empty instructions in initialize response")
	}
	if !strings.Contains(init.Instructions, "Collibra") {
		t.Fatalf("expected instructions to mention Collibra, got %q", init.Instructions)
	}
}

func TestServer_WithInstructionsAppendsToDefault(t *testing.T) {
	const snippet = "## Extra section\n\nappended content marker"
	chipServer := NewServer(WithInstructions(snippet))
	chipSession := newChipSession(t.Context(), chipServer)
	defer closeSilently(chipSession)

	got := chipSession.InitializeResult().Instructions
	if !strings.Contains(got, "Collibra") {
		t.Errorf("expected default instructions preserved, got %q", got)
	}
	if !strings.Contains(got, "appended content marker") {
		t.Errorf("expected appended snippet present, got %q", got)
	}
}

func TestServer_WithReplacementInstructionsReplacesDefault(t *testing.T) {
	const replacement = "# Replacement\n\nreplacement marker only"
	chipServer := NewServer(WithReplacementInstructions(replacement))
	chipSession := newChipSession(t.Context(), chipServer)
	defer closeSilently(chipSession)

	got := chipSession.InitializeResult().Instructions
	if got != replacement {
		t.Errorf("expected replacement text only, got %q", got)
	}
}

func TestServer_WithReplacementInstructionsEmptyKeepsDefault(t *testing.T) {
	chipServer := NewServer(WithReplacementInstructions(""))
	chipSession := newChipSession(t.Context(), chipServer)
	defer closeSilently(chipSession)

	got := chipSession.InitializeResult().Instructions
	if !strings.Contains(got, "Collibra") {
		t.Errorf("expected default instructions preserved when replacement is empty, got %q", got)
	}
}

func TestTool_IgnoreUnknownFields(t *testing.T) {
	chipServer := NewServer()
	RegisterTool[toolInput, toolOutput](chipServer, newTool())
	chipSession := newChipSession(t.Context(), chipServer)
	defer closeSilently(chipSession)

	_, err := chipSession.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "the_tool",
		Arguments: map[string]any{
			"input":          "Echo this message",
			"tool_reasoning": "This should be ignored",
		},
	})
	if err != nil {
		t.Fatalf("should not return error when extra fields in arguments: %v", err)
	}
}

func TestServerToolConfig_IsToolEnabled(t *testing.T) {
	cases := []struct {
		name     string
		cfg      ServerToolConfig
		tool     string
		expected bool
	}{
		{"empty config enables everything", ServerToolConfig{}, "foo", true},
		{"explicitly disabled", ServerToolConfig{DisabledTools: []string{"foo"}}, "foo", false},
		{"allow-list excludes others", ServerToolConfig{EnabledTools: []string{"bar"}}, "foo", false},
		{"allow-list includes self", ServerToolConfig{EnabledTools: []string{"foo"}}, "foo", true},
		{"disabled wins over enabled", ServerToolConfig{EnabledTools: []string{"foo"}, DisabledTools: []string{"foo"}}, "foo", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.cfg.IsToolEnabled(tc.tool); got != tc.expected {
				t.Fatalf("IsToolEnabled(%q) = %v, want %v", tc.tool, got, tc.expected)
			}
		})
	}
}

func TestServer_InitParamsAvailableOnToolContext(t *testing.T) {
	chipServer := NewServer()
	var captured *mcp.InitializeParams
	RegisterTool(chipServer, &Tool[toolInput, toolOutput]{
		Name:        "capture_init",
		Description: "Captures init params for testing.",
		Handler: func(ctx context.Context, _ toolInput) (toolOutput, error) {
			p, _ := GetInitParams(ctx)
			captured = p
			return toolOutput{}, nil
		},
	})
	chipSession := newChipSession(t.Context(), chipServer)
	defer closeSilently(chipSession)

	if _, err := chipSession.CallTool(t.Context(), &mcp.CallToolParams{
		Name:      "capture_init",
		Arguments: map[string]any{"input": "x"},
	}); err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if captured == nil {
		t.Fatal("expected init params to be captured on tool context")
	}
	if captured.ClientInfo == nil {
		t.Fatal("expected ClientInfo on captured init params")
	}
	if captured.ClientInfo.Name != "client" {
		t.Fatalf("expected ClientInfo.Name=client, got %q", captured.ClientInfo.Name)
	}
	if captured.ClientInfo.Version != "v0.0.1" {
		t.Fatalf("expected ClientInfo.Version=v0.0.1, got %q", captured.ClientInfo.Version)
	}
}

func newChipSession(ctx context.Context, chipServer *Server) *mcp.ClientSession {
	t1, t2 := mcp.NewInMemoryTransports()
	if _, err := chipServer.Connect(ctx, t1, nil); err != nil {
		log.Fatal(err)
	}
	chipClient := mcp.NewClient(&mcp.Implementation{Name: "client", Version: "v0.0.1"}, nil)
	chipSession, err := chipClient.Connect(ctx, t2, nil)
	if err != nil {
		log.Fatal(err)
	}
	return chipSession
}

func closeSilently(session *mcp.ClientSession) {
	_ = session.Close()
}

func TestServerToolConfig_IsExperimentalEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   ServerToolConfig
		feature  string
		expected bool
	}{
		{name: "nil list", config: ServerToolConfig{}, feature: "skills", expected: false},
		{name: "feature listed", config: ServerToolConfig{Experimental: []string{"skills"}}, feature: "skills", expected: true},
		{name: "unrelated feature listed", config: ServerToolConfig{Experimental: []string{"other"}}, feature: "skills", expected: false},
		{name: "multiple features", config: ServerToolConfig{Experimental: []string{"other", "skills"}}, feature: "skills", expected: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.config.IsExperimentalEnabled(tt.feature); got != tt.expected {
				t.Errorf("IsExperimentalEnabled(%q) = %v, want %v", tt.feature, got, tt.expected)
			}
		})
	}
}
