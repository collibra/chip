package prepare_add_business_term_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/prepare_add_business_term"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// testDomains returns a standard set of test domains.
func testDomains() clients.PrepareAddBusinessTermDomainsResponse {
	return clients.PrepareAddBusinessTermDomainsResponse{
		Total: 2,
		Results: []clients.PrepareAddBusinessTermDomain{
			{ID: "domain-1", Name: "Finance"},
			{ID: "domain-2", Name: "Marketing"},
		},
	}
}

// testAssetType returns a standard test business term asset type.
func testAssetType() clients.PrepareAddBusinessTermAssetType {
	return clients.PrepareAddBusinessTermAssetType{
		ID:   "asset-type-1",
		Name: "Business Term",
	}
}

// testRawAssignment represents the raw API response shape for assignments.
type testRawAssignment struct {
	ID                                   string                     `json:"id"`
	AssignedCharacteristicTypeReferences []testCharacteristicTypeRef `json:"assignedCharacteristicTypeReferences"`
}

// testCharacteristicTypeRef represents a characteristic type reference in the raw API.
type testCharacteristicTypeRef struct {
	ID                        string         `json:"id"`
	AssignedResourceReference testNamedRef   `json:"assignedResourceReference"`
	MinimumOccurrences        int            `json:"minimumOccurrences"`
	MaximumOccurrences        *int           `json:"maximumOccurrences"`
}

// testNamedRef is a simple id+name+discriminator reference.
type testNamedRef struct {
	ID                    string `json:"id"`
	Name                  string `json:"name"`
	ResourceDiscriminator string `json:"resourceDiscriminator,omitempty"`
}

// testRawAttributeType represents the raw API response shape for an attribute type.
type testRawAttributeType struct {
	ID                         string `json:"id"`
	Name                       string `json:"name"`
	Description                string `json:"description"`
	AttributeTypeDiscriminator string `json:"attributeTypeDiscriminator"`
}

// testAssignments returns a standard set of test assignments in the raw API format.
func testAssignments() []testRawAssignment {
	maxOcc := 1
	return []testRawAssignment{
		{
			ID: "assignment-1",
			AssignedCharacteristicTypeReferences: []testCharacteristicTypeRef{
				{
					ID:                        "ref-1",
					AssignedResourceReference: testNamedRef{ID: "attr-type-1", Name: "Definition", ResourceDiscriminator: "StringAttributeType"},
					MinimumOccurrences:        1,
					MaximumOccurrences:        &maxOcc,
				},
			},
		},
	}
}

// testAttributeType returns a standard test attribute type in the raw API format.
func testAttributeType() testRawAttributeType {
	return testRawAttributeType{
		ID:                         "attr-type-1",
		Name:                       "Definition",
		Description:                "The definition of the business term",
		AttributeTypeDiscriminator: "StringAttributeType",
	}
}

// registerCommonHandlers registers handlers for domain list, asset type, assignments, and attribute types.
func registerCommonHandlers(mux *http.ServeMux) {
	mux.Handle("GET /rest/2.0/domains", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareAddBusinessTermDomainsResponse) {
		return http.StatusOK, testDomains()
	}))

	mux.Handle("GET /rest/2.0/domains/{id}", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.PrepareAddBusinessTermDomain) {
		id := r.PathValue("id")
		for _, d := range testDomains().Results {
			if d.ID == id {
				return http.StatusOK, d
			}
		}
		return http.StatusNotFound, clients.PrepareAddBusinessTermDomain{}
	}))

	mux.Handle("GET /rest/2.0/assetTypes/publicId/{publicId}", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareAddBusinessTermAssetType) {
		return http.StatusOK, testAssetType()
	}))

	mux.Handle("GET /rest/2.0/assignments/assetType/{assetTypeId}", testutil.JsonHandlerOut(func(_ *http.Request) (int, []testRawAssignment) {
		return http.StatusOK, testAssignments()
	}))

	mux.Handle("GET /rest/2.0/attributeTypes/{id}", testutil.JsonHandlerOut(func(_ *http.Request) (int, testRawAttributeType) {
		return http.StatusOK, testAttributeType()
	}))
}

func TestPrepareAddBusinessTermReady(t *testing.T) {
	mux := http.NewServeMux()
	registerCommonHandlers(mux)

	// No duplicates found.
	mux.Handle("GET /rest/2.0/assets", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareAddBusinessTermAssetsResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermAssetsResponse{Total: 0, Results: []clients.PrepareAddBusinessTermAsset{}}
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := prepare_add_business_term.NewTool(client).Handler(t.Context(), prepare_add_business_term.Input{
		Name:     "Revenue",
		DomainID: "domain-1",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "ready" {
		t.Errorf("Expected status 'ready', got: '%s'", output.Status)
	}
	if output.ResolvedDomain == nil {
		t.Fatalf("Expected resolved domain, got nil")
	}
	if output.ResolvedDomain.ID != "domain-1" {
		t.Errorf("Expected resolved domain ID 'domain-1', got: '%s'", output.ResolvedDomain.ID)
	}
	if output.ResolvedDomain.Name != "Finance" {
		t.Errorf("Expected resolved domain name 'Finance', got: '%s'", output.ResolvedDomain.Name)
	}
	if len(output.AttributeSchema) != 1 {
		t.Fatalf("Expected 1 attribute schema entry, got: %d", len(output.AttributeSchema))
	}
	schema := output.AttributeSchema[0]
	if schema.ID != "attr-type-1" {
		t.Errorf("Expected attribute ID 'attr-type-1', got: '%s'", schema.ID)
	}
	if schema.Kind != "StringAttributeType" {
		t.Errorf("Expected attribute kind 'StringAttributeType', got: '%s'", schema.Kind)
	}
	if !schema.Required {
		t.Errorf("Expected attribute to be required")
	}
	if len(output.Duplicates) != 0 {
		t.Errorf("Expected no duplicates, got: %d", len(output.Duplicates))
	}
}

func TestPrepareAddBusinessTermReadyWithDomainName(t *testing.T) {
	mux := http.NewServeMux()
	registerCommonHandlers(mux)

	mux.Handle("GET /rest/2.0/assets", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareAddBusinessTermAssetsResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermAssetsResponse{Total: 0, Results: []clients.PrepareAddBusinessTermAsset{}}
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := prepare_add_business_term.NewTool(client).Handler(t.Context(), prepare_add_business_term.Input{
		Name:       "Revenue",
		DomainName: "finance", // case-insensitive match
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "ready" {
		t.Errorf("Expected status 'ready', got: '%s'", output.Status)
	}
	if output.ResolvedDomain == nil {
		t.Fatalf("Expected resolved domain, got nil")
	}
	if output.ResolvedDomain.ID != "domain-1" {
		t.Errorf("Expected resolved domain ID 'domain-1', got: '%s'", output.ResolvedDomain.ID)
	}
}

func TestPrepareAddBusinessTermIncomplete_MissingName(t *testing.T) {
	mux := http.NewServeMux()
	registerCommonHandlers(mux)

	// No duplicate search when name is empty — assets endpoint should not be called,
	// but register it anyway for safety.
	mux.Handle("GET /rest/2.0/assets", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareAddBusinessTermAssetsResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermAssetsResponse{Total: 0, Results: []clients.PrepareAddBusinessTermAsset{}}
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := prepare_add_business_term.NewTool(client).Handler(t.Context(), prepare_add_business_term.Input{
		Name:     "",
		DomainID: "domain-1",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "incomplete" {
		t.Errorf("Expected status 'incomplete', got: '%s'", output.Status)
	}
	if len(output.AvailableDomains) == 0 {
		t.Errorf("Expected available domains to be pre-fetched")
	}
	if len(output.AttributeSchema) == 0 {
		t.Errorf("Expected attribute schema to be hydrated")
	}
}

func TestPrepareAddBusinessTermIncomplete_MissingDomain(t *testing.T) {
	mux := http.NewServeMux()
	registerCommonHandlers(mux)

	mux.Handle("GET /rest/2.0/assets", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareAddBusinessTermAssetsResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermAssetsResponse{Total: 0, Results: []clients.PrepareAddBusinessTermAsset{}}
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := prepare_add_business_term.NewTool(client).Handler(t.Context(), prepare_add_business_term.Input{
		Name: "Revenue",
		// No domain provided.
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "incomplete" {
		t.Errorf("Expected status 'incomplete', got: '%s'", output.Status)
	}
	if output.ResolvedDomain != nil {
		t.Errorf("Expected nil resolved domain, got: %+v", output.ResolvedDomain)
	}
	if len(output.AvailableDomains) != 2 {
		t.Errorf("Expected 2 available domains, got: %d", len(output.AvailableDomains))
	}
}

func TestPrepareAddBusinessTermNeedsClarification(t *testing.T) {
	mux := http.NewServeMux()

	// Return two domains with the same name to trigger clarification.
	mux.Handle("GET /rest/2.0/domains", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareAddBusinessTermDomainsResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermDomainsResponse{
			Total: 2,
			Results: []clients.PrepareAddBusinessTermDomain{
				{ID: "domain-a", Name: "Sales"},
				{ID: "domain-b", Name: "Sales"},
			},
		}
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := prepare_add_business_term.NewTool(client).Handler(t.Context(), prepare_add_business_term.Input{
		Name:       "Revenue",
		DomainName: "Sales",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "needs_clarification" {
		t.Errorf("Expected status 'needs_clarification', got: '%s'", output.Status)
	}
	if len(output.AvailableDomains) != 2 {
		t.Errorf("Expected 2 matching domains, got: %d", len(output.AvailableDomains))
	}
}

func TestPrepareAddBusinessTermDuplicateFound(t *testing.T) {
	mux := http.NewServeMux()
	registerCommonHandlers(mux)

	// Return a duplicate asset.
	mux.Handle("GET /rest/2.0/assets", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareAddBusinessTermAssetsResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermAssetsResponse{
			Total: 1,
			Results: []clients.PrepareAddBusinessTermAsset{
				{
					ID:   "existing-asset-1",
					Name: "Revenue",
					Domain: clients.PrepareAddBusinessTermDomain{
						ID:   "domain-1",
						Name: "Finance",
					},
				},
			},
		}
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := prepare_add_business_term.NewTool(client).Handler(t.Context(), prepare_add_business_term.Input{
		Name:     "Revenue",
		DomainID: "domain-1",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "duplicate_found" {
		t.Errorf("Expected status 'duplicate_found', got: '%s'", output.Status)
	}
	if len(output.Duplicates) != 1 {
		t.Fatalf("Expected 1 duplicate, got: %d", len(output.Duplicates))
	}
	if output.Duplicates[0].ID != "existing-asset-1" {
		t.Errorf("Expected duplicate ID 'existing-asset-1', got: '%s'", output.Duplicates[0].ID)
	}
	if output.Duplicates[0].Domain.Name != "Finance" {
		t.Errorf("Expected duplicate domain name 'Finance', got: '%s'", output.Duplicates[0].Domain.Name)
	}
	if len(output.AttributeSchema) == 0 {
		t.Errorf("Expected attribute schema to be present on duplicate_found")
	}
}

func TestPrepareAddBusinessTermAPIError(t *testing.T) {
	mux := http.NewServeMux()

	// Domains endpoint returns an error.
	mux.Handle("GET /rest/2.0/domains", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := prepare_add_business_term.NewTool(client).Handler(t.Context(), prepare_add_business_term.Input{
		Name:     "Revenue",
		DomainID: "domain-1",
	})
	if err == nil {
		t.Fatalf("Expected error, got nil")
	}
}

func TestPrepareAddBusinessTermAssetTypeAPIError(t *testing.T) {
	mux := http.NewServeMux()

	// Domains works fine.
	mux.Handle("GET /rest/2.0/domains", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareAddBusinessTermDomainsResponse) {
		return http.StatusOK, testDomains()
	}))

	// Asset type endpoint returns error.
	mux.Handle("GET /rest/2.0/assetTypes/publicId/{publicId}", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	_, err := prepare_add_business_term.NewTool(client).Handler(t.Context(), prepare_add_business_term.Input{
		Name: "Revenue",
	})
	if err == nil {
		t.Fatalf("Expected error for asset type API failure, got nil")
	}
}

func TestPrepareAddBusinessTermEmptyAssignments(t *testing.T) {
	mux := http.NewServeMux()

	mux.Handle("GET /rest/2.0/domains", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareAddBusinessTermDomainsResponse) {
		return http.StatusOK, testDomains()
	}))
	mux.Handle("GET /rest/2.0/domains/{id}", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareAddBusinessTermDomain) {
		return http.StatusOK, clients.PrepareAddBusinessTermDomain{ID: "domain-1", Name: "Finance"}
	}))
	mux.Handle("GET /rest/2.0/assetTypes/publicId/{publicId}", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareAddBusinessTermAssetType) {
		return http.StatusOK, testAssetType()
	}))

	// Empty assignments — no attributes configured.
	mux.Handle("GET /rest/2.0/assignments/assetType/{assetTypeId}", testutil.JsonHandlerOut(func(_ *http.Request) (int, []testRawAssignment) {
		return http.StatusOK, []testRawAssignment{}
	}))

	mux.Handle("GET /rest/2.0/assets", testutil.JsonHandlerOut(func(_ *http.Request) (int, clients.PrepareAddBusinessTermAssetsResponse) {
		return http.StatusOK, clients.PrepareAddBusinessTermAssetsResponse{Total: 0, Results: []clients.PrepareAddBusinessTermAsset{}}
	}))

	server := httptest.NewServer(mux)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := prepare_add_business_term.NewTool(client).Handler(t.Context(), prepare_add_business_term.Input{
		Name:     "Revenue",
		DomainID: "domain-1",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if output.Status != "ready" {
		t.Errorf("Expected status 'ready', got: '%s'", output.Status)
	}
	if len(output.AttributeSchema) != 0 {
		t.Errorf("Expected 0 attribute schema entries, got: %d", len(output.AttributeSchema))
	}
}
