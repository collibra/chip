package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAddClassificationMatch_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/rest/catalog/1.0/dataClassification/classificationMatches" {
			t.Errorf("Expected path /rest/catalog/1.0/dataClassification/classificationMatches, got %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"id": "12345678-1234-1234-1234-123456789abc",
			"createdBy": "4d250cc5-e583-4640-9874-b93d82c7a6cb",
			"createdOn": 1475503010320,
			"lastModifiedBy": "4d250cc5-e583-4640-9874-b93d82c7a6cb",
			"lastModifiedOn": 1475503010320,
			"system": false,
			"resourceType": "ClassificationMatch",
			"status": "SUGGESTED",
			"confidence": 0.95,
			"asset": {
				"id": "9179b887-04ef-4ce5-ab3a-b5bbd39ea3c8",
				"resourceType": "Asset",
				"name": "Customer Email Column"
			},
			"classification": {
				"id": "be45c001-b173-48ff-ac91-3f6e45868c8b",
				"name": "Email Address"
			}
		}`))
	}))
	defer server.Close()

	client := &http.Client{
		Transport: &classificationMatchMockTransport{baseURL: server.URL},
	}

	input := AddDataClassificationMatchInput{
		AssetID:          "9179b887-04ef-4ce5-ab3a-b5bbd39ea3c8",
		ClassificationID: "be45c001-b173-48ff-ac91-3f6e45868c8b",
	}

	ctx := context.Background()
	output, err := handleAddClassificationMatch(client)(ctx, input)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !output.Success {
		t.Errorf("Expected success=true, got false. Error: %s", output.Error)
	}

	if output.Match == nil {
		t.Fatal("Expected match to be returned")
	}

	if output.Match.ID != "12345678-1234-1234-1234-123456789abc" {
		t.Errorf("Expected specific ID, got '%s'", output.Match.ID)
	}

	if output.Match.Status != "SUGGESTED" {
		t.Errorf("Expected status='SUGGESTED', got '%s'", output.Match.Status)
	}

	if output.Match.Confidence != 0.95 {
		t.Errorf("Expected confidence=0.95, got %f", output.Match.Confidence)
	}
}

func TestAddClassificationMatch_MissingAssetID(t *testing.T) {
	client := &http.Client{}

	input := AddDataClassificationMatchInput{
		ClassificationID: "be45c001-b173-48ff-ac91-3f6e45868c8b",
	}

	ctx := context.Background()
	output, err := handleAddClassificationMatch(client)(ctx, input)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if output.Success {
		t.Error("Expected success=false for missing asset ID")
	}

	if output.Error == "" {
		t.Error("Expected error message for missing asset ID")
	}
}

func TestAddClassificationMatch_MissingClassificationID(t *testing.T) {
	client := &http.Client{}

	input := AddDataClassificationMatchInput{
		AssetID: "9179b887-04ef-4ce5-ab3a-b5bbd39ea3c8",
	}

	ctx := context.Background()
	output, err := handleAddClassificationMatch(client)(ctx, input)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if output.Success {
		t.Error("Expected success=false for missing classification ID")
	}

	if output.Error == "" {
		t.Error("Expected error message for missing classification ID")
	}
}

func TestAddClassificationMatch_AssetNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{
			"statusCode": 404,
			"message": "Asset not found"
		}`))
	}))
	defer server.Close()

	client := &http.Client{
		Transport: &classificationMatchMockTransport{baseURL: server.URL},
	}

	input := AddDataClassificationMatchInput{
		AssetID:          "00000000-0000-0000-0000-000000000000",
		ClassificationID: "be45c001-b173-48ff-ac91-3f6e45868c8b",
	}

	ctx := context.Background()
	output, err := handleAddClassificationMatch(client)(ctx, input)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if output.Success {
		t.Error("Expected success=false for asset not found")
	}

	if output.Error == "" {
		t.Error("Expected error message for asset not found")
	}
}

func TestAddClassificationMatch_AlreadyExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{
			"statusCode": 422,
			"message": "Classification match already exists"
		}`))
	}))
	defer server.Close()

	client := &http.Client{
		Transport: &classificationMatchMockTransport{baseURL: server.URL},
	}

	input := AddDataClassificationMatchInput{
		AssetID:          "9179b887-04ef-4ce5-ab3a-b5bbd39ea3c8",
		ClassificationID: "be45c001-b173-48ff-ac91-3f6e45868c8b",
	}

	ctx := context.Background()
	output, err := handleAddClassificationMatch(client)(ctx, input)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if output.Success {
		t.Error("Expected success=false for already existing match")
	}

	if output.Error == "" {
		t.Error("Expected error message for already existing match")
	}
}

type classificationMatchMockTransport struct {
	baseURL string
}

func (t *classificationMatchMockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "http"
	if req.URL.Host == "" {
		req.URL.Host = t.baseURL[7:]
	}
	return http.DefaultTransport.RoundTrip(req)
}
