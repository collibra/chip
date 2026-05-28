package chip

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"log/slog"
	"slices"
	"strings"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

//go:embed instructions.md
var instructions string

type ToolHandlerFunc[In, Out any] func(ctx context.Context, input In) (Out, error)

type CallToolFunc func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error)

type ToolMiddleware interface {
	ToolHandle(ctx context.Context, toolRequest *mcp.CallToolRequest, next CallToolFunc) (*mcp.CallToolResult, error)
}

type ToolMiddlewareFunc func(ctx context.Context, toolRequest *mcp.CallToolRequest, next CallToolFunc) (*mcp.CallToolResult, error)

func (f ToolMiddlewareFunc) ToolHandle(ctx context.Context, toolRequest *mcp.CallToolRequest, next CallToolFunc) (*mcp.CallToolResult, error) {
	return f(ctx, toolRequest, next)
}

// initParamsStore holds the last InitializeParams received from a client.
// In stateless HTTP mode each call gets a fresh session, so the params from
// the initial handshake are captured here and re-injected into the per-request
// context by the receiving middleware.
type initParamsStore struct {
	mu     sync.RWMutex
	params *mcp.InitializeParams
}

func (s *initParamsStore) set(p *mcp.InitializeParams) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.params = p
}

func (s *initParamsStore) get() *mcp.InitializeParams {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.params
}

type Server struct {
	toolMiddlewares  []ToolMiddleware
	toolMetadata     map[string]*ToolMetadata
	instructionParts []string
	mcp.Server
}

func NewServer(opts ...ServerOption) *Server {
	s := &Server{
		toolMiddlewares:  []ToolMiddleware{},
		toolMetadata:     make(map[string]*ToolMetadata),
		instructionParts: []string{instructions},
	}

	for _, opt := range opts {
		opt(s)
	}

	s.Server = *mcp.NewServer(&mcp.Implementation{
		Name:    "Collibra MCP server",
		Title:   "Collibra Data Intelligence Platform MCP Server",
		Version: Version,
	}, &mcp.ServerOptions{
		Instructions: joinInstructions(s.instructionParts),
	})

	store := &initParamsStore{}
	s.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "initialize" {
				if params, ok := req.GetParams().(*mcp.InitializeParams); ok {
					store.set(params)
				}
			}
			if p := store.get(); p != nil {
				ctx = SetInitParams(ctx, p)
			}
			return next(ctx, method, req)
		}
	})

	return s
}

func joinInstructions(parts []string) string {
	return strings.Join(parts, "\n\n")
}

// GetToolMetadata returns the metadata for a given tool
func (s *Server) GetToolMetadata(toolName string) *ToolMetadata {
	return s.toolMetadata[toolName]
}

// ToolMetadata stores metadata about a registered tool
type ToolMetadata struct {
	Name        string
	Permissions []string
}

// ServerToolConfig is used to configure which tools are enabled/disabled at the server level
type ServerToolConfig struct {
	EnabledTools  []string
	DisabledTools []string
	// EnableDebugTools, when true, registers debug tools that are otherwise hidden.
	EnableDebugTools bool
	Experimental     []string
	// SkillsDir is the optional path to an external skills directory whose
	// contents are merged on top of the embedded catalog. Empty means the
	// embedded catalog alone is served. Only consulted when the "skills"
	// experimental feature is enabled.
	SkillsDir string
}

func (tc *ServerToolConfig) IsToolEnabled(toolName string) bool {
	if slices.Contains(tc.DisabledTools, toolName) {
		return false
	}
	if len(tc.EnabledTools) > 0 {
		return slices.Contains(tc.EnabledTools, toolName)
	}
	return true
}

// IsExperimentalEnabled reports whether the given experimental feature
// name was opted into via --experimental, COLLIBRA_MCP_EXPERIMENTAL, or
// mcp.experimental in the YAML config.
func (tc *ServerToolConfig) IsExperimentalEnabled(featureName string) bool {
	return slices.Contains(tc.Experimental, featureName)
}

type ServerOption func(*Server)

func WithToolMiddleware(middleware ToolMiddleware) ServerOption {
	return func(s *Server) {
		s.toolMiddlewares = append(s.toolMiddlewares, middleware)
	}
}

// WithInstructions appends a snippet to the server's initialize instructions.
// Use this so optional features (e.g. experimental skills) can contribute
// their own bootstrap text only when enabled.
func WithInstructions(snippet string) ServerOption {
	return func(s *Server) {
		if snippet != "" {
			s.instructionParts = append(s.instructionParts, snippet)
		}
	}
}

// WithReplacementInstructions replaces the server's default initialize
// instructions with the given text, discarding any previously appended parts
// (including the embedded default). Use this when an optional feature owns
// the entire bootstrap surface — e.g. the experimental skills feature, which
// routes the model through skill discovery instead of carrying workflow
// recipes in instructions.
func WithReplacementInstructions(text string) ServerOption {
	return func(s *Server) {
		if text == "" {
			return
		}
		s.instructionParts = []string{text}
	}
}

type Tool[In, Out any] struct {
	Name        string
	Description string
	Handler     ToolHandlerFunc[In, Out]
	Permissions []string
	Annotations *mcp.ToolAnnotations
}

func RegisterTool[In, Out any](s *Server, tool *Tool[In, Out]) {
	slog.Info(fmt.Sprintf("Registering tool: %s", tool.Name))

	// Store tool metadata
	s.toolMetadata[tool.Name] = &ToolMetadata{
		Name:        tool.Name,
		Permissions: tool.Permissions,
	}

	handler := func(ctx context.Context, toolRequest *mcp.CallToolRequest, input In) (*mcp.CallToolResult, Out, error) {
		var capturedOutput Out

		middlewareChain := func(ctx context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			out, err := tool.Handler(ctx, input)
			if err != nil {
				slog.ErrorContext(ctx, "error while calling tool function", "error", err)
			}
			capturedOutput = out
			return nil, err
		}

		for i := len(s.toolMiddlewares) - 1; i >= 0; i-- {
			mw := s.toolMiddlewares[i]
			next := middlewareChain
			middlewareChain = func(ctx context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				return mw.ToolHandle(ctx, r, next)
			}
		}

		ctx = SetCallToolRequest(ctx, toolRequest)
		res, err := middlewareChain(ctx, toolRequest)

		return res, capturedOutput, err
	}

	mcp.AddTool(&s.Server, &mcp.Tool{
		Name:         tool.Name,
		Description:  tool.Description,
		InputSchema:  buildSchema[In](),
		OutputSchema: buildSchema[Out](),
		Annotations:  tool.Annotations,
	}, handler)
}

func buildSchema[Schema any]() *jsonschema.Schema {
	inputSchema, err := jsonschema.For[Schema](nil)
	if err != nil {
		log.Fatal(err)
	}
	if inputSchema == nil {
		log.Fatalf("jsonschema.For returned nil schema for %T", *new(Schema))
	}
	inputSchema.AdditionalProperties = nil
	// jsonschema.For emits `type: ["null", "array"]` for every Go slice
	// because a nil slice is a valid value at the Go level. Some MCP
	// clients (notably the Claude Code harness) only recognize a singular
	// `type` and otherwise stringify the value, which then fails our
	// server-side validation. Drop "null" from any type union so the
	// remaining single type is emitted as a plain string.
	stripNullFromTypeUnions(inputSchema)
	return inputSchema
}

// stripNullFromTypeUnions walks the schema and collapses any
// `Types: [..., "null", ...]` union by dropping "null", moving a sole
// remaining type into Type. Slices and pointers are the only Go shapes
// that produce such unions in practice; dropping "null" loses nothing
// for LLM-facing input schemas because the harness can simply omit the
// property when there is no value to send.
func stripNullFromTypeUnions(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	if len(s.Types) > 0 {
		kept := s.Types[:0:0]
		for _, t := range s.Types {
			if t != "null" {
				kept = append(kept, t)
			}
		}
		switch len(kept) {
		case 0:
			s.Types = nil
		case 1:
			s.Type = kept[0]
			s.Types = nil
		default:
			s.Types = kept
		}
	}

	stripNullFromTypeUnions(s.Items)
	for _, sub := range s.ItemsArray {
		stripNullFromTypeUnions(sub)
	}
	for _, sub := range s.PrefixItems {
		stripNullFromTypeUnions(sub)
	}
	stripNullFromTypeUnions(s.AdditionalItems)
	stripNullFromTypeUnions(s.Contains)
	stripNullFromTypeUnions(s.UnevaluatedItems)
	for _, sub := range s.Properties {
		stripNullFromTypeUnions(sub)
	}
	for _, sub := range s.PatternProperties {
		stripNullFromTypeUnions(sub)
	}
	stripNullFromTypeUnions(s.AdditionalProperties)
	stripNullFromTypeUnions(s.PropertyNames)
	stripNullFromTypeUnions(s.UnevaluatedProperties)
	for _, sub := range s.AllOf {
		stripNullFromTypeUnions(sub)
	}
	for _, sub := range s.AnyOf {
		stripNullFromTypeUnions(sub)
	}
	for _, sub := range s.OneOf {
		stripNullFromTypeUnions(sub)
	}
	stripNullFromTypeUnions(s.Not)
	stripNullFromTypeUnions(s.If)
	stripNullFromTypeUnions(s.Then)
	stripNullFromTypeUnions(s.Else)
	for _, sub := range s.DependentSchemas {
		stripNullFromTypeUnions(sub)
	}
	for _, sub := range s.Defs {
		stripNullFromTypeUnions(sub)
	}
	stripNullFromTypeUnions(s.ContentSchema)
}
