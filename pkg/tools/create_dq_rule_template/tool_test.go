package create_dq_rule_template_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/create_dq_rule_template"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// server mocks the create endpoint plus the DGC asset lookup used to resolve
// business rule links by name. createdBody captures what was POSTed; it stays
// empty when the tool wrote nothing.
type server struct {
	createdBody map[string]any
	createCalls int
}

func newServer(t *testing.T, createCode int, createBody any, assets []map[string]any) (*http.Client, *server) {
	t.Helper()
	state := &server{}
	mux := http.NewServeMux()

	mux.HandleFunc("POST /rest/dq/1.0/ruleTemplates", func(w http.ResponseWriter, r *http.Request) {
		state.createCalls++
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &state.createdBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(createCode)
		_ = json.NewEncoder(w).Encode(createBody)
	})
	mux.HandleFunc("GET /rest/2.0/assetTypes/publicId/{publicId}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "asset-type-uuid", "publicId": r.PathValue("publicId")})
	})
	mux.HandleFunc("GET /rest/2.0/assets", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": assets})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv), state
}

func createdTemplate() map[string]any {
	return map[string]any{
		"id":               "11111111-2222-3333-4444-555555555555",
		"ruleTemplateName": "Row Count Range",
		"description":      "Row count within the learned range",
		"sql":              "select * from @dataset where {{column}} is null",
		"dialect":          "snowflake",
		"dimensions":       []string{"Completeness"},
		"isSystem":         false,
	}
}

func validInput() tools.Input {
	return tools.Input{
		Name:        "Row Count Range",
		SQL:         "select * from @dataset where {{column}} is null",
		Dialect:     "snowflake",
		Dimensions:  []string{"Completeness"},
		Description: "Row count within the learned range",
	}
}

func TestCreateRequiredFields(t *testing.T) {
	client, state := newServer(t, http.StatusCreated, createdTemplate(), nil)
	tool := tools.NewTool(client)

	missing := map[string]func(tools.Input) tools.Input{
		"name":        func(in tools.Input) tools.Input { in.Name = "  "; return in },
		"sql":         func(in tools.Input) tools.Input { in.SQL = ""; return in },
		"dialect":     func(in tools.Input) tools.Input { in.Dialect = ""; return in },
		"description": func(in tools.Input) tools.Input { in.Description = ""; return in },
		"dimensions":  func(in tools.Input) tools.Input { in.Dimensions = nil; return in },
	}
	for field, mutate := range missing {
		out, err := tool.Handler(t.Context(), mutate(validInput()))
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", field, err)
		}
		if out.Status != tools.StatusValidationError {
			t.Errorf("%s: status = %q, want validation_error", field, out.Status)
		}
		if !strings.Contains(out.Message, field) {
			t.Errorf("%s: message = %q, want it to name the offending field", field, out.Message)
		}
	}
	if state.createCalls != 0 {
		t.Errorf("create was called %d times during validation failures, want 0", state.createCalls)
	}
}

func TestCreatePreviewWritesNothing(t *testing.T) {
	client, state := newServer(t, http.StatusCreated, createdTemplate(), nil)

	out, err := tools.NewTool(client).Handler(t.Context(), validInput())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusPreview {
		t.Fatalf("status = %q, want preview (%s)", out.Status, out.Message)
	}
	if state.createCalls != 0 {
		t.Fatalf("create was called %d times without confirm, want 0", state.createCalls)
	}
	if out.Template != nil {
		t.Error("template is set on a preview, want nil")
	}
	preview := out.Preview
	if preview == nil {
		t.Fatal("preview = nil, want the template that would be created")
	}
	// The preview must echo every field that would be written.
	if preview.Name != "Row Count Range" || preview.SQL == "" || preview.Dialect != "snowflake" ||
		preview.Description == "" || len(preview.Dimensions) != 1 {
		t.Errorf("preview = %+v, want every write field echoed", preview)
	}
}

func TestCreateWithConfirmWrites(t *testing.T) {
	client, state := newServer(t, http.StatusCreated, createdTemplate(), nil)

	input := validInput()
	input.Confirm = true
	out, err := tools.NewTool(client).Handler(t.Context(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if state.createCalls != 1 {
		t.Fatalf("create was called %d times, want 1", state.createCalls)
	}
	if out.Template == nil || out.Template.ID != "11111111-2222-3333-4444-555555555555" {
		t.Fatalf("template = %+v, want the created template with its id", out.Template)
	}
	if got := state.createdBody["ruleTemplateName"]; got != "Row Count Range" {
		t.Errorf("posted ruleTemplateName = %v, want Row Count Range", got)
	}
}

func TestCreateResolvesBusinessRuleLinkByName(t *testing.T) {
	assets := []map[string]any{{"id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "displayName": "Email must be valid"}}
	client, state := newServer(t, http.StatusCreated, createdTemplate(), assets)

	input := validInput()
	input.BusinessRuleLinks = []string{"Email must be valid", "99999999-8888-7777-6666-555555555555"}
	input.Confirm = true
	out, err := tools.NewTool(client).Handler(t.Context(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}

	posted, ok := state.createdBody["businessRuleAssetIds"].([]any)
	if !ok || len(posted) != 2 {
		t.Fatalf("posted businessRuleAssetIds = %v, want the resolved name plus the passed-through UUID", state.createdBody["businessRuleAssetIds"])
	}
	if posted[0] != "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee" {
		t.Errorf("posted[0] = %v, want the UUID resolved from the name", posted[0])
	}
	if posted[1] != "99999999-8888-7777-6666-555555555555" {
		t.Errorf("posted[1] = %v, want the UUID passed through unresolved", posted[1])
	}
}

func TestCreateRejectsUnresolvableBusinessRuleLink(t *testing.T) {
	for name, assets := range map[string][]map[string]any{
		"no match": {},
		"ambiguous": {
			{"id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "displayName": "Shared name"},
			{"id": "bbbbbbbb-cccc-dddd-eeee-ffffffffffff", "displayName": "Shared name"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			client, state := newServer(t, http.StatusCreated, createdTemplate(), assets)
			input := validInput()
			input.BusinessRuleLinks = []string{"Shared name"}
			input.Confirm = true

			out, _ := tools.NewTool(client).Handler(t.Context(), input)
			if out.Status != tools.StatusValidationError {
				t.Fatalf("status = %q, want validation_error", out.Status)
			}
			if state.createCalls != 0 {
				t.Errorf("create was called %d times despite an unresolvable link, want 0", state.createCalls)
			}
			if !strings.Contains(out.Message, "Shared name") {
				t.Errorf("message = %q, want it to name the unresolvable link", out.Message)
			}
		})
	}
}

func TestCreateDuplicateNameIsReported(t *testing.T) {
	client, _ := newServer(t, http.StatusConflict, map[string]any{"message": "already exists"}, nil)

	input := validInput()
	input.Confirm = true
	out, _ := tools.NewTool(client).Handler(t.Context(), input)
	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
	if !strings.Contains(out.Message, "already exists") {
		t.Errorf("message = %q, want it to say the name is taken", out.Message)
	}
	if !strings.Contains(out.Guidance, "update_data_quality_rule_template") {
		t.Errorf("guidance = %q, want it to point at the update tool", out.Guidance)
	}
}

func TestCreateSurfacesValidationFailureFromAPI(t *testing.T) {
	client, _ := newServer(t, http.StatusBadRequest, map[string]any{"message": "cannot translate sql"}, nil)

	input := validInput()
	input.Confirm = true
	out, _ := tools.NewTool(client).Handler(t.Context(), input)
	if out.Status != tools.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
	if !strings.Contains(out.Message, "cannot translate sql") {
		t.Errorf("message = %q, want it to carry the API's validation detail", out.Message)
	}
}
