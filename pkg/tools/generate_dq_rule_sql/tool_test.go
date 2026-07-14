package generate_dq_rule_sql_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/generate_dq_rule_sql"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func server(t *testing.T, code int, resp clients.Text2SQLResponse, captured *clients.Text2SQLRequest) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/dq/internal/v1/ai/text2sql", func(w http.ResponseWriter, r *http.Request) {
		if captured != nil {
			_ = json.NewDecoder(r.Body).Decode(captured)
		}
		if code != http.StatusOK {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func input() generate_dq_rule_sql.Input {
	return generate_dq_rule_sql.Input{
		EdgeSiteID: "site", ConnectionID: "conn", JobName: "PUBLIC.USERS",
		Columns: []string{"email"}, Query: "email must not be null",
	}
}

func TestGenerateDQRuleSQL_HappyPath(t *testing.T) {
	var got clients.Text2SQLRequest
	c := server(t, http.StatusOK, clients.Text2SQLResponse{SQLQuery: "SELECT * FROM @dataset WHERE email IS NULL"}, &got)
	out, err := generate_dq_rule_sql.NewTool(c).Handler(t.Context(), input())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != generate_dq_rule_sql.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if out.SQLQuery == "" {
		t.Fatalf("expected generated SQL")
	}
	if len(got.Columns) != 1 || got.Columns[0] != "email" {
		t.Fatalf("columns not forwarded: %+v", got.Columns)
	}
}

func TestGenerateDQRuleSQL_MissingInput(t *testing.T) {
	c := server(t, http.StatusOK, clients.Text2SQLResponse{}, nil)
	in := input()
	in.Query = ""
	out, _ := generate_dq_rule_sql.NewTool(c).Handler(t.Context(), in)
	if out.Status != generate_dq_rule_sql.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestGenerateDQRuleSQL_NoColumns(t *testing.T) {
	c := server(t, http.StatusOK, clients.Text2SQLResponse{}, nil)
	in := input()
	in.Columns = nil
	out, _ := generate_dq_rule_sql.NewTool(c).Handler(t.Context(), in)
	if out.Status != generate_dq_rule_sql.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}

func TestGenerateDQRuleSQL_BadRequestSurfaces(t *testing.T) {
	c := server(t, http.StatusBadRequest, clients.Text2SQLResponse{}, nil)
	out, _ := generate_dq_rule_sql.NewTool(c).Handler(t.Context(), input())
	if out.Status != generate_dq_rule_sql.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}
