package clients

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/testutil"
)

const assetID = "9179b887-04ef-4ce5-ab3a-b5bbd39ea3c8"

func TestRequireAssetReturnsIdentity(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/2.0/assets/"+assetID, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"` + assetID + `","name":"Customer","type":{"id":"00000000-0000-0000-0000-000000011001","name":"Business Term"}}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	asset, err := RequireAsset(t.Context(), testutil.NewClient(server), "business term", assetID)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if asset.Name != "Customer" || asset.Type.Name != "Business Term" {
		t.Fatalf("expected the asset name and type to decode, got %+v", asset)
	}
}

func TestRequireAssetLabelsTheMissingId(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()

	_, err := RequireAsset(t.Context(), testutil.NewClient(server), "business term", assetID)
	if err == nil {
		t.Fatal("expected an error for a missing asset")
	}
	if !strings.Contains(err.Error(), "business term") || !strings.Contains(err.Error(), assetID) {
		t.Fatalf("expected the error to name the label and id, got: %v", err)
	}
}

func TestRequireAssetSurfacesOtherStatuses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"statusCode":403}`))
	}))
	defer server.Close()

	_, err := RequireAsset(t.Context(), testutil.NewClient(server), "business term", assetID)
	if err == nil || strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected a 403 to stay distinct from not-found, got: %v", err)
	}
}
