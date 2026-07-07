package catalog_generic_save_config_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/catalog_generic_save_config"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestCatalogGenericSaveConfig(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000001/configuration", testutil.JsonHandlerInOut(func(r *http.Request, in clients.SaveGenericConfigRequest) (int, clients.GenericConfiguration) {
		return http.StatusOK, clients.GenericConfiguration{Id: "cfg-1", IngestibleId: "00000000-0000-0000-0000-000000000001", Value: in.Configuration}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		IngestibleId:  "00000000-0000-0000-0000-000000000001",
		Configuration: `{"catalog":"my_catalog"}`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got: %s", output.Error)
	}
	if output.Config == nil || output.Config.Id != "cfg-1" {
		t.Fatal("expected config id cfg-1")
	}
}

func TestCatalogGenericSaveConfigAPIError(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/rest/catalog/1.0/genericIntegration/00000000-0000-0000-0000-000000000001/configuration", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusBadRequest, "bad request"
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{
		IngestibleId:  "00000000-0000-0000-0000-000000000001",
		Configuration: "{}",
	})
	if err != nil {
		t.Fatalf("unexpected go error: %v", err)
	}
	if output.Success {
		t.Fatal("expected failure")
	}
}

func TestCatalogGenericSaveConfigInvalidId(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	_, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), tools.Input{IngestibleId: "bad", Configuration: "{}"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
