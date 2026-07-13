package preview_dq_rule_sql_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/preview_dq_rule_sql"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func server(t *testing.T, code int, data clients.DQPreviewData) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/dq/internal/v1/rules/previewRule", func(w http.ResponseWriter, r *http.Request) {
		if code != http.StatusOK {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(data)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func input() preview_dq_rule_sql.Input {
	return preview_dq_rule_sql.Input{
		EdgeSiteID: "site", ConnectionID: "conn", SchemaName: "public", JobName: "DS", PreviewRule: "SELECT 1",
	}
}

func TestPreviewDQRuleSQL_HappyPath(t *testing.T) {
	c := server(t, http.StatusOK, clients.DQPreviewData{
		Columns: []clients.DQPreviewColumn{{ColumnName: "id", ColumnType: "INT", Position: 0}},
		Data:    []clients.DQPreviewRow{{Values: []string{"1"}}, {Values: []string{"2"}}},
	})
	out, err := preview_dq_rule_sql.NewTool(c).Handler(t.Context(), input())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != preview_dq_rule_sql.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if len(out.Columns) != 1 || out.Columns[0].Name != "id" || len(out.Rows) != 2 {
		t.Fatalf("unexpected preview: cols=%+v rows=%+v", out.Columns, out.Rows)
	}
}

func TestPreviewDQRuleSQL_BadSQLSurfaces(t *testing.T) {
	c := server(t, http.StatusBadRequest, clients.DQPreviewData{})
	out, _ := preview_dq_rule_sql.NewTool(c).Handler(t.Context(), input())
	if out.Status != preview_dq_rule_sql.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}

func TestPreviewDQRuleSQL_MissingInput(t *testing.T) {
	c := server(t, http.StatusOK, clients.DQPreviewData{})
	in := input()
	in.ConnectionID = ""
	out, _ := preview_dq_rule_sql.NewTool(c).Handler(t.Context(), in)
	if out.Status != preview_dq_rule_sql.StatusValidationError {
		t.Fatalf("status = %q, want validation_error", out.Status)
	}
}
