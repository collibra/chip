package list_asset_types_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/list_asset_types"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestListAssetTypes(t *testing.T) {
	assetTypeId, _ := uuid.NewUUID()
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(httpRequest *http.Request) (int, clients.AssetTypePagedResponse) {
		return http.StatusOK, clients.AssetTypePagedResponse{
			Total:  1,
			Offset: 0,
			Limit:  100,
			Results: []clients.AssetTypeDetails{
				{
					ID:                 assetTypeId.String(),
					Name:               "Data Element",
					Description:        "A data element asset type",
					PublicId:           "DataElement",
					DisplayNameEnabled: true,
					RatingEnabled:      false,
					FinalType:          false,
					System:             false,
					Product:            "Data Governance Center",
				},
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Limit: 100,
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != tools.StatusSuccess {
		t.Fatalf("Expected status success, got: %s", output.Status)
	}

	if output.Total != 1 {
		t.Fatalf("Expected 1 result, got: %d", output.Total)
	}

	if len(output.AssetTypes) != 1 {
		t.Fatalf("Expected 1 asset type, got: %d", len(output.AssetTypes))
	}

	if !output.ScanExhaustive {
		t.Fatalf("Expected an unfiltered/server-side listing to be exhaustive")
	}

	assetType := output.AssetTypes[0]
	if assetType.Name != "Data Element" {
		t.Fatalf("Expected name 'Data Element', got: '%s'", assetType.Name)
	}

	if assetType.ID != assetTypeId.String() {
		t.Fatalf("Expected ID '%s', got: '%s'", assetTypeId.String(), assetType.ID)
	}

	if assetType.PublicId != "DataElement" {
		t.Fatalf("Expected publicId 'DataElement', got: '%s'", assetType.PublicId)
	}
}

// TestListAssetTypes_NameForwardedServerSide asserts AC-1: the `name` filter
// is forwarded to Collibra as a server-side query param, not filtered
// client-side.
func TestListAssetTypes_NameForwardedServerSide(t *testing.T) {
	var gotName string
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(httpRequest *http.Request) (int, clients.AssetTypePagedResponse) {
		gotName = httpRequest.URL.Query().Get("name")
		return http.StatusOK, clients.AssetTypePagedResponse{
			Total:  1,
			Offset: 0,
			Limit:  100,
			Results: []clients.AssetTypeDetails{
				{ID: "1", Name: "Issue", PublicId: "Issue", Product: "Data Helpdesk"},
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: "Issue"})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if gotName != "Issue" {
		t.Fatalf("Expected server-side name query param 'Issue', got: %q", gotName)
	}
	if output.Status != tools.StatusSuccess {
		t.Fatalf("Expected status success, got: %s", output.Status)
	}
	if !output.ScanExhaustive {
		t.Fatalf("Expected a name-only filter to be reported as exhaustive")
	}
	if len(output.AssetTypes) != 1 || output.AssetTypes[0].Name != "Issue" {
		t.Fatalf("Expected the single 'Issue' result to be returned, got: %+v", output.AssetTypes)
	}
}

// TestListAssetTypes_ProductFilter asserts AC-2: a product filter matches
// each record individually (root and subtypes each carry their own product
// tag), applied over a single page that fully covers the catalog, and is
// reported exhaustive per AC-3.
func TestListAssetTypes_ProductFilter(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(httpRequest *http.Request) (int, clients.AssetTypePagedResponse) {
		return http.StatusOK, clients.AssetTypePagedResponse{
			Total:  3,
			Offset: 0,
			Limit:  1000,
			Results: []clients.AssetTypeDetails{
				{ID: "1", Name: "Issue", PublicId: "Issue", Product: "Data Helpdesk"},
				{ID: "2", Name: "Data Issue", PublicId: "DataIssue", Product: "Data Helpdesk"},
				{ID: "3", Name: "Business Term", PublicId: "BusinessTerm", Product: "Data Governance Center"},
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Product: "Data Helpdesk"})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != tools.StatusSuccess {
		t.Fatalf("Expected status success, got: %s", output.Status)
	}
	if !output.ScanExhaustive {
		t.Fatalf("Expected the scan to be exhaustive since the whole (3-item) catalog fits in one page")
	}
	if output.Scanned != 3 {
		t.Fatalf("Expected 3 asset types scanned, got: %d", output.Scanned)
	}
	if output.Total != 2 || len(output.AssetTypes) != 2 {
		t.Fatalf("Expected 2 matching 'Data Helpdesk' asset types, got total=%d assetTypes=%+v", output.Total, output.AssetTypes)
	}
	for _, at := range output.AssetTypes {
		if at.Product != "Data Helpdesk" {
			t.Fatalf("Expected only 'Data Helpdesk' asset types, got: %s", at.Product)
		}
	}
}

// TestListAssetTypes_PublicIdFilter_MultiPage asserts AC-2 for publicId and
// that the client-side scan pages through the whole catalog (not just the
// first page) before concluding no more matches exist, reporting exhaustive.
func TestListAssetTypes_PublicIdFilter_MultiPage(t *testing.T) {
	const total = 3
	pages := [][]clients.AssetTypeDetails{
		{{ID: "1", Name: "Decoy", PublicId: "Decoy"}},
		{{ID: "2", Name: "Issue", PublicId: "Issue", Product: "Data Helpdesk"}},
		{{ID: "3", Name: "Another Decoy", PublicId: "AnotherDecoy"}},
	}

	var requestedOffsets []string
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(httpRequest *http.Request) (int, clients.AssetTypePagedResponse) {
		offsetStr := httpRequest.URL.Query().Get("offset")
		requestedOffsets = append(requestedOffsets, offsetStr)
		offset, _ := strconv.Atoi(offsetStr)
		if offset < 0 || offset >= len(pages) {
			return http.StatusOK, clients.AssetTypePagedResponse{Total: total, Offset: int64(offset), Limit: 1, Results: nil}
		}
		return http.StatusOK, clients.AssetTypePagedResponse{
			Total:   total,
			Offset:  int64(offset),
			Limit:   1,
			Results: pages[offset],
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{PublicId: "issue"})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(requestedOffsets) < 3 {
		t.Fatalf("Expected the scan to page through all %d pages, only requested: %v", total, requestedOffsets)
	}
	if output.Status != tools.StatusSuccess {
		t.Fatalf("Expected status success, got: %s", output.Status)
	}
	if !output.ScanExhaustive {
		t.Fatalf("Expected the multi-page scan to complete and be reported exhaustive")
	}
	if output.Scanned != total {
		t.Fatalf("Expected %d asset types scanned, got: %d", total, output.Scanned)
	}
	if output.Total != 1 || len(output.AssetTypes) != 1 || output.AssetTypes[0].PublicId != "Issue" {
		t.Fatalf("Expected exactly the case-insensitively matching 'Issue' publicId, got: %+v", output.AssetTypes)
	}
}

// TestListAssetTypes_ScanCapped_NotExhaustive asserts AC-3: when a
// publicId/product scan can't reach the end of a catalog larger than the
// scan cap, the response reports scanExhaustive=false rather than silently
// returning a partial result as if it were complete.
func TestListAssetTypes_ScanCapped_NotExhaustive(t *testing.T) {
	const catalogSize = 6000 // larger than the tool's scan cap
	const pageSize = 1000

	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(httpRequest *http.Request) (int, clients.AssetTypePagedResponse) {
		offset, _ := strconv.Atoi(httpRequest.URL.Query().Get("offset"))
		results := make([]clients.AssetTypeDetails, 0, pageSize)
		for i := 0; i < pageSize && offset+i < catalogSize; i++ {
			results = append(results, clients.AssetTypeDetails{
				ID:       fmt.Sprintf("id-%d", offset+i),
				Name:     fmt.Sprintf("ASSET_TYPE_%d", offset+i),
				PublicId: fmt.Sprintf("AssetType%d", offset+i),
				Product:  "Data Governance Center", // never matches the filter below
			})
		}
		return http.StatusOK, clients.AssetTypePagedResponse{
			Total:   catalogSize,
			Offset:  int64(offset),
			Limit:   pageSize,
			Results: results,
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{PublicId: "DoesNotExist"})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != tools.StatusSuccess {
		t.Fatalf("Expected status success, got: %s", output.Status)
	}
	if output.ScanExhaustive {
		t.Fatalf("Expected scanExhaustive=false when the catalog (%d) exceeds the scan cap", catalogSize)
	}
	if output.Scanned <= 0 || output.Scanned >= catalogSize {
		t.Fatalf("Expected a bounded, capped scan count less than the full catalog size, got: %d", output.Scanned)
	}
	if len(output.AssetTypes) != 0 {
		t.Fatalf("Expected no matches for a publicId absent from the (scanned prefix of the) catalog, got: %+v", output.AssetTypes)
	}
}

// TestListAssetTypes_InvalidOffset asserts input validation runs before any
// network call.
func TestListAssetTypes_InvalidOffset(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Offset: -1})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if output.Status != tools.StatusValidationError {
		t.Fatalf("Expected status validation_error, got: %s", output.Status)
	}
}
