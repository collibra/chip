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

// newAssetType builds a minimal AssetTypeDetails fixture for stub responses.
func newAssetType(name, publicId, product string) clients.AssetTypeDetails {
	id, _ := uuid.NewUUID()
	return clients.AssetTypeDetails{
		ID:       id.String(),
		Name:     name,
		PublicId: publicId,
		Product:  product,
	}
}

// AC-1: an unfiltered call sends no name param and returns the same
// total/offset/limit/assetTypes shape as before the change, with
// resultsTruncated false and scanned 0.
func TestListAssetTypes_UnfilteredMatchesPreExistingBehavior(t *testing.T) {
	assetTypeId, _ := uuid.NewUUID()
	var gotName string
	var sawName bool
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.AssetTypePagedResponse) {
		gotName, sawName = r.URL.Query().Get("name"), r.URL.Query().Has("name")
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
					Product:            "HELPDESK",
				},
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Limit: 100})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if sawName || gotName != "" {
		t.Fatalf("Expected no name param, got: %q (present: %v)", gotName, sawName)
	}
	if output.Total != 1 {
		t.Fatalf("Expected total 1, got: %d", output.Total)
	}
	if output.Offset != 0 || output.Limit != 100 {
		t.Fatalf("Expected offset 0, limit 100, got offset=%d limit=%d", output.Offset, output.Limit)
	}
	if len(output.AssetTypes) != 1 {
		t.Fatalf("Expected 1 asset type, got: %d", len(output.AssetTypes))
	}
	assetType := output.AssetTypes[0]
	if assetType.Name != "Data Element" || assetType.ID != assetTypeId.String() || assetType.PublicId != "DataElement" {
		t.Fatalf("Unexpected asset type: %+v", assetType)
	}
	if output.ResultsTruncated {
		t.Fatalf("Expected resultsTruncated false, got true")
	}
	if output.Scanned != 0 {
		t.Fatalf("Expected scanned 0, got: %d", output.Scanned)
	}
}

// AC-2 and AC-12: name is forwarded verbatim (trimmed) as a server-side query
// param, no nameMatchMode is sent, and chip performs no client-side name
// matching — a non-matching record still comes back.
func TestListAssetTypes_NameForwardedServerSideNoClientFilter(t *testing.T) {
	var gotName string
	var sawNameMatchMode bool
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.AssetTypePagedResponse) {
		gotName = r.URL.Query().Get("name")
		sawNameMatchMode = r.URL.Query().Has("nameMatchMode")
		return http.StatusOK, clients.AssetTypePagedResponse{
			Total:  1,
			Offset: 0,
			Limit:  100,
			Results: []clients.AssetTypeDetails{
				newAssetType("Completely Unrelated", "SomethingElse", "HELPDESK"),
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: "  Issue  "})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if gotName != "Issue" {
		t.Fatalf("Expected trimmed name 'Issue' sent, got: %q", gotName)
	}
	if sawNameMatchMode {
		t.Fatalf("Expected no nameMatchMode param sent")
	}
	if len(output.AssetTypes) != 1 || output.AssetTypes[0].Name != "Completely Unrelated" {
		t.Fatalf("Expected the non-matching record to be returned unfiltered, got: %+v", output.AssetTypes)
	}
}

// AC-3: publicId filters case-insensitively, with a match beyond the first
// scanned page.
func TestListAssetTypes_PublicIdFilterSpansPages(t *testing.T) {
	const total = 1500
	const needleIndex = 1200
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.AssetTypePagedResponse) {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		results := pageOf(total, offset, limit, func(i int) clients.AssetTypeDetails {
			if i == needleIndex {
				return newAssetType("Needle", "TargetPublicId", "HELPDESK")
			}
			return newAssetType(fmt.Sprintf("Other %d", i), fmt.Sprintf("Other-%d", i), "HELPDESK")
		})
		return http.StatusOK, clients.AssetTypePagedResponse{Total: total, Offset: int64(offset), Limit: int64(limit), Results: results}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{PublicId: "target"})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(output.AssetTypes) != 1 {
		t.Fatalf("Expected exactly 1 match, got: %d (%+v)", len(output.AssetTypes), output.AssetTypes)
	}
	if output.AssetTypes[0].PublicId != "TargetPublicId" {
		t.Fatalf("Expected the needle record, got: %+v", output.AssetTypes[0])
	}
	if output.ResultsTruncated {
		t.Fatalf("Expected resultsTruncated false")
	}
	if output.Scanned != total {
		t.Fatalf("Expected scanned %d, got: %d", total, output.Scanned)
	}
}

// AC-4: product filters case-insensitively.
func TestListAssetTypes_ProductFilterCaseInsensitive(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.AssetTypePagedResponse) {
		results := []clients.AssetTypeDetails{
			newAssetType("Issue", "Issue", "HELPDESK"),
			newAssetType("Term", "BusinessTerm", "GLOSSARY"),
		}
		return http.StatusOK, clients.AssetTypePagedResponse{Total: int64(len(results)), Results: results}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Product: "helpdesk"})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(output.AssetTypes) != 1 || output.AssetTypes[0].Name != "Issue" {
		t.Fatalf("Expected only the HELPDESK record, got: %+v", output.AssetTypes)
	}
}

// AC-5: name is sent server-side, and publicId/product filter on top of the
// records returned for that name — an AND, not an OR.
func TestListAssetTypes_NameCombinedWithProductIsAnd(t *testing.T) {
	var gotName string
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.AssetTypePagedResponse) {
		gotName = r.URL.Query().Get("name")
		results := []clients.AssetTypeDetails{
			newAssetType("Issue", "Issue", "HELPDESK"),
			newAssetType("Issue Category", "IssueCategory", "GLOSSARY"),
		}
		return http.StatusOK, clients.AssetTypePagedResponse{Total: int64(len(results)), Results: results}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: "Issue", Product: "HELPDESK"})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if gotName != "Issue" {
		t.Fatalf("Expected name 'Issue' sent to the API, got: %q", gotName)
	}
	if len(output.AssetTypes) != 1 || output.AssetTypes[0].PublicId != "Issue" {
		t.Fatalf("Expected only the HELPDESK record to survive the product filter, got: %+v", output.AssetTypes)
	}
}

// AC-6: a record with no product never satisfies a product filter, and is
// returned unchanged when no product filter is supplied.
func TestListAssetTypes_MissingProductNeverMatchesFilter(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.AssetTypePagedResponse) {
		results := []clients.AssetTypeDetails{newAssetType("No Product", "NoProduct", "")}
		return http.StatusOK, clients.AssetTypePagedResponse{Total: int64(len(results)), Results: results}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)

	filtered, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Product: "HELPDESK"})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if len(filtered.AssetTypes) != 0 {
		t.Fatalf("Expected no match against a productless record, got: %+v", filtered.AssetTypes)
	}

	unfiltered, err := tools.NewTool(client).Handler(t.Context(), tools.Input{PublicId: "NoProduct"})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if len(unfiltered.AssetTypes) != 1 || unfiltered.AssetTypes[0].Product != "" {
		t.Fatalf("Expected the productless record returned unchanged, got: %+v", unfiltered.AssetTypes)
	}
}

// AC-7 and AC-8: resultsTruncated/scanned are always present, and false/actual
// count for a catalog smaller than the scan cap.
func TestListAssetTypes_ScanBelowCapNotTruncated(t *testing.T) {
	const total = 50
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.AssetTypePagedResponse) {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		results := pageOf(total, offset, limit, func(i int) clients.AssetTypeDetails {
			return newAssetType(fmt.Sprintf("Type %d", i), fmt.Sprintf("Type-%d", i), "HELPDESK")
		})
		return http.StatusOK, clients.AssetTypePagedResponse{Total: total, Offset: int64(offset), Limit: int64(limit), Results: results}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Product: "helpdesk"})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.ResultsTruncated {
		t.Fatalf("Expected resultsTruncated false for a catalog below the scan cap")
	}
	if output.Scanned != total {
		t.Fatalf("Expected scanned %d, got: %d", total, output.Scanned)
	}
}

// AC-9: a catalog larger than the scan cap returns successfully, truncated,
// having scanned exactly the cap.
func TestListAssetTypes_ScanAboveCapIsTruncated(t *testing.T) {
	const total = 10000
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.AssetTypePagedResponse) {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		results := pageOf(total, offset, limit, func(i int) clients.AssetTypeDetails {
			return newAssetType(fmt.Sprintf("Type %d", i), fmt.Sprintf("Type-%d", i), "HELPDESK")
		})
		return http.StatusOK, clients.AssetTypePagedResponse{Total: total, Offset: int64(offset), Limit: int64(limit), Results: results}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Product: "nonexistent-product"})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !output.ResultsTruncated {
		t.Fatalf("Expected resultsTruncated true above the scan cap")
	}
	if output.Scanned != 5000 {
		t.Fatalf("Expected scanned to equal the scan cap constant (5000), got: %d", output.Scanned)
	}
}

// AC-10: the scan must advance by the number of records actually returned,
// not by the requested page size, and must not treat a short page as the end
// of the catalog. A server that clamps every page to 200 records while
// reporting total: 2500 must still be examined in full.
func TestListAssetTypes_ScanAdvancesByActualPageSizeNotRequestedLimit(t *testing.T) {
	const total = 2500
	const serverPageCap = 200
	const needleIndex = 2200
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.AssetTypePagedResponse) {
		offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		// Ignore the requested limit entirely: always return at most
		// serverPageCap records, regardless of what was asked for.
		results := pageOf(total, offset, serverPageCap, func(i int) clients.AssetTypeDetails {
			if i == needleIndex {
				return newAssetType("Needle", "TargetPublicId", "HELPDESK")
			}
			return newAssetType(fmt.Sprintf("Type %d", i), fmt.Sprintf("Type-%d", i), "HELPDESK")
		})
		return http.StatusOK, clients.AssetTypePagedResponse{Total: total, Offset: int64(offset), Limit: serverPageCap, Results: results}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{PublicId: "target"})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Scanned != total {
		t.Fatalf("Expected scanned %d (full catalog), got: %d — a naive offset+=requestedLimit loop would stop early", total, output.Scanned)
	}
	if output.ResultsTruncated {
		t.Fatalf("Expected resultsTruncated false: the whole catalog was examined")
	}
	if len(output.AssetTypes) != 1 || output.AssetTypes[0].PublicId != "TargetPublicId" {
		t.Fatalf("Expected the needle at index %d to be found, got: %+v", needleIndex, output.AssetTypes)
	}
}

// AC-11: total on a filtered response is the match count, and assetTypes is
// the offset/limit window over the matches.
func TestListAssetTypes_TotalIsMatchCountAndAssetTypesIsWindowed(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.AssetTypePagedResponse) {
		results := make([]clients.AssetTypeDetails, 0, 5)
		for i := 0; i < 5; i++ {
			results = append(results, newAssetType(fmt.Sprintf("Match %d", i), fmt.Sprintf("Match-%d", i), "HELPDESK"))
		}
		return http.StatusOK, clients.AssetTypePagedResponse{Total: int64(len(results)), Results: results}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Product: "helpdesk", Offset: 1, Limit: 2})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Total != 5 {
		t.Fatalf("Expected total 5 (match count, not a page size), got: %d", output.Total)
	}
	if len(output.AssetTypes) != 2 {
		t.Fatalf("Expected a 2-item window, got: %d", len(output.AssetTypes))
	}
	if output.AssetTypes[0].PublicId != "Match-1" || output.AssetTypes[1].PublicId != "Match-2" {
		t.Fatalf("Expected the offset=1,limit=2 window over the matches, got: %+v", output.AssetTypes)
	}
}

// AC-13: a limit outside [1, 1000] is a validation_error, never forwarded to
// DGC.
func TestListAssetTypes_LimitOutOfRangeIsValidationError(t *testing.T) {
	requestSent := false
	handler := http.NewServeMux()
	handler.Handle("/rest/2.0/assetTypes", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.AssetTypePagedResponse) {
		requestSent = true
		return http.StatusOK, clients.AssetTypePagedResponse{}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)

	for _, limit := range []int{-1, 1001, 5000} {
		requestSent = false
		output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Limit: limit})
		if err != nil {
			t.Fatalf("Expected no error (validation errors are returned in Output), got: %v", err)
		}
		if output.Status != tools.StatusValidationError {
			t.Fatalf("limit=%d: expected status validation_error, got: %q", limit, output.Status)
		}
		if requestSent {
			t.Fatalf("limit=%d: expected no request forwarded to DGC", limit)
		}
	}

	// Boundary values are accepted.
	for _, limit := range []int{1, 1000} {
		output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Limit: limit})
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if output.Status != tools.StatusSuccess {
			t.Fatalf("limit=%d: expected status success, got: %q", limit, output.Status)
		}
	}
}

// pageOf builds the [offset, offset+limit) slice of a total-sized catalog
// using build to construct each record by its absolute index.
func pageOf(total, offset, limit int, build func(i int) clients.AssetTypeDetails) []clients.AssetTypeDetails {
	if offset >= total {
		return nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	results := make([]clients.AssetTypeDetails, 0, end-offset)
	for i := offset; i < end; i++ {
		results = append(results, build(i))
	}
	return results
}
