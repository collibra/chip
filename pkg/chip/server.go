package chip

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"log/slog"
	"reflect"
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
			p := store.get()
			if ss, ok := req.GetSession().(*mcp.ServerSession); ok {
				if sp := ss.InitializeParams(); sp != nil && sp.ClientInfo != nil {
					p = sp
				}
			}
			if p != nil {
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
	Title       string
	Description string
	Handler     ToolHandlerFunc[In, Out]
	Permissions []string
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
			// Make nil slices/maps marshal as []/{} so output conforms to the
			// concrete-typed schema stripNullableTypes produces (see
			// normalizeNilCollections).
			normalizeNilCollections(reflect.ValueOf(&capturedOutput).Elem())
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
		Title:        tool.Title,
		Description:  tool.Description,
		InputSchema:  buildSchema[In](),
		OutputSchema: buildSchema[Out](),
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
	stripNullableTypes(inputSchema)
	return inputSchema
}

// stripNullableTypes rewrites nullable type unions (e.g. ["null","array"])
// emitted by the reflector into a single concrete type ("array"). jsonschema.For
// marks every nilable Go type — slices, maps, pointers — as nullable, producing
// a `"type": ["null", T]` union. Some MCP clients (notably the Claude desktop
// app) fail to recognise such a union as a structured type and serialise the
// argument to a JSON string instead, which the server then rejects with
// `has type "string", want one of "null, array"`. chip never expects an explicit
// null — optional fields are simply omitted — so collapsing the union to its
// concrete type is safe and makes the schema portable across clients.
//
// This also narrows output schemas to concrete types; normalizeNilCollections
// is the counterpart that keeps output DATA (nil slices/maps) conforming.
func stripNullableTypes(s *jsonschema.Schema) {
	if s == nil {
		return
	}
	if len(s.Types) > 0 {
		filtered := slices.DeleteFunc(slices.Clone(s.Types), func(t string) bool { return t == "null" })
		switch len(filtered) {
		case 0:
			// Was ["null"] only — nothing concrete to keep; leave untouched.
		case 1:
			s.Type, s.Types = filtered[0], nil
		default:
			s.Types = filtered
		}
	}

	stripNullableTypes(s.Items)
	stripNullableTypes(s.AdditionalProperties)
	stripNullableTypes(s.Not)
	for _, child := range s.Properties {
		stripNullableTypes(child)
	}
	for _, child := range s.PrefixItems {
		stripNullableTypes(child)
	}
	for _, child := range s.Defs {
		stripNullableTypes(child)
	}
	for _, child := range s.AllOf {
		stripNullableTypes(child)
	}
	for _, child := range s.AnyOf {
		stripNullableTypes(child)
	}
	for _, child := range s.OneOf {
		stripNullableTypes(child)
	}
}

// normalizeNilCollections replaces nil slices with [] and nil maps with {} in a
// tool's output, so the output always matches its declared schema.
//
// Why this is needed: chip builds every tool's schema from its Go types, and
// stripNullableTypes describes each list/map as a plain "array"/"object" rather
// than "this type or null". That was added so some MCP clients (e.g. Cowork)
// would stop mis-sending list *arguments* — an input-side fix.
//
// The side effect: the same rule also applies to *output* schemas, which now
// disallow null. But an unset Go slice/map serializes to null, so any tool that
// returns no results (or returns early with an error) emits null and fails its
// own output validation with `has type "null", want "array"`.
//
// The fix is to correct the data, not loosen the schema (loosening it would
// undo the input fix). We walk the output once and turn every nil slice/map
// into an empty one. Empty collections still satisfy `omitempty`, so optional
// fields stay omitted and the schema clients see is unchanged. Doing it here
// covers every tool — current and future — so no handler has to remember to
// initialise its slices.
func normalizeNilCollections(v reflect.Value) {
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			normalizeNilCollections(v.Elem())
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if f := v.Field(i); f.CanSet() {
				normalizeNilCollections(f)
			}
		}
	case reflect.Slice:
		if v.IsNil() {
			if v.CanSet() {
				v.Set(reflect.MakeSlice(v.Type(), 0, 0))
			}
			return
		}
		for i := 0; i < v.Len(); i++ {
			normalizeNilCollections(v.Index(i))
		}
	case reflect.Map:
		if v.IsNil() && v.CanSet() {
			v.Set(reflect.MakeMap(v.Type()))
		}
		// ponytail: map values returned by reflect aren't addressable, so a nil
		// slice/map nested *inside* a map value can't be normalised in place. No
		// tool output nests collections that way today; revisit if one does.
	}
}
