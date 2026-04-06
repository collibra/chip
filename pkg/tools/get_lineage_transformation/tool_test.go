package get_lineage_transformation_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/get_lineage_transformation"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestGetLineageTransformation(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/technical_lineage_resource/rest/lineageGraphRead/v1/transformations/transform-1", testutil.JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{
			"id":                  "transform-1",
			"name":                "etl_sales_daily",
			"description":         "Daily ETL for sales data",
			"transformationLogic": "SELECT * FROM raw_sales WHERE date = CURRENT_DATE",
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		TransformationId: "transform-1",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !output.Found {
		t.Fatalf("Expected transformation to be found")
	}

	if output.Transformation.Id != "transform-1" {
		t.Fatalf("Expected transformation ID 'transform-1', got: '%s'", output.Transformation.Id)
	}

	if output.Transformation.Name != "etl_sales_daily" {
		t.Fatalf("Expected transformation name 'etl_sales_daily', got: '%s'", output.Transformation.Name)
	}

	if output.Transformation.TransformationLogic == "" {
		t.Fatalf("Expected transformation logic to be present")
	}
}

func TestGetLineageTransformationNotFound(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/technical_lineage_resource/rest/lineageGraphRead/v1/transformations/transform-unknown", testutil.JsonHandlerOut(func(r *http.Request) (int, string) {
		return http.StatusNotFound, "transformation not found"
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		TransformationId: "transform-unknown",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Found {
		t.Fatalf("Expected transformation not to be found")
	}

	if output.Error == "" {
		t.Fatalf("Expected an error message")
	}
}

func TestGetLineageTransformationMissingId(t *testing.T) {
	server := httptest.NewServer(http.NewServeMux())
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Found {
		t.Fatalf("Expected transformation not to be found")
	}

	if output.Error == "" {
		t.Fatalf("Expected an error message")
	}
}
