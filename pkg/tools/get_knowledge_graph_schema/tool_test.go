package get_knowledge_graph_schema_test

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/collibra/chip/pkg/tools/get_knowledge_graph_schema"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func server(t *testing.T, descriptionsStatus int, descriptionsBody string, attributesStatus int, attributesBody string) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/knowledgeGraph/v1/hackaton/descriptions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(descriptionsStatus)
		_, _ = w.Write([]byte(descriptionsBody))
	})
	// NOTE: "hackation" (not "hackaton") is the real, live path for this endpoint
	// upstream — verified against the running service, not a typo on this side.
	mux.HandleFunc("GET /rest/knowledgeGraph/v1/hackation/attributes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(attributesStatus)
		_, _ = w.Write([]byte(attributesBody))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func TestGetKnowledgeGraphSchema_HappyPath_MergesAttributesWithDescriptions(t *testing.T) {
	attrs := `[{"name":"DataQualityScore","type":"Attribute","attributeType":"Float"},{"name":"SchemaContainsTable","type":"Relation"}]`
	descs := `{"DataQualityScore":"0-100 data quality score.","SchemaContainsTable":"Schema contains Table."}`
	c := server(t, http.StatusOK, descs, http.StatusOK, attrs)

	out, err := get_knowledge_graph_schema.NewTool(c).Handler(t.Context(), get_knowledge_graph_schema.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != get_knowledge_graph_schema.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if out.Count != 2 {
		t.Fatalf("count = %d, want 2", out.Count)
	}
	byName := map[string]get_knowledge_graph_schema.SchemaAttribute{}
	for _, a := range out.Attributes {
		byName[a.Name] = a
	}
	dqs := byName["DataQualityScore"]
	if dqs.Description != "0-100 data quality score." || dqs.Kind != "Attribute" || dqs.AttributeType != "Float" {
		t.Fatalf("unexpected merged attribute: %+v", dqs)
	}
	sct := byName["SchemaContainsTable"]
	if sct.Kind != "Relation" || sct.Description != "Schema contains Table." {
		t.Fatalf("unexpected merged relation: %+v", sct)
	}
}

func TestGetKnowledgeGraphSchema_RelationIncludesSourceAndTargetTypesWithTheirDescriptions(t *testing.T) {
	attrs := `[{"name":"contains","type":"Relation","sourceTypes":["database","schema"],"targetTypes":["schema","table"]}]`
	descs := `{"contains":"Structural containment.","database":"Physical or logical database system.","schema":"Logical namespace grouping tables.","table":"Database table, view, or external table."}`
	c := server(t, http.StatusOK, descs, http.StatusOK, attrs)

	out, err := get_knowledge_graph_schema.NewTool(c).Handler(t.Context(), get_knowledge_graph_schema.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Count != 1 {
		t.Fatalf("count = %d, want 1", out.Count)
	}
	rel := out.Attributes[0]
	if rel.Description != "Structural containment." {
		t.Fatalf("unexpected relation description: %q", rel.Description)
	}
	wantSources := []get_knowledge_graph_schema.TypeRef{
		{PublicID: "database", Description: "Physical or logical database system."},
		{PublicID: "schema", Description: "Logical namespace grouping tables."},
	}
	if !reflect.DeepEqual(rel.SourceTypes, wantSources) {
		t.Fatalf("sourceTypes = %+v, want %+v", rel.SourceTypes, wantSources)
	}
	wantTargets := []get_knowledge_graph_schema.TypeRef{
		{PublicID: "schema", Description: "Logical namespace grouping tables."},
		{PublicID: "table", Description: "Database table, view, or external table."},
	}
	if !reflect.DeepEqual(rel.TargetTypes, wantTargets) {
		t.Fatalf("targetTypes = %+v, want %+v", rel.TargetTypes, wantTargets)
	}
}

func TestGetKnowledgeGraphSchema_MissingDescriptionLeavesItEmpty(t *testing.T) {
	c := server(t, http.StatusOK, `{}`, http.StatusOK, `[{"name":"name","type":"Attribute","attributeType":"String"}]`)

	out, err := get_knowledge_graph_schema.NewTool(c).Handler(t.Context(), get_knowledge_graph_schema.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Attributes) != 1 || out.Attributes[0].Description != "" {
		t.Fatalf("unexpected attributes: %+v", out.Attributes)
	}
}

func TestGetKnowledgeGraphSchema_DownstreamErrorSurfaces(t *testing.T) {
	c := server(t, http.StatusInternalServerError, `{"message":"boom"}`, http.StatusOK, `[]`)

	out, _ := get_knowledge_graph_schema.NewTool(c).Handler(t.Context(), get_knowledge_graph_schema.Input{})
	if out.Status != get_knowledge_graph_schema.StatusError {
		t.Fatalf("status = %q, want error", out.Status)
	}
}
