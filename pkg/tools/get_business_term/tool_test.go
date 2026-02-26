package get_business_term_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/get_business_term"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// newAssetResponse is a helper to create a GetBusinessTermAssetResponse.
func newAssetResponse(id, name, displayName, typeName, statusName, domainName, domainID string) clients.GetBusinessTermAssetResponse {
	return clients.GetBusinessTermAssetResponse{
		ID:          id,
		Name:        name,
		DisplayName: displayName,
		Type:        clients.GetBusinessTermNamedRef{Name: typeName, ID: "type-001"},
		Status:      clients.GetBusinessTermNamedRef{Name: statusName, ID: "status-001"},
		Domain:      clients.GetBusinessTermNamedRef{Name: domainName, ID: domainID},
		ResourceType: "Asset",
	}
}

// newRelation is a helper to create a GetBusinessTermRelation.
func newRelation(id, sourceID, sourceName, sourceTypeName, targetID, targetName, targetTypeName, relTypeName string) clients.GetBusinessTermRelation {
	return clients.GetBusinessTermRelation{
		ID: id,
		Source: clients.GetBusinessTermRelationAsset{
			ID:   sourceID,
			Name: sourceName,
			Type: clients.GetBusinessTermNamedRef{Name: sourceTypeName, ID: "stype-001"},
		},
		Target: clients.GetBusinessTermRelationAsset{
			ID:   targetID,
			Name: targetName,
			Type: clients.GetBusinessTermNamedRef{Name: targetTypeName, ID: "ttype-001"},
		},
		Type: clients.GetBusinessTermNamedRef{Name: relTypeName, ID: "rtype-001"},
	}
}

// relationsResponse is a helper to wrap relations into a response.
func relationsResponse(rels ...clients.GetBusinessTermRelation) clients.GetBusinessTermRelationsResponse {
	return clients.GetBusinessTermRelationsResponse{
		Results: rels,
		Total:   len(rels),
		Limit:   1000,
		Offset:  0,
	}
}

func TestGetBusinessTerm_FullLineage(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("/rest/2.0/assets/bt-001", testutil.JsonHandlerOut(
		func(_ *http.Request) (int, clients.GetBusinessTermAssetResponse) {
			return http.StatusOK, newAssetResponse(
				"bt-001", "Customer Name", "Customer Name",
				"Business Term", "Approved", "Customer Domain", "domain-001",
			)
		},
	))

	mux.Handle("/rest/2.0/relations", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermRelationsResponse) {
			sourceID := r.URL.Query().Get("sourceId")
			switch sourceID {
			case "bt-001":
				return http.StatusOK, relationsResponse(
					newRelation("rel-001", "bt-001", "Customer Name", "Business Term",
						"da-001", "customer_name_attr", "Data Attribute",
						"Business Term maps to Data Attribute"),
				)
			case "da-001":
				return http.StatusOK, relationsResponse(
					newRelation("rel-002", "da-001", "customer_name_attr", "Data Attribute",
						"tbl-001", "customers", "Table",
						"Data Attribute is part of Table"),
				)
			case "tbl-001":
				return http.StatusOK, relationsResponse(
					newRelation("rel-003", "tbl-001", "customers", "Table",
						"col-001", "name", "Column",
						"Table contains Column"),
				)
			default:
				return http.StatusOK, relationsResponse()
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

	// Verify asset details.
	if output.ID != "bt-001" {
		t.Errorf("ID: got %q, want %q", output.ID, "bt-001")
	}
	if output.Name != "Customer Name" {
		t.Errorf("Name: got %q, want %q", output.Name, "Customer Name")
	}
	if output.DisplayName != "Customer Name" {
		t.Errorf("DisplayName: got %q, want %q", output.DisplayName, "Customer Name")
	}
	if output.AssetType != "Business Term" {
		t.Errorf("AssetType: got %q, want %q", output.AssetType, "Business Term")
	}
	if output.Status != "Approved" {
		t.Errorf("Status: got %q, want %q", output.Status, "Approved")
	}
	if output.DomainName != "Customer Domain" {
		t.Errorf("DomainName: got %q, want %q", output.DomainName, "Customer Domain")
	}
	if output.DomainID != "domain-001" {
		t.Errorf("DomainID: got %q, want %q", output.DomainID, "domain-001")
	}

	// Verify lineage: 1 Data Attribute → 1 Table → 1 Column.
	if len(output.Lineage) != 1 {
		t.Fatalf("Lineage length: got %d, want 1", len(output.Lineage))
	}

	da := output.Lineage[0]
	if da.ID != "da-001" {
		t.Errorf("DA ID: got %q, want %q", da.ID, "da-001")
	}
	if da.Name != "customer_name_attr" {
		t.Errorf("DA Name: got %q, want %q", da.Name, "customer_name_attr")
	}
	if da.RelationType != "Business Term maps to Data Attribute" {
		t.Errorf("DA RelationType: got %q, want %q", da.RelationType, "Business Term maps to Data Attribute")
	}

	if len(da.Tables) != 1 {
		t.Fatalf("Tables length: got %d, want 1", len(da.Tables))
	}

	tbl := da.Tables[0]
	if tbl.ID != "tbl-001" {
		t.Errorf("Table ID: got %q, want %q", tbl.ID, "tbl-001")
	}
	if tbl.Name != "customers" {
		t.Errorf("Table Name: got %q, want %q", tbl.Name, "customers")
	}
	if tbl.RelationType != "Data Attribute is part of Table" {
		t.Errorf("Table RelationType: got %q, want %q", tbl.RelationType, "Data Attribute is part of Table")
	}

	if len(tbl.Columns) != 1 {
		t.Fatalf("Columns length: got %d, want 1", len(tbl.Columns))
	}

	col := tbl.Columns[0]
	if col.ID != "col-001" {
		t.Errorf("Column ID: got %q, want %q", col.ID, "col-001")
	}
	if col.Name != "name" {
		t.Errorf("Column Name: got %q, want %q", col.Name, "name")
	}
	if col.RelationType != "Table contains Column" {
		t.Errorf("Column RelationType: got %q, want %q", col.RelationType, "Table contains Column")
	}
}

func TestGetBusinessTerm_NoRelations(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("/rest/2.0/assets/bt-empty", testutil.JsonHandlerOut(
		func(_ *http.Request) (int, clients.GetBusinessTermAssetResponse) {
			return http.StatusOK, newAssetResponse(
				"bt-empty", "Isolated Term", "Isolated Term",
				"Business Term", "Draft", "Glossary", "domain-002",
			)
		},
	))

	mux.Handle("/rest/2.0/relations", testutil.JsonHandlerOut(
		func(_ *http.Request) (int, clients.GetBusinessTermRelationsResponse) {
			return http.StatusOK, relationsResponse()
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: "bt-empty",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Name != "Isolated Term" {
		t.Errorf("Name: got %q, want %q", output.Name, "Isolated Term")
	}
	if len(output.Lineage) != 0 {
		t.Errorf("Lineage length: got %d, want 0", len(output.Lineage))
	}
}

func TestGetBusinessTerm_NonPhysicalRelationsFiltered(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("/rest/2.0/assets/bt-mixed", testutil.JsonHandlerOut(
		func(_ *http.Request) (int, clients.GetBusinessTermAssetResponse) {
			return http.StatusOK, newAssetResponse(
				"bt-mixed", "Mixed Relations", "Mixed Relations",
				"Business Term", "Approved", "Domain A", "domain-003",
			)
		},
	))

	mux.Handle("/rest/2.0/relations", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermRelationsResponse) {
			sourceID := r.URL.Query().Get("sourceId")
			if sourceID == "bt-mixed" {
				return http.StatusOK, relationsResponse(
					// Non-physical: should be filtered out.
					newRelation("rel-x1", "bt-mixed", "Mixed Relations", "Business Term",
						"other-001", "Some Policy", "Policy",
						"Business Term relates to Policy"),
					// Non-physical: should be filtered out.
					newRelation("rel-x2", "bt-mixed", "Mixed Relations", "Business Term",
						"other-002", "Some Report", "Report",
						"Business Term relates to Report"),
				)
			}
			return http.StatusOK, relationsResponse()
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: "bt-mixed",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.Lineage) != 0 {
		t.Errorf("Lineage length: got %d, want 0 (non-physical relations should be filtered)", len(output.Lineage))
	}
}

func TestGetBusinessTerm_AssetNotFound(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("/rest/2.0/assets/nonexistent-id", testutil.JsonHandlerOut(
		func(_ *http.Request) (int, struct{}) {
			return http.StatusNotFound, struct{}{}
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

func TestGetBusinessTerm_RelationsServerError(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("/rest/2.0/assets/bt-err", testutil.JsonHandlerOut(
		func(_ *http.Request) (int, clients.GetBusinessTermAssetResponse) {
			return http.StatusOK, newAssetResponse(
				"bt-err", "Error Term", "Error Term",
				"Business Term", "Approved", "Domain", "domain-004",
			)
		},
	))

	mux.Handle("/rest/2.0/relations", testutil.JsonHandlerOut(
		func(_ *http.Request) (int, struct{}) {
			return http.StatusInternalServerError, struct{}{}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: "bt-err",
	})
	if err == nil {
		t.Fatal("expected error when relations endpoint fails, got nil")
	}
}

func TestGetBusinessTerm_MultipleDataAttributes(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("/rest/2.0/assets/bt-multi", testutil.JsonHandlerOut(
		func(_ *http.Request) (int, clients.GetBusinessTermAssetResponse) {
			return http.StatusOK, newAssetResponse(
				"bt-multi", "Customer Info", "Customer Info",
				"Business Term", "Approved", "Customer Domain", "domain-005",
			)
		},
	))

	mux.Handle("/rest/2.0/relations", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermRelationsResponse) {
			sourceID := r.URL.Query().Get("sourceId")
			switch sourceID {
			case "bt-multi":
				return http.StatusOK, relationsResponse(
					newRelation("rel-a", "bt-multi", "Customer Info", "Business Term",
						"da-a", "first_name_attr", "Data Attribute",
						"maps to"),
					newRelation("rel-b", "bt-multi", "Customer Info", "Business Term",
						"da-b", "last_name_attr", "Data Attribute",
						"maps to"),
				)
			case "da-a":
				return http.StatusOK, relationsResponse(
					newRelation("rel-c", "da-a", "first_name_attr", "Data Attribute",
						"tbl-a", "users", "Table",
						"is part of"),
				)
			case "da-b":
				return http.StatusOK, relationsResponse(
					newRelation("rel-d", "da-b", "last_name_attr", "Data Attribute",
						"tbl-b", "contacts", "Table",
						"is part of"),
				)
			case "tbl-a":
				return http.StatusOK, relationsResponse(
					newRelation("rel-e", "tbl-a", "users", "Table",
						"col-a1", "first_name", "Column",
						"contains"),
					newRelation("rel-f", "tbl-a", "users", "Table",
						"col-a2", "last_name", "Column",
						"contains"),
				)
			case "tbl-b":
				return http.StatusOK, relationsResponse(
					newRelation("rel-g", "tbl-b", "contacts", "Table",
						"col-b1", "surname", "Column",
						"contains"),
				)
			default:
				return http.StatusOK, relationsResponse()
			}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: "bt-multi",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 2 Data Attributes.
	if len(output.Lineage) != 2 {
		t.Fatalf("Lineage length: got %d, want 2", len(output.Lineage))
	}

	// First DA: 1 Table with 2 Columns.
	da0 := output.Lineage[0]
	if da0.Name != "first_name_attr" {
		t.Errorf("DA[0] Name: got %q, want %q", da0.Name, "first_name_attr")
	}
	if len(da0.Tables) != 1 {
		t.Fatalf("DA[0] Tables length: got %d, want 1", len(da0.Tables))
	}
	if len(da0.Tables[0].Columns) != 2 {
		t.Fatalf("DA[0] Table[0] Columns length: got %d, want 2", len(da0.Tables[0].Columns))
	}
	if da0.Tables[0].Columns[0].Name != "first_name" {
		t.Errorf("DA[0] Table[0] Col[0] Name: got %q, want %q", da0.Tables[0].Columns[0].Name, "first_name")
	}
	if da0.Tables[0].Columns[1].Name != "last_name" {
		t.Errorf("DA[0] Table[0] Col[1] Name: got %q, want %q", da0.Tables[0].Columns[1].Name, "last_name")
	}

	// Second DA: 1 Table with 1 Column.
	da1 := output.Lineage[1]
	if da1.Name != "last_name_attr" {
		t.Errorf("DA[1] Name: got %q, want %q", da1.Name, "last_name_attr")
	}
	if len(da1.Tables) != 1 {
		t.Fatalf("DA[1] Tables length: got %d, want 1", len(da1.Tables))
	}
	if len(da1.Tables[0].Columns) != 1 {
		t.Fatalf("DA[1] Table[0] Columns length: got %d, want 1", len(da1.Tables[0].Columns))
	}
	if da1.Tables[0].Columns[0].Name != "surname" {
		t.Errorf("DA[1] Table[0] Col[0] Name: got %q, want %q", da1.Tables[0].Columns[0].Name, "surname")
	}
}

func TestGetBusinessTerm_DataAttributeWithNoTables(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("/rest/2.0/assets/bt-notables", testutil.JsonHandlerOut(
		func(_ *http.Request) (int, clients.GetBusinessTermAssetResponse) {
			return http.StatusOK, newAssetResponse(
				"bt-notables", "Orphan Term", "Orphan Term",
				"Business Term", "Candidate", "Glossary", "domain-006",
			)
		},
	))

	mux.Handle("/rest/2.0/relations", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermRelationsResponse) {
			sourceID := r.URL.Query().Get("sourceId")
			switch sourceID {
			case "bt-notables":
				return http.StatusOK, relationsResponse(
					newRelation("rel-z", "bt-notables", "Orphan Term", "Business Term",
						"da-orphan", "orphan_attr", "Data Attribute",
						"maps to"),
				)
			case "da-orphan":
				// Data Attribute has no Table relations.
				return http.StatusOK, relationsResponse()
			default:
				return http.StatusOK, relationsResponse()
			}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: "bt-notables",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.Lineage) != 1 {
		t.Fatalf("Lineage length: got %d, want 1", len(output.Lineage))
	}
	if len(output.Lineage[0].Tables) != 0 {
		t.Errorf("Tables length: got %d, want 0", len(output.Lineage[0].Tables))
	}
}

func TestGetBusinessTerm_TableWithNoColumns(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("/rest/2.0/assets/bt-nocols", testutil.JsonHandlerOut(
		func(_ *http.Request) (int, clients.GetBusinessTermAssetResponse) {
			return http.StatusOK, newAssetResponse(
				"bt-nocols", "No Columns Term", "No Columns Term",
				"Business Term", "Approved", "Domain", "domain-007",
			)
		},
	))

	mux.Handle("/rest/2.0/relations", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermRelationsResponse) {
			sourceID := r.URL.Query().Get("sourceId")
			switch sourceID {
			case "bt-nocols":
				return http.StatusOK, relationsResponse(
					newRelation("rel-nc1", "bt-nocols", "No Columns Term", "Business Term",
						"da-nc", "some_attr", "Data Attribute",
						"maps to"),
				)
			case "da-nc":
				return http.StatusOK, relationsResponse(
					newRelation("rel-nc2", "da-nc", "some_attr", "Data Attribute",
						"tbl-nc", "empty_table", "Table",
						"is part of"),
				)
			case "tbl-nc":
				// Table has no Column relations.
				return http.StatusOK, relationsResponse()
			default:
				return http.StatusOK, relationsResponse()
			}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: "bt-nocols",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.Lineage) != 1 {
		t.Fatalf("Lineage length: got %d, want 1", len(output.Lineage))
	}
	if len(output.Lineage[0].Tables) != 1 {
		t.Fatalf("Tables length: got %d, want 1", len(output.Lineage[0].Tables))
	}
	if len(output.Lineage[0].Tables[0].Columns) != 0 {
		t.Errorf("Columns length: got %d, want 0", len(output.Lineage[0].Tables[0].Columns))
	}
}
