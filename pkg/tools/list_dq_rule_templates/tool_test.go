package list_dq_rule_templates_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/list_dq_rule_templates"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func server(t *testing.T, code int, body any, gotQuery *string) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/dq/internal/v1/rules/templates", func(w http.ResponseWriter, r *http.Request) {
		if gotQuery != nil {
			*gotQuery = r.URL.RawQuery
		}
		if code != http.StatusOK {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func TestListDQRuleTemplates_HappyPath(t *testing.T) {
	var query string
	tol := 5
	c := server(t, http.StatusOK, map[string]any{
		"results": []clients.DQRuleTemplate{
			{ID: "t1", Name: "Not Null Check", Dimensions: []string{"Completeness"}, Tolerance: &tol, Ootb: true},
		},
		"total": 1, "offset": 0, "limit": 100,
	}, &query)

	ootb := true
	out, err := list_dq_rule_templates.NewTool(c).Handler(t.Context(), list_dq_rule_templates.Input{Ootb: &ootb})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != list_dq_rule_templates.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if len(out.Templates) != 1 || out.Templates[0].Name != "Not Null Check" || !out.Templates[0].Ootb {
		t.Fatalf("unexpected templates: %+v", out.Templates)
	}
	if !strings.Contains(query, "isOotb=true") {
		t.Fatalf("expected isOotb=true in query, got %q", query)
	}
}

func TestListDQRuleTemplates_InvalidLimit(t *testing.T) {
	c := server(t, http.StatusOK, map[string]any{}, nil)
	out, _ := list_dq_rule_templates.NewTool(c).Handler(t.Context(), list_dq_rule_templates.Input{Limit: 500})
	if out.Status != list_dq_rule_templates.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestListDQRuleTemplates_ForbiddenSurfaces(t *testing.T) {
	c := server(t, http.StatusForbidden, nil, nil)
	out, _ := list_dq_rule_templates.NewTool(c).Handler(t.Context(), list_dq_rule_templates.Input{})
	if out.Status != list_dq_rule_templates.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}
