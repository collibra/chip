package get_business_term_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/get_business_term"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// Test IDs used across tests.
const (
	businessTermID  = "bt-0001-0001-0001-000000000001"
	dataAttributeID = "da-0002-0002-0002-000000000002"
	tableID         = "tb-0003-0003-0003-000000000003"
	columnID        = "cl-0004-0004-0004-000000000004"
	nonExistentID   = "00000000-0000-0000-0000-000000000000"
)

func newAssetResponse(id, name, displayName, typeName, status, domain string) clients.GetBusinessTermAssetResponse {
	return clients.GetBusinessTermAssetResponse{
		ID:          id,
		Name:        name,
		DisplayName: displayName,
		Type:        clients.GetBusinessTermAssetType{Name: typeName},
		Status:      clients.GetBusinessTermAssetStatus{Name: status},
		Domain:      clients.GetBusinessTermAssetDomain{Name: domain},
	}
}

func newRelation(sourceID, sourceName, sourceType, targetID, targetName, targetType, relType string) clients.GetBusinessTermRelation {
	return clients.GetBusinessTermRelation{
		Source: clients.GetBusinessTermRelationAsset{
			ID:   sourceID,
			Name: sourceName,
			Type: clients.GetBusinessTermAssetType{Name: sourceType},
		},
		Target: clients.GetBusinessTermRelationAsset{
			ID:   targetID,
			Name: targetName,
			Type: clients.GetBusinessTermAssetType{Name: targetType},
		},
		Type: clients.GetBusinessTermRelationType{Name: relType},
	}
}

func TestGetBusinessTerm_FullLineage(t *testing.T) {
	mux := http.NewServeMux()

	// Asset endpoint: return the Business Term.
	mux.Handle("GET /rest/2.0/assets/{id}", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermAssetResponse) {
			id := r.PathValue("id")
			if id == businessTermID {
				return http.StatusOK, newAssetResponse(
					businessTermID, "Customer Name", "Customer Name",
					"Business Term", "Accepted", "Marketing",
				)
			}
			return http.StatusNotFound, clients.GetBusinessTermAssetResponse{}
		},
	))

	// Relations endpoint: return relations based on sourceId.
	mux.Handle("GET /rest/2.0/relations", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermRelationsResponse) {
			sourceID := r.URL.Query().Get("sourceId")
			switch sourceID {
			case businessTermID:
				return http.StatusOK, clients.GetBusinessTermRelationsResponse{
					Total: 1,
					Results: []clients.GetBusinessTermRelation{
						newRelation(businessTermID, "Customer Name", "Business Term",
							dataAttributeID, "customer_name_attr", "Data Attribute", "maps to"),
					},
				}
			case dataAttributeID:
				return http.StatusOK, clients.GetBusinessTermRelationsResponse{
					Total: 1,
					Results: []clients.GetBusinessTermRelation{
						newRelation(dataAttributeID, "customer_name_attr", "Data Attribute",
							tableID, "customers_table", "Table", "is stored in"),
					},
				}
			case tableID:
				return http.StatusOK, clients.GetBusinessTermRelationsResponse{
					Total: 1,
					Results: []clients.GetBusinessTermRelation{
						newRelation(tableID, "customers_table", "Table",
							columnID, "name_column", "Column", "contains"),
					},
				}
			default:
				return http.StatusOK, clients.GetBusinessTermRelationsResponse{Total: 0, Results: []clients.GetBusinessTermRelation{}}
			}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: businessTermID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify Business Term details.
	if output.ID != businessTermID {
		t.Errorf("ID: got %q, want %q", output.ID, businessTermID)
	}
	if output.Name != "Customer Name" {
		t.Errorf("Name: got %q, want %q", output.Name, "Customer Name")
	}
	if output.AssetType != "Business Term" {
		t.Errorf("AssetType: got %q, want %q", output.AssetType, "Business Term")
	}
	if output.Status != "Accepted" {
		t.Errorf("Status: got %q, want %q", output.Status, "Accepted")
	}
	if output.Domain != "Marketing" {
		t.Errorf("Domain: got %q, want %q", output.Domain, "Marketing")
	}

	// Verify Data Attributes lineage.
	if len(output.DataAttributes) != 1 {
		t.Fatalf("DataAttributes count: got %d, want 1", len(output.DataAttributes))
	}
	da := output.DataAttributes[0]
	if da.ID != dataAttributeID {
		t.Errorf("DataAttribute ID: got %q, want %q", da.ID, dataAttributeID)
	}
	if da.Name != "customer_name_attr" {
		t.Errorf("DataAttribute Name: got %q, want %q", da.Name, "customer_name_attr")
	}
	if da.RelationType != "maps to" {
		t.Errorf("DataAttribute RelationType: got %q, want %q", da.RelationType, "maps to")
	}

	// Verify Tables lineage.
	if len(da.Tables) != 1 {
		t.Fatalf("Tables count: got %d, want 1", len(da.Tables))
	}
	table := da.Tables[0]
	if table.ID != tableID {
		t.Errorf("Table ID: got %q, want %q", table.ID, tableID)
	}
	if table.Name != "customers_table" {
		t.Errorf("Table Name: got %q, want %q", table.Name, "customers_table")
	}
	if table.RelationType != "is stored in" {
		t.Errorf("Table RelationType: got %q, want %q", table.RelationType, "is stored in")
	}

	// Verify Columns lineage.
	if len(table.Columns) != 1 {
		t.Fatalf("Columns count: got %d, want 1", len(table.Columns))
	}
	col := table.Columns[0]
	if col.ID != columnID {
		t.Errorf("Column ID: got %q, want %q", col.ID, columnID)
	}
	if col.Name != "name_column" {
		t.Errorf("Column Name: got %q, want %q", col.Name, "name_column")
	}
	if col.RelationType != "contains" {
		t.Errorf("Column RelationType: got %q, want %q", col.RelationType, "contains")
	}
}

func TestGetBusinessTerm_NotFound(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("GET /rest/2.0/assets/{id}", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermAssetResponse) {
			return http.StatusNotFound, clients.GetBusinessTermAssetResponse{}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: nonExistentID,
	})
	if err == nil {
		t.Fatal("expected error for non-existent asset, got nil")
	}
	if err.Error() != "Business Term not found" {
		t.Errorf("error message: got %q, want %q", err.Error(), "Business Term not found")
	}
}

func TestGetBusinessTerm_EmptyAssetID(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: "",
	})
	if err == nil {
		t.Fatal("expected error for empty asset_id, got nil")
	}
	if err.Error() != "asset_id is required" {
		t.Errorf("error message: got %q, want %q", err.Error(), "asset_id is required")
	}
}

func TestGetBusinessTerm_NoRelations(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("GET /rest/2.0/assets/{id}", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermAssetResponse) {
			return http.StatusOK, newAssetResponse(
				businessTermID, "Orphan Term", "Orphan Term",
				"Business Term", "Draft", "Finance",
			)
		},
	))

	mux.Handle("GET /rest/2.0/relations", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermRelationsResponse) {
			return http.StatusOK, clients.GetBusinessTermRelationsResponse{
				Total:   0,
				Results: []clients.GetBusinessTermRelation{},
			}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: businessTermID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Name != "Orphan Term" {
		t.Errorf("Name: got %q, want %q", output.Name, "Orphan Term")
	}
	if output.Domain != "Finance" {
		t.Errorf("Domain: got %q, want %q", output.Domain, "Finance")
	}
	if len(output.DataAttributes) != 0 {
		t.Errorf("DataAttributes count: got %d, want 0", len(output.DataAttributes))
	}
}

func TestGetBusinessTerm_RelationsWithNoPhysicalData(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("GET /rest/2.0/assets/{id}", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermAssetResponse) {
			return http.StatusOK, newAssetResponse(
				businessTermID, "Revenue", "Revenue",
				"Business Term", "Accepted", "Sales",
			)
		},
	))

	// Relations exist but none are Data Attribute targets.
	mux.Handle("GET /rest/2.0/relations", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermRelationsResponse) {
			return http.StatusOK, clients.GetBusinessTermRelationsResponse{
				Total: 2,
				Results: []clients.GetBusinessTermRelation{
					newRelation(businessTermID, "Revenue", "Business Term",
						"other-0001", "Related Policy", "Policy", "governed by"),
					newRelation(businessTermID, "Revenue", "Business Term",
						"other-0002", "KPI Dashboard", "Report", "reported in"),
				},
			}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: businessTermID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Name != "Revenue" {
		t.Errorf("Name: got %q, want %q", output.Name, "Revenue")
	}
	if len(output.DataAttributes) != 0 {
		t.Errorf("DataAttributes count: got %d, want 0 (non-physical relations should be filtered)", len(output.DataAttributes))
	}
}

func TestGetBusinessTerm_MultipleDataAttributes(t *testing.T) {
	da2ID := "da-0005-0005-0005-000000000005"
	table2ID := "tb-0006-0006-0006-000000000006"

	mux := http.NewServeMux()

	mux.Handle("GET /rest/2.0/assets/{id}", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermAssetResponse) {
			return http.StatusOK, newAssetResponse(
				businessTermID, "Email Address", "Email Address",
				"Business Term", "Accepted", "IT",
			)
		},
	))

	mux.Handle("GET /rest/2.0/relations", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermRelationsResponse) {
			sourceID := r.URL.Query().Get("sourceId")
			switch sourceID {
			case businessTermID:
				return http.StatusOK, clients.GetBusinessTermRelationsResponse{
					Total: 2,
					Results: []clients.GetBusinessTermRelation{
						newRelation(businessTermID, "Email Address", "Business Term",
							dataAttributeID, "email_attr_1", "Data Attribute", "maps to"),
						newRelation(businessTermID, "Email Address", "Business Term",
							da2ID, "email_attr_2", "Data Attribute", "maps to"),
					},
				}
			case dataAttributeID:
				return http.StatusOK, clients.GetBusinessTermRelationsResponse{
					Total: 1,
					Results: []clients.GetBusinessTermRelation{
						newRelation(dataAttributeID, "email_attr_1", "Data Attribute",
							tableID, "users_table", "Table", "is stored in"),
					},
				}
			case da2ID:
				return http.StatusOK, clients.GetBusinessTermRelationsResponse{
					Total: 1,
					Results: []clients.GetBusinessTermRelation{
						newRelation(da2ID, "email_attr_2", "Data Attribute",
							table2ID, "contacts_table", "Table", "is stored in"),
					},
				}
			case tableID:
				return http.StatusOK, clients.GetBusinessTermRelationsResponse{
					Total: 1,
					Results: []clients.GetBusinessTermRelation{
						newRelation(tableID, "users_table", "Table",
							columnID, "email_col", "Column", "contains"),
					},
				}
			case table2ID:
				return http.StatusOK, clients.GetBusinessTermRelationsResponse{
					Total:   0,
					Results: []clients.GetBusinessTermRelation{},
				}
			default:
				return http.StatusOK, clients.GetBusinessTermRelationsResponse{Total: 0, Results: []clients.GetBusinessTermRelation{}}
			}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: businessTermID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.DataAttributes) != 2 {
		t.Fatalf("DataAttributes count: got %d, want 2", len(output.DataAttributes))
	}

	// First Data Attribute has a Table with a Column.
	da1 := output.DataAttributes[0]
	if da1.Name != "email_attr_1" {
		t.Errorf("DA1 Name: got %q, want %q", da1.Name, "email_attr_1")
	}
	if len(da1.Tables) != 1 {
		t.Fatalf("DA1 Tables count: got %d, want 1", len(da1.Tables))
	}
	if len(da1.Tables[0].Columns) != 1 {
		t.Fatalf("DA1 Table Columns count: got %d, want 1", len(da1.Tables[0].Columns))
	}
	if da1.Tables[0].Columns[0].Name != "email_col" {
		t.Errorf("DA1 Column Name: got %q, want %q", da1.Tables[0].Columns[0].Name, "email_col")
	}

	// Second Data Attribute has a Table but no Columns.
	da2 := output.DataAttributes[1]
	if da2.Name != "email_attr_2" {
		t.Errorf("DA2 Name: got %q, want %q", da2.Name, "email_attr_2")
	}
	if len(da2.Tables) != 1 {
		t.Fatalf("DA2 Tables count: got %d, want 1", len(da2.Tables))
	}
	if len(da2.Tables[0].Columns) != 0 {
		t.Errorf("DA2 Table Columns count: got %d, want 0", len(da2.Tables[0].Columns))
	}
}

func TestGetBusinessTerm_DataAttributeWithNoTables(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("GET /rest/2.0/assets/{id}", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermAssetResponse) {
			return http.StatusOK, newAssetResponse(
				businessTermID, "Phone Number", "Phone Number",
				"Business Term", "Draft", "Support",
			)
		},
	))

	mux.Handle("GET /rest/2.0/relations", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermRelationsResponse) {
			sourceID := r.URL.Query().Get("sourceId")
			switch sourceID {
			case businessTermID:
				return http.StatusOK, clients.GetBusinessTermRelationsResponse{
					Total: 1,
					Results: []clients.GetBusinessTermRelation{
						newRelation(businessTermID, "Phone Number", "Business Term",
							dataAttributeID, "phone_attr", "Data Attribute", "maps to"),
					},
				}
			case dataAttributeID:
				// Data Attribute has no Table relations.
				return http.StatusOK, clients.GetBusinessTermRelationsResponse{
					Total:   0,
					Results: []clients.GetBusinessTermRelation{},
				}
			default:
				return http.StatusOK, clients.GetBusinessTermRelationsResponse{Total: 0, Results: []clients.GetBusinessTermRelation{}}
			}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: businessTermID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(output.DataAttributes) != 1 {
		t.Fatalf("DataAttributes count: got %d, want 1", len(output.DataAttributes))
	}
	if output.DataAttributes[0].Name != "phone_attr" {
		t.Errorf("DataAttribute Name: got %q, want %q", output.DataAttributes[0].Name, "phone_attr")
	}
	if len(output.DataAttributes[0].Tables) != 0 {
		t.Errorf("Tables count: got %d, want 0", len(output.DataAttributes[0].Tables))
	}
}

func TestGetBusinessTerm_ServerError(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("GET /rest/2.0/assets/{id}", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermAssetResponse) {
			return http.StatusInternalServerError, clients.GetBusinessTermAssetResponse{}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: businessTermID,
	})
	if err == nil {
		t.Fatal("expected error for server error, got nil")
	}
}

func TestGetBusinessTerm_DisplayName(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("GET /rest/2.0/assets/{id}", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermAssetResponse) {
			return http.StatusOK, newAssetResponse(
				businessTermID, "cust_addr", "Customer Address",
				"Business Term", "Approved", "Operations",
			)
		},
	))

	mux.Handle("GET /rest/2.0/relations", testutil.JsonHandlerOut(
		func(r *http.Request) (int, clients.GetBusinessTermRelationsResponse) {
			return http.StatusOK, clients.GetBusinessTermRelationsResponse{
				Total:   0,
				Results: []clients.GetBusinessTermRelation{},
			}
		},
	))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := get_business_term.NewTool(client).Handler(t.Context(), get_business_term.Input{
		AssetID: businessTermID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.DisplayName != "Customer Address" {
		t.Errorf("DisplayName: got %q, want %q", output.DisplayName, "Customer Address")
	}
	if output.Name != "cust_addr" {
		t.Errorf("Name: got %q, want %q", output.Name, "cust_addr")
	}
}
