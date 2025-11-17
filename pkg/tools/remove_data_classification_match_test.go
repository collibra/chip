package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRemoveClassificationMatch_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "DELETE" {
			t.Errorf("Expected DELETE request, got %s", r.Method)
		}
		if r.URL.Path != "/rest/catalog/1.0/dataClassification/classificationMatches/12345678-1234-1234-1234-123456789abc" {
			t.Errorf("Expected path /rest/catalog/1.0/dataClassification/classificationMatches/12345678-1234-1234-1234-123456789abc, got %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := &http.Client{
		Transport: &classificationMatchMockTransport{baseURL: server.URL},
	}

	input := RemoveDataClassificationMatchInput{
		ClassificationMatchID: "12345678-1234-1234-1234-123456789abc",
	}

	ctx := context.Background()
	output, err := handleRemoveDataClassificationMatch(ctx, client, input)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !output.Success {
		t.Errorf("Expected success=true, got false. Error: %s", output.Error)
	}
}

func TestRemoveClassificationMatch_MissingClassificationMatchID(t *testing.T) {
	client := &http.Client{}

	input := RemoveDataClassificationMatchInput{}

	ctx := context.Background()
	output, err := handleRemoveDataClassificationMatch(ctx, client, input)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if output.Success {
		t.Error("Expected success=false for missing classification match ID")
	}

	if output.Error == "" {
		t.Error("Expected error message for missing classification match ID")
	}
}

func TestRemoveClassificationMatch_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := &http.Client{
		Transport: &classificationMatchMockTransport{baseURL: server.URL},
	}

	input := RemoveDataClassificationMatchInput{
		ClassificationMatchID: "00000000-0000-0000-0000-000000000000",
	}

	ctx := context.Background()
	output, err := handleRemoveDataClassificationMatch(ctx, client, input)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if output.Success {
		t.Error("Expected success=false for classification match not found")
	}

	if output.Error == "" {
		t.Error("Expected error message for classification match not found")
	}
}

func TestRemoveClassificationMatch_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{
			"statusCode": 500,
			"message": "Internal server error"
		}`))
	}))
	defer server.Close()

	client := &http.Client{
		Transport: &classificationMatchMockTransport{baseURL: server.URL},
	}

	input := RemoveDataClassificationMatchInput{
		ClassificationMatchID: "12345678-1234-1234-1234-123456789abc",
	}

	ctx := context.Background()
	output, err := handleRemoveDataClassificationMatch(ctx, client, input)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if output.Success {
		t.Error("Expected success=false for server error")
	}

	if output.Error == "" {
		t.Error("Expected error message for server error")
	}
}
