package tools_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/tools"
)

func TestSearchLineageTransformations(t *testing.T) {
	handler := http.NewServeMux()
	handler.Handle("/technical_lineage_resource/rest/lineageGraphRead/v1/transformations", JsonHandlerOut(func(r *http.Request) (int, map[string]any) {
		return http.StatusOK, map[string]any{
			"results": []map[string]any{
				{
					"id":          "transform-1",
					"name":        "etl_sales_daily",
					"description": "Daily ETL for sales data",
				},
			},
			"pagination": map[string]any{
				"nextCursor": "cursor-abc",
			},
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewSearchLineageTransformationsTool(client).Handler(t.Context(), tools.SearchLineageTransformationsInput{
		NameContains: "etl",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(output.Results) != 1 {
		t.Fatalf("Expected 1 result, got: %d", len(output.Results))
	}

	transformation := output.Results[0]
	if transformation.Id != "transform-1" {
		t.Fatalf("Expected transformation ID 'transform-1', got: '%s'", transformation.Id)
	}

	if transformation.Name != "etl_sales_daily" {
		t.Fatalf("Expected transformation name 'etl_sales_daily', got: '%s'", transformation.Name)
	}

	if output.Pagination == nil || output.Pagination.NextCursor != "cursor-abc" {
		t.Fatalf("Expected nextCursor 'cursor-abc'")
	}
}
