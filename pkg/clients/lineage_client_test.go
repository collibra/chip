package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path"
	"testing"
)

// redirectClient rewrites requests to hit the test server instead of relative paths.
type redirectClient struct {
	baseURL string
	next    http.RoundTripper
}

func (c *redirectClient) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	base, _ := url.Parse(c.baseURL)
	clone.URL.Scheme = base.Scheme
	clone.URL.Host = base.Host
	clone.URL.Path = path.Join(base.Path, req.URL.Path)
	clone.URL.RawQuery = req.URL.RawQuery
	return c.next.RoundTrip(clone)
}

func newTestClient(server *httptest.Server) *http.Client {
	return &http.Client{Transport: &redirectClient{baseURL: server.URL, next: http.DefaultTransport}}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// --- LineageEntity.UnmarshalJSON ---

func TestLineageEntityUnmarshalJSON_plainStrings(t *testing.T) {
	data := []byte(`{"id":"1","name":"col","type":"column","dgcId":"550e8400-e29b-41d4-a716-446655440000","parentId":"42"}`)
	var e LineageEntity
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.DgcId != "550e8400-e29b-41d4-a716-446655440000" {
		t.Errorf("expected dgcId string, got %q", e.DgcId)
	}
	if e.ParentId != "42" {
		t.Errorf("expected parentId string, got %q", e.ParentId)
	}
}

func TestLineageEntityUnmarshalJSON_jsonNullableObjects(t *testing.T) {
	// Simulates response from server without JsonNullableModule registered.
	data := []byte(`{"id":"32","name":"SALESFACT","type":"table","sourceIds":[],"dgcId":{"undefined":true,"present":false},"parentId":{"undefined":false,"present":true}}`)
	var e LineageEntity
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.Id != "32" {
		t.Errorf("expected id 32, got %q", e.Id)
	}
	if e.DgcId != "" {
		t.Errorf("expected empty dgcId, got %q", e.DgcId)
	}
	if e.ParentId != "" {
		t.Errorf("expected empty parentId, got %q", e.ParentId)
	}
}

func TestLineageEntityUnmarshalJSON_nullFields(t *testing.T) {
	data := []byte(`{"id":"5","name":"t","type":"table","dgcId":null,"parentId":null}`)
	var e LineageEntity
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.DgcId != "" {
		t.Errorf("expected empty dgcId, got %q", e.DgcId)
	}
	if e.ParentId != "" {
		t.Errorf("expected empty parentId, got %q", e.ParentId)
	}
}

func TestLineageEntityUnmarshalJSON_missingFields(t *testing.T) {
	data := []byte(`{"id":"7","name":"t","type":"table"}`)
	var e LineageEntity
	if err := json.Unmarshal(data, &e); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e.DgcId != "" || e.ParentId != "" {
		t.Errorf("expected empty optional fields, got dgcId=%q parentId=%q", e.DgcId, e.ParentId)
	}
}

// --- GetLineageEntity ---

func TestGetLineageEntity_RoutesCorrectly(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		writeJSON(w, http.StatusOK, map[string]any{"id": "entity-1", "name": "col1", "type": "Column"})
	}))
	defer server.Close()

	_, err := GetLineageEntity(context.Background(), newTestClient(server), "entity-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "/technical_lineage_resource/rest/lineageGraphRead/v1/entities/entity-1"
	if capturedPath != expected {
		t.Errorf("expected path %q, got %q", expected, capturedPath)
	}
}

// --- GetLineageUpstream ---

func TestGetLineageUpstream_RoutesCorrectly(t *testing.T) {
	var capturedPath string
	var capturedQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.Query()
		writeJSON(w, http.StatusOK, map[string]any{"relations": []any{}, "pagination": nil})
	}))
	defer server.Close()

	_, err := GetLineageUpstream(context.Background(), newTestClient(server), "entity-1", "Column", 10, "cursor-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "/technical_lineage_resource/rest/lineageGraphRead/v1/entities/entity-1/upstream"
	if capturedPath != expected {
		t.Errorf("expected path %q, got %q", expected, capturedPath)
	}
	if capturedQuery.Get("entityType") != "Column" {
		t.Errorf("expected entityType=Column, got %q", capturedQuery.Get("entityType"))
	}
	if capturedQuery.Get("limit") != "10" {
		t.Errorf("expected limit=10, got %q", capturedQuery.Get("limit"))
	}
	if capturedQuery.Get("cursor") != "cursor-abc" {
		t.Errorf("expected cursor=cursor-abc, got %q", capturedQuery.Get("cursor"))
	}
}

// --- GetLineageDownstream ---

func TestGetLineageDownstream_RoutesCorrectly(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		writeJSON(w, http.StatusOK, map[string]any{"relations": []any{}, "pagination": nil})
	}))
	defer server.Close()

	_, err := GetLineageDownstream(context.Background(), newTestClient(server), "entity-2", "", 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "/technical_lineage_resource/rest/lineageGraphRead/v1/entities/entity-2/downstream"
	if capturedPath != expected {
		t.Errorf("expected path %q, got %q", expected, capturedPath)
	}
}

// --- SearchLineageEntities ---

func TestSearchLineageEntities_RoutesCorrectly(t *testing.T) {
	var capturedPath string
	var capturedQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.Query()
		writeJSON(w, http.StatusOK, map[string]any{"results": []any{}, "pagination": nil})
	}))
	defer server.Close()

	_, err := SearchLineageEntities(context.Background(), newTestClient(server), "orders", "Table", "dgc-id-1", 5, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "/technical_lineage_resource/rest/lineageGraphRead/v1/entities"
	if capturedPath != expected {
		t.Errorf("expected path %q, got %q", expected, capturedPath)
	}
	if capturedQuery.Get("nameContains") != "orders" {
		t.Errorf("expected nameContains=orders, got %q", capturedQuery.Get("nameContains"))
	}
	if capturedQuery.Get("type") != "Table" {
		t.Errorf("expected type=Table, got %q", capturedQuery.Get("type"))
	}
	if capturedQuery.Get("dgcId") != "dgc-id-1" {
		t.Errorf("expected dgcId=dgc-id-1, got %q", capturedQuery.Get("dgcId"))
	}
	if capturedQuery.Get("limit") != "5" {
		t.Errorf("expected limit=5, got %q", capturedQuery.Get("limit"))
	}
}

func TestSearchLineageEntities_JsonNullableObjects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate server without JsonNullableModule
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"results":[{"id":"32","name":"SALESFACT","type":"table","sourceIds":[],"dgcId":{"undefined":true,"present":false},"parentId":{"undefined":false,"present":true}}],"nextCursor":null}`))
	}))
	defer server.Close()

	out, err := SearchLineageEntities(context.Background(), newTestClient(server), "SALES", "", "", 5, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(out.Results))
	}
	e := out.Results[0]
	if e.Id != "32" {
		t.Errorf("expected id 32, got %q", e.Id)
	}
	if e.DgcId != "" {
		t.Errorf("expected empty dgcId, got %q", e.DgcId)
	}
}

// --- GetLineageTransformation ---

func TestGetLineageTransformation_RoutesCorrectly(t *testing.T) {
	var capturedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		writeJSON(w, http.StatusOK, map[string]any{"id": "transform-1", "name": "t1"})
	}))
	defer server.Close()

	_, err := GetLineageTransformation(context.Background(), newTestClient(server), "transform-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "/technical_lineage_resource/rest/lineageGraphRead/v1/transformations/transform-1"
	if capturedPath != expected {
		t.Errorf("expected path %q, got %q", expected, capturedPath)
	}
}

// --- SearchLineageTransformations ---

func TestSearchLineageTransformations_RoutesCorrectly(t *testing.T) {
	var capturedPath string
	var capturedQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.Query()
		writeJSON(w, http.StatusOK, map[string]any{"results": []any{}, "pagination": nil})
	}))
	defer server.Close()

	_, err := SearchLineageTransformations(context.Background(), newTestClient(server), "etl", 20, "next-cursor")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "/technical_lineage_resource/rest/lineageGraphRead/v1/transformations"
	if capturedPath != expected {
		t.Errorf("expected path %q, got %q", expected, capturedPath)
	}
	if capturedQuery.Get("nameContains") != "etl" {
		t.Errorf("expected nameContains=etl, got %q", capturedQuery.Get("nameContains"))
	}
	if capturedQuery.Get("limit") != "20" {
		t.Errorf("expected limit=20, got %q", capturedQuery.Get("limit"))
	}
	if capturedQuery.Get("cursor") != "next-cursor" {
		t.Errorf("expected cursor=next-cursor, got %q", capturedQuery.Get("cursor"))
	}
}
