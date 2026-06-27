package prepare_create_dq_job_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/prepare_create_dq_job"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	connID = "conn-1"
	siteID = "site-1"
	// Catalog asset chain UUIDs for the tableAssetId resolution test.
	sysAssetID    = "00000000-0000-0000-0000-0000000000d1"
	dbAssetID     = "00000000-0000-0000-0000-0000000000c1"
	schemaAssetID = "00000000-0000-0000-0000-0000000000b1"
	tableAssetID  = "00000000-0000-0000-0000-0000000000a1"
)

func dqMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/rest/dq/internal/v1/connections", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{
			"results": []map[string]any{{
				"connectionId":        connID,
				"connectionName":      "POSTGRES-SOURCE",
				"capabilityTypes":     []string{"PULLUP"},
				"databaseProductName": "POSTGRES",
				"edgeSiteId":          siteID,
				"edgeSiteName":        "EDGE-1",
				"systemAssetId":       sysAssetID,
			}},
		}
	}))
	// Catalog asset graph (GraphQL): Table -> Schema -> Database -> System via incoming relations.
	mux.HandleFunc("/graphql/knowledgeGraph/v1", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		s := string(body)
		w.Header().Set("Content-Type", "application/json")
		var asset string
		switch {
		case strings.Contains(s, tableAssetID):
			asset = `{"id":"` + tableAssetID + `","displayName":"customers","type":{"name":"Table"},"incomingRelations":[{"type":{"id":"r","role":"contains"},"source":{"id":"` + schemaAssetID + `","displayName":"sales","type":{"name":"Schema"}}}]}`
		case strings.Contains(s, schemaAssetID):
			asset = `{"id":"` + schemaAssetID + `","displayName":"sales","type":{"name":"Schema"},"incomingRelations":[{"type":{"id":"r","role":"has"},"source":{"id":"` + dbAssetID + `","displayName":"postgres","type":{"name":"Database"}}}]}`
		case strings.Contains(s, dbAssetID):
			asset = `{"id":"` + dbAssetID + `","displayName":"postgres","type":{"name":"Database"},"incomingRelations":[{"type":{"id":"r","role":"groups"},"source":{"id":"` + sysAssetID + `","displayName":"postgres","type":{"name":"System"}}}]}`
		}
		_, _ = w.Write([]byte(`{"data":{"assets":[` + asset + `]}}`))
	})
	mux.Handle("/rest/dq/internal/v1/monitoring/edge/connections/"+connID+"/dataSources", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{"total": 1, "results": []map[string]any{{"dataSourceName": "postgres", "supportsSchemas": true}}}
	}))
	mux.Handle("/rest/dq/internal/v1/monitoring/edge/"+siteID+"/connections/"+connID+"/schemas", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{"total": 1, "results": []map[string]any{{"name": "sales"}}}
	}))
	mux.Handle("/rest/dq/internal/v1/monitoring/edge/"+siteID+"/connections/"+connID+"/tables", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{"total": 1, "results": []map[string]any{{"name": "customers", "type": "TABLE"}}}
	}))
	mux.Handle("/rest/dq/internal/v1/monitoring/edge/"+siteID+"/connections/"+connID+"/columns", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{"results": []map[string]any{{"name": "id", "type": "int4"}, {"name": "balance", "type": "numeric"}}}
	}))
	return mux
}

func TestReadyResolvesFullPlan(t *testing.T) {
	server := httptest.NewServer(dqMux())
	defer server.Close()

	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Connection:     "postgres-source", // case-insensitive name match
		DataSourceName: "postgres",
		SchemaName:     "sales",
		TableName:      "customers",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusReady {
		t.Fatalf("expected status ready, got %q (%s)", out.Status, out.Message)
	}
	if out.Resolved == nil {
		t.Fatalf("expected resolved plan, got nil")
	}
	if out.Resolved.JobType != "PULLUP" {
		t.Errorf("expected jobType PULLUP, got %q", out.Resolved.JobType)
	}
	if out.Resolved.EdgeSiteName != "EDGE-1" || out.Resolved.EdgeConnectionName != "POSTGRES-SOURCE" {
		t.Errorf("unexpected edge fields: %+v", out.Resolved)
	}
	if out.Resolved.DatabaseProductName != "POSTGRES" {
		t.Errorf("expected databaseProductName POSTGRES, got %q", out.Resolved.DatabaseProductName)
	}
	if out.Resolved.SuggestedJobName != "sales.customers" {
		t.Errorf("expected suggestedJobName sales.customers, got %q", out.Resolved.SuggestedJobName)
	}
	if len(out.Columns) != 2 {
		t.Errorf("expected 2 columns, got %d", len(out.Columns))
	}
	// Monitors are surfaced at ready with a default-enabled indicator.
	if len(out.Monitors) != 9 {
		t.Fatalf("expected 9 available monitors, got %d", len(out.Monitors))
	}
	byKey := map[string]bool{}
	for _, m := range out.Monitors {
		byKey[m.Key] = m.DefaultEnabled
	}
	if !byKey["rowCount"] {
		t.Errorf("expected rowCount to be defaultEnabled")
	}
	if byKey["min"] {
		t.Errorf("expected min to be off by default")
	}
	if byKey["descriptiveStatistics"] {
		t.Errorf("expected descriptiveStatistics to be off by default")
	}
	// Advanced monitor settings are surfaced explicitly (with defaults) at ready.
	if len(out.AdaptiveMonitorSettings) != 2 {
		t.Fatalf("expected 2 adaptive monitor settings, got %d", len(out.AdaptiveMonitorSettings))
	}
	defByKey := map[string]int{}
	for _, s := range out.AdaptiveMonitorSettings {
		defByKey[s.Key] = s.Default
	}
	if defByKey["dataLookback"] != 10 || defByKey["learningPhase"] != 4 {
		t.Errorf("expected dataLookback=10, learningPhase=4 defaults, got %+v", out.AdaptiveMonitorSettings)
	}
	// Notifications catalog is surfaced at ready.
	if len(out.Notifications) != 7 {
		t.Fatalf("expected 7 notification options, got %d", len(out.Notifications))
	}
	notifDefault := map[string]bool{}
	for _, n := range out.Notifications {
		notifDefault[n.Key] = n.DefaultEnabled
	}
	if !notifDefault["jobFailed"] || notifDefault["jobCompleted"] {
		t.Errorf("unexpected notification defaults: %+v", out.Notifications)
	}
}

func TestTableAssetIdResolvesLocation(t *testing.T) {
	server := httptest.NewServer(dqMux())
	defer server.Close()

	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		TableAssetID: tableAssetID, // resolve everything from the catalog Table asset
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusReady {
		t.Fatalf("expected status ready, got %q (%s)", out.Status, out.Message)
	}
	r := out.Resolved
	if r == nil {
		t.Fatalf("expected resolved plan, got nil")
	}
	if r.EdgeConnectionName != "POSTGRES-SOURCE" || r.DataSourceName != "postgres" || r.SchemaName != "sales" || r.TableName != "customers" {
		t.Errorf("unexpected resolved location from table asset: %+v", r)
	}
	if r.TableAssetLink != "/asset/"+tableAssetID {
		t.Errorf("expected table asset deep link, got %q", r.TableAssetLink)
	}
}

func TestNoConnectionEnumerates(t *testing.T) {
	server := httptest.NewServer(dqMux())
	defer server.Close()

	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusIncomplete {
		t.Fatalf("expected status incomplete, got %q", out.Status)
	}
	if len(out.ConnectionOptions) != 1 || out.ConnectionOptions[0].ConnectionName != "POSTGRES-SOURCE" {
		t.Fatalf("expected one connection option POSTGRES-SOURCE, got %+v", out.ConnectionOptions)
	}
}

func TestUnknownSchemaNeedsClarification(t *testing.T) {
	server := httptest.NewServer(dqMux())
	defer server.Close()

	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		Connection:     connID, // resolve by UUID
		DataSourceName: "postgres",
		SchemaName:     "nope",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tools.StatusNeedsClarification {
		t.Fatalf("expected needs_clarification, got %q (%s)", out.Status, out.Message)
	}
	if len(out.SchemaOptions) != 1 || out.SchemaOptions[0] != "sales" {
		t.Errorf("expected schemaOptions [sales], got %+v", out.SchemaOptions)
	}
}
