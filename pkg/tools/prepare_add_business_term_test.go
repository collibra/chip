package tools_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools"
)

// newFullHandler creates a test mux with all endpoints configured for the happy path.
func newFullHandler() *http.ServeMux {
	handler := http.NewServeMux()

	// GET /rest/2.0/domains - list all domains (paginated response)
	handler.Handle("GET /rest/2.0/domains", JsonHandlerOut(func(r *http.Request) (int, clients.PrepareAddBusinessTermDomainPagedResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermDomainPagedResponse{
			Results: []clients.PrepareAddBusinessTermDomainResponse{
				{ID: "domain-1", Name: "Business Glossary", Description: "Main glossary domain"},
				{ID: "domain-2", Name: "Technical Glossary", Description: "Technical terms"},
			},
			Total: 2,
		}
	}))

	// GET /rest/2.0/domains/{id} - get domain by ID
	handler.Handle("GET /rest/2.0/domains/{id}", JsonHandlerOut(func(r *http.Request) (int, clients.PrepareAddBusinessTermDomainResponse) {
		domainID := r.PathValue("id")
		if domainID == "domain-1" {
			return http.StatusOK, clients.PrepareAddBusinessTermDomainResponse{
				ID: "domain-1", Name: "Business Glossary", Description: "Main glossary domain",
			}
		}
		return http.StatusNotFound, clients.PrepareAddBusinessTermDomainResponse{}
	}))

	// GET /rest/2.0/assetTypes/publicId/{publicId} - get asset type
	handler.Handle("GET /rest/2.0/assetTypes/publicId/{publicId}", JsonHandlerOut(func(r *http.Request) (int, clients.PrepareAddBusinessTermAssetTypeResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermAssetTypeResponse{
			ID:       "at-bt-1",
			PublicID: clients.BusinessTermAssetTypePublicID,
			Name:     "Business Term",
		}
	}))

	// GET /rest/2.0/assets - search for duplicates
	handler.Handle("GET /rest/2.0/assets", JsonHandlerOut(func(r *http.Request) (int, clients.PrepareAddBusinessTermSearchAssetsResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermSearchAssetsResponse{
			Results: []clients.PrepareAddBusinessTermAssetResponse{},
			Total:   0,
		}
	}))

	// GET /rest/2.0/assignments/assetType/{id} - get assignments
	handler.Handle("GET /rest/2.0/assignments/assetType/{id}", JsonHandlerOut(func(r *http.Request) (int, []clients.PrepareAddBusinessTermAssignmentResponse) {
		return http.StatusOK, []clients.PrepareAddBusinessTermAssignmentResponse{
			{
				ID: "assign-1",
				AssignedCharacteristicTypeReferences: []clients.PrepareAddBusinessTermCharacteristicTypeRef{
					{
						ID:                        "ref-1",
						AssignedResourceReference: clients.PrepareAddBusinessTermResourceRef{ID: "attr-type-def", Name: "Definition", ResourceDiscriminator: "StringAttributeType"},
						MinimumOccurrences:        1,
					},
					{
						ID:                        "ref-2",
						AssignedResourceReference: clients.PrepareAddBusinessTermResourceRef{ID: "attr-type-note", Name: "Note", ResourceDiscriminator: "StringAttributeType"},
						MinimumOccurrences:        0,
					},
				},
			},
		}
	}))

	// GET /rest/2.0/attributeTypes/{id} - get attribute type details
	handler.Handle("GET /rest/2.0/attributeTypes/{id}", JsonHandlerOut(func(r *http.Request) (int, clients.PrepareAddBusinessTermAttributeTypeResponse) {
		attrID := r.PathValue("id")
		switch attrID {
		case "attr-type-def":
			return http.StatusOK, clients.PrepareAddBusinessTermAttributeTypeResponse{
				ID:          "attr-type-def",
				Name:        "Definition",
				Kind:        "String",
				Description: "The definition of the business term",
				Constraints: &clients.PrepareAddBusinessTermConstraintsResponse{
					MinLength: 1,
					MaxLength: 4000,
				},
				RelationTypes: []clients.PrepareAddBusinessTermRelationTypeResponse{
					{ID: "rt-1", Name: "is related to", Direction: "outgoing", TargetAssetTypeID: "at-bt-1"},
				},
			}
		case "attr-type-note":
			return http.StatusOK, clients.PrepareAddBusinessTermAttributeTypeResponse{
				ID:          "attr-type-note",
				Name:        "Note",
				Kind:        "String",
				Description: "Additional notes",
			}
		default:
			return http.StatusNotFound, clients.PrepareAddBusinessTermAttributeTypeResponse{}
		}
	}))

	return handler
}

func TestPrepareAddBusinessTermReady(t *testing.T) {
	server := httptest.NewServer(newFullHandler())
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewPrepareAddBusinessTermTool(client).Handler(t.Context(), tools.PrepareAddBusinessTermInput{
		Name:     "Revenue",
		DomainID: "domain-1",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "ready" {
		t.Errorf("Expected status 'ready', got '%s'", output.Status)
	}
	if output.Domain == nil {
		t.Fatalf("Expected domain to be set")
	}
	if output.Domain.ID != "domain-1" {
		t.Errorf("Expected domain ID 'domain-1', got '%s'", output.Domain.ID)
	}
	if output.Domain.Name != "Business Glossary" {
		t.Errorf("Expected domain name 'Business Glossary', got '%s'", output.Domain.Name)
	}
	if len(output.AttributeSchema) != 2 {
		t.Fatalf("Expected 2 attributes in schema, got %d", len(output.AttributeSchema))
	}

	// Check first attribute (Definition - required)
	defAttr := output.AttributeSchema[0]
	if defAttr.Name != "Definition" {
		t.Errorf("Expected first attribute name 'Definition', got '%s'", defAttr.Name)
	}
	if defAttr.Kind != "String" {
		t.Errorf("Expected attribute kind 'String', got '%s'", defAttr.Kind)
	}
	if !defAttr.Required {
		t.Errorf("Expected Definition attribute to be required")
	}
	if defAttr.Constraints == nil {
		t.Fatalf("Expected Definition attribute to have constraints")
	}
	if defAttr.Constraints.MinLength != 1 {
		t.Errorf("Expected MinLength 1, got %d", defAttr.Constraints.MinLength)
	}
	if defAttr.Constraints.MaxLength != 4000 {
		t.Errorf("Expected MaxLength 4000, got %d", defAttr.Constraints.MaxLength)
	}
	if len(defAttr.RelationTypes) != 1 {
		t.Fatalf("Expected 1 relation type, got %d", len(defAttr.RelationTypes))
	}
	if defAttr.RelationTypes[0].Direction != "outgoing" {
		t.Errorf("Expected relation direction 'outgoing', got '%s'", defAttr.RelationTypes[0].Direction)
	}

	// Check second attribute (Note - not required, no constraints)
	noteAttr := output.AttributeSchema[1]
	if noteAttr.Name != "Note" {
		t.Errorf("Expected second attribute name 'Note', got '%s'", noteAttr.Name)
	}
	if noteAttr.Required {
		t.Errorf("Expected Note attribute to not be required")
	}
	if noteAttr.Constraints != nil {
		t.Errorf("Expected Note attribute to have no constraints")
	}
}

func TestPrepareAddBusinessTermReadyByDomainName(t *testing.T) {
	server := httptest.NewServer(newFullHandler())
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewPrepareAddBusinessTermTool(client).Handler(t.Context(), tools.PrepareAddBusinessTermInput{
		Name:       "Revenue",
		DomainName: "Business Glossary",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "ready" {
		t.Errorf("Expected status 'ready', got '%s': %s", output.Status, output.Message)
	}
	if output.Domain == nil {
		t.Fatalf("Expected domain to be set")
	}
	if output.Domain.ID != "domain-1" {
		t.Errorf("Expected domain ID 'domain-1', got '%s'", output.Domain.ID)
	}
}

func TestPrepareAddBusinessTermDuplicateFound(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("GET /rest/2.0/domains/{id}", JsonHandlerOut(func(r *http.Request) (int, clients.PrepareAddBusinessTermDomainResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermDomainResponse{
			ID: "domain-1", Name: "Business Glossary", Description: "Main glossary domain",
		}
	}))

	handler.Handle("GET /rest/2.0/assetTypes/publicId/{publicId}", JsonHandlerOut(func(r *http.Request) (int, clients.PrepareAddBusinessTermAssetTypeResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermAssetTypeResponse{
			ID: "at-bt-1", PublicID: clients.BusinessTermAssetTypePublicID, Name: "Business Term",
		}
	}))

	handler.Handle("GET /rest/2.0/assets", JsonHandlerOut(func(r *http.Request) (int, clients.PrepareAddBusinessTermSearchAssetsResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermSearchAssetsResponse{
			Results: []clients.PrepareAddBusinessTermAssetResponse{
				{
					ID:   "existing-1",
					Name: "Revenue",
					Type: clients.PrepareAddBusinessTermResourceRef{ID: "at-bt-1", Name: "Business Term"},
					Domain: clients.PrepareAddBusinessTermResourceRef{ID: "domain-1", Name: "Business Glossary"},
					Description: "Existing revenue term",
				},
			},
			Total: 1,
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewPrepareAddBusinessTermTool(client).Handler(t.Context(), tools.PrepareAddBusinessTermInput{
		Name:     "Revenue",
		DomainID: "domain-1",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "duplicate_found" {
		t.Errorf("Expected status 'duplicate_found', got '%s'", output.Status)
	}
	if len(output.Duplicates) != 1 {
		t.Fatalf("Expected 1 duplicate, got %d", len(output.Duplicates))
	}
	if output.Duplicates[0].ID != "existing-1" {
		t.Errorf("Expected duplicate ID 'existing-1', got '%s'", output.Duplicates[0].ID)
	}
	if output.Domain == nil || output.Domain.ID != "domain-1" {
		t.Errorf("Expected resolved domain to be included in duplicate_found response")
	}
}

func TestPrepareAddBusinessTermIncompleteMissingName(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("GET /rest/2.0/domains", JsonHandlerOut(func(r *http.Request) (int, clients.PrepareAddBusinessTermDomainPagedResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermDomainPagedResponse{
			Results: []clients.PrepareAddBusinessTermDomainResponse{
				{ID: "domain-1", Name: "Business Glossary", Description: "Main glossary domain"},
			},
			Total: 1,
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewPrepareAddBusinessTermTool(client).Handler(t.Context(), tools.PrepareAddBusinessTermInput{
		Name: "",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "incomplete" {
		t.Errorf("Expected status 'incomplete', got '%s'", output.Status)
	}
	if len(output.AvailableDomains) != 1 {
		t.Errorf("Expected 1 available domain, got %d", len(output.AvailableDomains))
	}
}

func TestPrepareAddBusinessTermIncompleteMissingDomain(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("GET /rest/2.0/domains", JsonHandlerOut(func(r *http.Request) (int, clients.PrepareAddBusinessTermDomainPagedResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermDomainPagedResponse{
			Results: []clients.PrepareAddBusinessTermDomainResponse{
				{ID: "domain-1", Name: "Business Glossary", Description: "Main glossary domain"},
				{ID: "domain-2", Name: "Technical Glossary", Description: "Technical terms"},
			},
			Total: 2,
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewPrepareAddBusinessTermTool(client).Handler(t.Context(), tools.PrepareAddBusinessTermInput{
		Name: "Revenue",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "incomplete" {
		t.Errorf("Expected status 'incomplete', got '%s'", output.Status)
	}
	if len(output.AvailableDomains) != 2 {
		t.Errorf("Expected 2 available domains, got %d", len(output.AvailableDomains))
	}
}

func TestPrepareAddBusinessTermNeedsClarificationDomainIDNotFound(t *testing.T) {
	handler := http.NewServeMux()

	// Domain lookup returns 404
	handler.Handle("GET /rest/2.0/domains/{id}", JsonHandlerOut(func(r *http.Request) (int, map[string]string) {
		return http.StatusNotFound, map[string]string{"error": "not found"}
	}))

	// List domains for fallback
	handler.Handle("GET /rest/2.0/domains", JsonHandlerOut(func(r *http.Request) (int, clients.PrepareAddBusinessTermDomainPagedResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermDomainPagedResponse{
			Results: []clients.PrepareAddBusinessTermDomainResponse{
				{ID: "domain-1", Name: "Business Glossary", Description: "Main glossary domain"},
			},
			Total: 1,
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewPrepareAddBusinessTermTool(client).Handler(t.Context(), tools.PrepareAddBusinessTermInput{
		Name:     "Revenue",
		DomainID: "invalid-domain-id",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "needs_clarification" {
		t.Errorf("Expected status 'needs_clarification', got '%s'", output.Status)
	}
	if len(output.AvailableDomains) != 1 {
		t.Errorf("Expected 1 available domain, got %d", len(output.AvailableDomains))
	}
}

func TestPrepareAddBusinessTermNeedsClarificationDomainNameNotFound(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("GET /rest/2.0/domains", JsonHandlerOut(func(r *http.Request) (int, clients.PrepareAddBusinessTermDomainPagedResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermDomainPagedResponse{
			Results: []clients.PrepareAddBusinessTermDomainResponse{
				{ID: "domain-1", Name: "Business Glossary", Description: "Main glossary domain"},
			},
			Total: 1,
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewPrepareAddBusinessTermTool(client).Handler(t.Context(), tools.PrepareAddBusinessTermInput{
		Name:       "Revenue",
		DomainName: "Nonexistent Domain",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "needs_clarification" {
		t.Errorf("Expected status 'needs_clarification', got '%s'", output.Status)
	}
	if len(output.AvailableDomains) != 1 {
		t.Errorf("Expected 1 available domain as fallback, got %d", len(output.AvailableDomains))
	}
}

func TestPrepareAddBusinessTermAssetTypeAPIError(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("GET /rest/2.0/domains/{id}", JsonHandlerOut(func(r *http.Request) (int, clients.PrepareAddBusinessTermDomainResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermDomainResponse{
			ID: "domain-1", Name: "Business Glossary", Description: "Main glossary domain",
		}
	}))

	// Asset type endpoint returns 500
	handler.Handle("GET /rest/2.0/assetTypes/publicId/{publicId}", JsonHandlerOut(func(r *http.Request) (int, map[string]string) {
		return http.StatusInternalServerError, map[string]string{"error": "internal server error"}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)
	_, err := tools.NewPrepareAddBusinessTermTool(client).Handler(t.Context(), tools.PrepareAddBusinessTermInput{
		Name:     "Revenue",
		DomainID: "domain-1",
	})
	if err == nil {
		t.Fatalf("Expected an error for API failure, got nil")
	}
}

func TestPrepareAddBusinessTermDomainNameCaseInsensitive(t *testing.T) {
	server := httptest.NewServer(newFullHandler())
	defer server.Close()

	client := newClient(server)
	// Use lowercase when actual name is "Business Glossary"
	output, err := tools.NewPrepareAddBusinessTermTool(client).Handler(t.Context(), tools.PrepareAddBusinessTermInput{
		Name:       "Revenue",
		DomainName: "business glossary",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "ready" {
		t.Errorf("Expected status 'ready' with case-insensitive match, got '%s': %s", output.Status, output.Message)
	}
	if output.Domain == nil || output.Domain.ID != "domain-1" {
		t.Errorf("Expected domain to resolve to domain-1")
	}
}

func TestPrepareAddBusinessTermEmptyAttributeSchema(t *testing.T) {
	handler := http.NewServeMux()

	handler.Handle("GET /rest/2.0/domains/{id}", JsonHandlerOut(func(r *http.Request) (int, clients.PrepareAddBusinessTermDomainResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermDomainResponse{
			ID: "domain-1", Name: "Business Glossary", Description: "Main glossary domain",
		}
	}))

	handler.Handle("GET /rest/2.0/assetTypes/publicId/{publicId}", JsonHandlerOut(func(r *http.Request) (int, clients.PrepareAddBusinessTermAssetTypeResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermAssetTypeResponse{
			ID: "at-bt-1", PublicID: clients.BusinessTermAssetTypePublicID, Name: "Business Term",
		}
	}))

	handler.Handle("GET /rest/2.0/assets", JsonHandlerOut(func(r *http.Request) (int, clients.PrepareAddBusinessTermSearchAssetsResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermSearchAssetsResponse{
			Results: []clients.PrepareAddBusinessTermAssetResponse{},
			Total:   0,
		}
	}))

	// No assignments - empty attribute schema
	handler.Handle("GET /rest/2.0/assignments/assetType/{id}", JsonHandlerOut(func(r *http.Request) (int, []clients.PrepareAddBusinessTermAssignmentResponse) {
		return http.StatusOK, []clients.PrepareAddBusinessTermAssignmentResponse{}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewPrepareAddBusinessTermTool(client).Handler(t.Context(), tools.PrepareAddBusinessTermInput{
		Name:     "Revenue",
		DomainID: "domain-1",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "ready" {
		t.Errorf("Expected status 'ready', got '%s'", output.Status)
	}
	if len(output.AttributeSchema) != 0 {
		t.Errorf("Expected empty attribute schema, got %d attributes", len(output.AttributeSchema))
	}
}

func TestPrepareAddBusinessTermOutputSerialization(t *testing.T) {
	server := httptest.NewServer(newFullHandler())
	defer server.Close()

	client := newClient(server)
	output, err := tools.NewPrepareAddBusinessTermTool(client).Handler(t.Context(), tools.PrepareAddBusinessTermInput{
		Name:     "Revenue",
		DomainID: "domain-1",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Verify the output serializes to valid JSON
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatalf("Failed to marshal output to JSON: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Failed to unmarshal output JSON: %v", err)
	}

	if parsed["status"] != "ready" {
		t.Errorf("Expected serialized status 'ready', got '%v'", parsed["status"])
	}
	if _, ok := parsed["domain"]; !ok {
		t.Errorf("Expected 'domain' field in serialized output")
	}
	if _, ok := parsed["attribute_schema"]; !ok {
		t.Errorf("Expected 'attribute_schema' field in serialized output")
	}
}
