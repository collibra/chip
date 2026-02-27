package get_business_term_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/get_business_term"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// newAssetResponse returns a standard asset response for testing.
func newAssetResponse(id, name, typeName, statusName, domainName string) clients.BusinessTermAssetResponse {
	return clients.BusinessTermAssetResponse{
		ID:          id,
		Name:        name,
		DisplayName: name,
		Type:        clients.BusinessTermNamedRef{ID: "type-001", Name: typeName},
		Status:      clients.BusinessTermNamedRef{ID: "status-001", Name: statusName},
		Domain:      clients.BusinessTermNamedRef{ID: "domain-001", Name: domainName},
	}
}

// newRelation builds a BusinessTermRelation for testing.
func newRelation(id, relType, srcID, srcName, srcType, tgtID, tgtName, tgtType string) clients.BusinessTermRelation {
	return clients.BusinessTermRelation{
		ID:   id,
		Type: clients.BusinessTermNamedRef{ID: "rt-" + id, Name: relType},
		Source: clients.BusinessTermRelationAsset{
			ID:   srcID,
			Name: srcName,
			Type: clients.BusinessTermRelationAssetType{Name: srcType},
		},
		Target: clients.BusinessTermRelationAsset{
			ID:   tgtID,
			Name: tgtName,
			Type: clients.BusinessTermRelationAssetType{Name: tgtType},
		},
	}
}

func TestGetBusinessTermSuccess(t *testing.T) {
	mux := http.NewServeMux()

	// GET /rest/2.0/attributes?assetId=... — asset attributes
	mux.Handle("GET /rest/2.0/attributes", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermAttributesResponse) {
			return http.StatusOK, clients.BusinessTermAttributesResponse{
				Total: 3,
				Results: []clients.BusinessTermAttributeResponse{
					{Type: clients.BusinessTermAttributeType{Name: "Description"}, Value: "Total revenue from customers"},
					{Type: clients.BusinessTermAttributeType{Name: "Note"}, Value: "Updated quarterly"},
					{Type: clients.BusinessTermAttributeType{Name: "Example"}, Value: "$1M"},
				},
			}
		},
	))

	// GET /rest/2.0/assets/{id} — asset details
	mux.Handle("GET /rest/2.0/assets/{id}", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermAssetResponse) {
			return http.StatusOK, newAssetResponse("bt-001", "Customer Revenue", "Business Term", "Approved", "Finance")
		},
	))

	// GET /rest/2.0/relations — lineage chain
	mux.Handle("GET /rest/2.0/relations", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermRelationsResponse) {
			sourceID := r.URL.Query().Get("sourceId")
			switch sourceID {
			case "bt-001":
				return http.StatusOK, clients.BusinessTermRelationsResponse{
					Total: 1,
					Results: []clients.BusinessTermRelation{
						newRelation("rel-001", "Business Term to Data Attribute",
							"bt-001", "Customer Revenue", "Business Term",
							"da-001", "Revenue Amount", "Data Attribute"),
					},
				}
			case "da-001":
				return http.StatusOK, clients.BusinessTermRelationsResponse{
					Total: 1,
					Results: []clients.BusinessTermRelation{
						newRelation("rel-002", "Data Attribute to Table",
							"da-001", "Revenue Amount", "Data Attribute",
							"table-001", "revenue_table", "Table"),
					},
				}
			case "table-001":
				return http.StatusOK, clients.BusinessTermRelationsResponse{
					Total: 1,
					Results: []clients.BusinessTermRelation{
						newRelation("rel-003", "Table to Column",
							"table-001", "revenue_table", "Table",
							"col-001", "amount", "Column"),
					},
				}
			default:
				return http.StatusOK, clients.BusinessTermRelationsResponse{Total: 0}
			}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: "bt-001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify asset details
	if output.Name != "Customer Revenue" {
		t.Errorf("Name: got %q, want %q", output.Name, "Customer Revenue")
	}
	if output.Description != "Total revenue from customers" {
		t.Errorf("Description: got %q, want %q", output.Description, "Total revenue from customers")
	}
	if output.Domain != "Finance" {
		t.Errorf("Domain: got %q, want %q", output.Domain, "Finance")
	}
	if output.Status != "Approved" {
		t.Errorf("Status: got %q, want %q", output.Status, "Approved")
	}
	if output.AssetType != "Business Term" {
		t.Errorf("AssetType: got %q, want %q", output.AssetType, "Business Term")
	}

	// Verify attributes
	if len(output.Attributes) != 3 {
		t.Fatalf("Attributes: got %d items, want 3", len(output.Attributes))
	}
	wantAttrs := []struct{ name, value string }{
		{"Description", "Total revenue from customers"},
		{"Note", "Updated quarterly"},
		{"Example", "$1M"},
	}
	for i, want := range wantAttrs {
		if output.Attributes[i].Name != want.name {
			t.Errorf("Attributes[%d].Name: got %q, want %q", i, output.Attributes[i].Name, want.name)
		}
		if output.Attributes[i].Value != want.value {
			t.Errorf("Attributes[%d].Value: got %q, want %q", i, output.Attributes[i].Value, want.value)
		}
	}

	// Verify lineage
	if len(output.Lineage) != 1 {
		t.Fatalf("Lineage: got %d data attributes, want 1", len(output.Lineage))
	}
	da := output.Lineage[0]
	if da.ID != "da-001" {
		t.Errorf("Lineage[0].ID: got %q, want %q", da.ID, "da-001")
	}
	if da.Name != "Revenue Amount" {
		t.Errorf("Lineage[0].Name: got %q, want %q", da.Name, "Revenue Amount")
	}
	if da.RelationType != "Business Term to Data Attribute" {
		t.Errorf("Lineage[0].RelationType: got %q, want %q", da.RelationType, "Business Term to Data Attribute")
	}

	if len(da.Tables) != 1 {
		t.Fatalf("Lineage[0].Tables: got %d, want 1", len(da.Tables))
	}
	tbl := da.Tables[0]
	if tbl.ID != "table-001" {
		t.Errorf("Table.ID: got %q, want %q", tbl.ID, "table-001")
	}
	if tbl.Name != "revenue_table" {
		t.Errorf("Table.Name: got %q, want %q", tbl.Name, "revenue_table")
	}

	if len(tbl.Columns) != 1 {
		t.Fatalf("Table.Columns: got %d, want 1", len(tbl.Columns))
	}
	col := tbl.Columns[0]
	if col.ID != "col-001" {
		t.Errorf("Column.ID: got %q, want %q", col.ID, "col-001")
	}
	if col.Name != "amount" {
		t.Errorf("Column.Name: got %q, want %q", col.Name, "amount")
	}
}

func TestGetBusinessTermNotFound(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("GET /rest/2.0/attributes", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermAttributesResponse) {
			return http.StatusNotFound, clients.BusinessTermAttributesResponse{}
		},
	))

	mux.Handle("GET /rest/2.0/assets/{id}", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermAssetResponse) {
			return http.StatusNotFound, clients.BusinessTermAssetResponse{}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: "nonexistent-id",
	})
	if err == nil {
		t.Fatal("expected error for non-existent asset, got nil")
	}
}

func TestGetBusinessTermEmptyAssetID(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: "",
	})
	if err == nil {
		t.Fatal("expected error for empty asset_id, got nil")
	}
}

func TestGetBusinessTermNoAttributes(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("GET /rest/2.0/attributes", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermAttributesResponse) {
			return http.StatusOK, clients.BusinessTermAttributesResponse{Total: 0}
		},
	))

	mux.Handle("GET /rest/2.0/assets/{id}", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermAssetResponse) {
			return http.StatusOK, newAssetResponse("bt-002", "Empty Term", "Business Term", "Draft", "Marketing")
		},
	))

	mux.Handle("GET /rest/2.0/relations", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermRelationsResponse) {
			return http.StatusOK, clients.BusinessTermRelationsResponse{Total: 0}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: "bt-002",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Name != "Empty Term" {
		t.Errorf("Name: got %q, want %q", output.Name, "Empty Term")
	}
	if output.Description != "" {
		t.Errorf("Description: got %q, want empty string", output.Description)
	}
	if len(output.Attributes) != 0 {
		t.Errorf("Attributes: got %d items, want 0", len(output.Attributes))
	}
	if len(output.Lineage) != 0 {
		t.Errorf("Lineage: got %d items, want 0", len(output.Lineage))
	}
}

func TestGetBusinessTermNoLineage(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("GET /rest/2.0/attributes", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermAttributesResponse) {
			return http.StatusOK, clients.BusinessTermAttributesResponse{
				Total: 1,
				Results: []clients.BusinessTermAttributeResponse{
					{Type: clients.BusinessTermAttributeType{Name: "Description"}, Value: "A term with no lineage"},
				},
			}
		},
	))

	mux.Handle("GET /rest/2.0/assets/{id}", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermAssetResponse) {
			return http.StatusOK, newAssetResponse("bt-003", "Isolated Term", "Business Term", "Approved", "HR")
		},
	))

	// Return relations with non-physical-data types only
	mux.Handle("GET /rest/2.0/relations", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermRelationsResponse) {
			return http.StatusOK, clients.BusinessTermRelationsResponse{
				Total: 1,
				Results: []clients.BusinessTermRelation{
					newRelation("rel-100", "Business Term to Report",
						"bt-003", "Isolated Term", "Business Term",
						"rpt-001", "Revenue Report", "Report"),
				},
			}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: "bt-003",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Name != "Isolated Term" {
		t.Errorf("Name: got %q, want %q", output.Name, "Isolated Term")
	}
	if output.Description != "A term with no lineage" {
		t.Errorf("Description: got %q, want %q", output.Description, "A term with no lineage")
	}
	if len(output.Lineage) != 0 {
		t.Errorf("Lineage: got %d items, want 0 (non-physical types should be filtered)", len(output.Lineage))
	}
}

func TestGetBusinessTermMultipleDataAttributes(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("GET /rest/2.0/attributes", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermAttributesResponse) {
			return http.StatusOK, clients.BusinessTermAttributesResponse{
				Total: 1,
				Results: []clients.BusinessTermAttributeResponse{
					{Type: clients.BusinessTermAttributeType{Name: "Description"}, Value: "Multi-lineage term"},
				},
			}
		},
	))

	mux.Handle("GET /rest/2.0/assets/{id}", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermAssetResponse) {
			return http.StatusOK, newAssetResponse("bt-004", "Multi Term", "Business Term", "Approved", "Sales")
		},
	))

	mux.Handle("GET /rest/2.0/relations", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermRelationsResponse) {
			sourceID := r.URL.Query().Get("sourceId")
			switch sourceID {
			case "bt-004":
				return http.StatusOK, clients.BusinessTermRelationsResponse{
					Total: 2,
					Results: []clients.BusinessTermRelation{
						newRelation("rel-010", "BT to DA",
							"bt-004", "Multi Term", "Business Term",
							"da-010", "Attr One", "Data Attribute"),
						newRelation("rel-011", "BT to DA",
							"bt-004", "Multi Term", "Business Term",
							"da-011", "Attr Two", "Data Attribute"),
					},
				}
			case "da-010":
				return http.StatusOK, clients.BusinessTermRelationsResponse{
					Total: 1,
					Results: []clients.BusinessTermRelation{
						newRelation("rel-020", "DA to Table",
							"da-010", "Attr One", "Data Attribute",
							"table-010", "orders", "Table"),
					},
				}
			case "da-011":
				return http.StatusOK, clients.BusinessTermRelationsResponse{Total: 0}
			case "table-010":
				return http.StatusOK, clients.BusinessTermRelationsResponse{
					Total: 2,
					Results: []clients.BusinessTermRelation{
						newRelation("rel-030", "Table to Column",
							"table-010", "orders", "Table",
							"col-010", "order_id", "Column"),
						newRelation("rel-031", "Table to Column",
							"table-010", "orders", "Table",
							"col-011", "total", "Column"),
					},
				}
			default:
				return http.StatusOK, clients.BusinessTermRelationsResponse{Total: 0}
			}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: "bt-004",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have 2 data attributes
	if len(output.Lineage) != 2 {
		t.Fatalf("Lineage: got %d data attributes, want 2", len(output.Lineage))
	}

	// First DA has 1 table with 2 columns
	da0 := output.Lineage[0]
	if da0.Name != "Attr One" {
		t.Errorf("Lineage[0].Name: got %q, want %q", da0.Name, "Attr One")
	}
	if len(da0.Tables) != 1 {
		t.Fatalf("Lineage[0].Tables: got %d, want 1", len(da0.Tables))
	}
	if len(da0.Tables[0].Columns) != 2 {
		t.Fatalf("Lineage[0].Tables[0].Columns: got %d, want 2", len(da0.Tables[0].Columns))
	}
	if da0.Tables[0].Columns[0].Name != "order_id" {
		t.Errorf("Column[0].Name: got %q, want %q", da0.Tables[0].Columns[0].Name, "order_id")
	}
	if da0.Tables[0].Columns[1].Name != "total" {
		t.Errorf("Column[1].Name: got %q, want %q", da0.Tables[0].Columns[1].Name, "total")
	}

	// Second DA has no tables
	da1 := output.Lineage[1]
	if da1.Name != "Attr Two" {
		t.Errorf("Lineage[1].Name: got %q, want %q", da1.Name, "Attr Two")
	}
	if len(da1.Tables) != 0 {
		t.Errorf("Lineage[1].Tables: got %d, want 0", len(da1.Tables))
	}
}

func TestGetBusinessTermNonStringAttributeValue(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("GET /rest/2.0/attributes", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermAttributesResponse) {
			return http.StatusOK, clients.BusinessTermAttributesResponse{
				Total: 4,
				Results: []clients.BusinessTermAttributeResponse{
					{Type: clients.BusinessTermAttributeType{Name: "Description"}, Value: "A simple description"},
					{Type: clients.BusinessTermAttributeType{Name: "Tags"}, Value: []interface{}{"finance", "revenue"}},
					{Type: clients.BusinessTermAttributeType{Name: "Count"}, Value: float64(42)},
					{Type: clients.BusinessTermAttributeType{Name: "Empty"}, Value: nil},
				},
			}
		},
	))

	mux.Handle("GET /rest/2.0/assets/{id}", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermAssetResponse) {
			return http.StatusOK, newAssetResponse("bt-005", "Typed Attrs", "Business Term", "Approved", "Tech")
		},
	))

	mux.Handle("GET /rest/2.0/relations", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermRelationsResponse) {
			return http.StatusOK, clients.BusinessTermRelationsResponse{Total: 0}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: "bt-005",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.Attributes) != 4 {
		t.Fatalf("Attributes: got %d, want 4", len(output.Attributes))
	}

	// String value
	if output.Attributes[0].Value != "A simple description" {
		t.Errorf("Attributes[0].Value: got %q, want %q", output.Attributes[0].Value, "A simple description")
	}
	// Array value should be JSON-marshaled
	if output.Attributes[1].Value != `["finance","revenue"]` {
		t.Errorf("Attributes[1].Value: got %q, want %q", output.Attributes[1].Value, `["finance","revenue"]`)
	}
	// Numeric value should be JSON-marshaled
	if output.Attributes[2].Value != "42" {
		t.Errorf("Attributes[2].Value: got %q, want %q", output.Attributes[2].Value, "42")
	}
	// Nil value should be empty string
	if output.Attributes[3].Value != "" {
		t.Errorf("Attributes[3].Value: got %q, want empty string", output.Attributes[3].Value)
	}
}

func TestGetBusinessTermServerError(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("GET /rest/2.0/attributes", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermAttributesResponse) {
			return http.StatusInternalServerError, clients.BusinessTermAttributesResponse{}
		},
	))

	mux.Handle("GET /rest/2.0/assets/{id}", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.BusinessTermAssetResponse) {
			return http.StatusInternalServerError, clients.BusinessTermAssetResponse{}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: "bt-err",
	})
	if err == nil {
		t.Fatal("expected error for server error, got nil")
	}
}
