package get_asset_details_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/get_asset_details"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestGetAssetDetails(t *testing.T) {
	assetId, _ := uuid.NewUUID()
	handler := http.NewServeMux()
	handler.Handle("/graphql/knowledgeGraph/v1", testutil.JsonHandlerInOut(func(httpRequest *http.Request, request clients.Request) (int, clients.Response) {
		return http.StatusOK, clients.Response{
			Data: &clients.AssetQueryData{
				Assets: []clients.Asset{
					{
						ID:          assetId.String(),
						DisplayName: "My Asset Name",
					},
				},
			},
		}
	}))
	handler.Handle("/rest/2.0/responsibilities", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.ResponsibilityPagedResponse) {
		return http.StatusOK, clients.ResponsibilityPagedResponse{
			Total:  0,
			Offset: 0,
			Limit:  100,
		}
	}))
	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)

	output, err := tools.NewTool(client, false).Handler(t.Context(), tools.Input{
		AssetID: assetId.String(),
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !output.Found {
		t.Fatalf("Asset not found")
	}
	if output.Asset.DisplayName != "My Asset Name" {
		t.Fatalf("Expected answer 'My Asset Name', got: '%s'", output.Asset.DisplayName)
	}
	if len(output.Responsibilities) != 0 {
		t.Fatalf("Expected no responsibilities, got: %d", len(output.Responsibilities))
	}
	if output.ResponsibilitiesStatus != "No responsibilities assigned" {
		t.Fatalf("Expected 'No responsibilities assigned', got: '%s'", output.ResponsibilitiesStatus)
	}
}

func TestGetAssetDetailsReturnsTags(t *testing.T) {
	assetId, _ := uuid.NewUUID()
	var sentQuery string

	handler := http.NewServeMux()
	handler.Handle("/graphql/knowledgeGraph/v1", testutil.JsonHandlerInOut(func(httpRequest *http.Request, request clients.Request) (int, clients.Response) {
		sentQuery = request.Query
		return http.StatusOK, clients.Response{
			Data: &clients.AssetQueryData{
				Assets: []clients.Asset{
					{
						ID:          assetId.String(),
						DisplayName: "My Asset Name",
						Tags:        []clients.Tag{{ID: "tag-1", Name: "MCP"}},
					},
				},
			},
		}
	}))
	handler.Handle("/rest/2.0/responsibilities", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.ResponsibilityPagedResponse) {
		return http.StatusOK, clients.ResponsibilityPagedResponse{Limit: 100}
	}))
	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server), false).Handler(t.Context(), tools.Input{
		AssetID: assetId.String(),
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !strings.Contains(sentQuery, "tags(limit:") {
		t.Errorf("tags selected without an explicit limit, will truncate at 10:\n%s", sentQuery)
	}
	if len(output.Asset.Tags) != 1 || output.Asset.Tags[0].Name != "MCP" {
		t.Fatalf("Expected one tag named MCP, got: %+v", output.Asset.Tags)
	}
}

func TestGetAssetDetailsWithResponsibilities(t *testing.T) {
	assetId, _ := uuid.NewUUID()
	domainId := "domain-123"
	handler := http.NewServeMux()
	handler.Handle("/graphql/knowledgeGraph/v1", testutil.JsonHandlerInOut(func(httpRequest *http.Request, request clients.Request) (int, clients.Response) {
		return http.StatusOK, clients.Response{
			Data: &clients.AssetQueryData{
				Assets: []clients.Asset{
					{
						ID:          assetId.String(),
						DisplayName: "My Asset Name",
					},
				},
			},
		}
	}))
	handler.Handle("/rest/2.0/responsibilities", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.ResponsibilityPagedResponse) {
		return http.StatusOK, clients.ResponsibilityPagedResponse{
			Total:  3,
			Offset: 0,
			Limit:  100,
			Results: []clients.Responsibility{
				{
					ID:   "resp-1",
					Role: &clients.ResourceRole{ID: "role-1", Name: "Owner"},
					Owner: &clients.ResourceRef{
						ID:                    "user-1",
						ResourceDiscriminator: "User",
					},
					BaseResource: &clients.ResourceRef{
						ID:                    assetId.String(),
						ResourceDiscriminator: "Asset",
					},
				},
				{
					ID:   "resp-2",
					Role: &clients.ResourceRole{ID: "role-2", Name: "Business Steward"},
					Owner: &clients.ResourceRef{
						ID:                    "group-1",
						ResourceDiscriminator: "UserGroup",
					},
					BaseResource: &clients.ResourceRef{
						ID:                    assetId.String(),
						ResourceDiscriminator: "Asset",
					},
				},
				{
					ID:   "resp-3",
					Role: &clients.ResourceRole{ID: "role-3", Name: "Technical Steward"},
					Owner: &clients.ResourceRef{
						ID:                    "user-2",
						ResourceDiscriminator: "User",
					},
					BaseResource: &clients.ResourceRef{
						ID:                    domainId,
						ResourceDiscriminator: "Domain",
					},
				},
			},
		}
	}))
	handler.Handle("/rest/2.0/users/user-1", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.UserResponse) {
		return http.StatusOK, clients.UserResponse{
			ID:        "user-1",
			UserName:  "john.doe",
			FirstName: "John",
			LastName:  "Doe",
		}
	}))
	handler.Handle("/rest/2.0/users/user-2", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.UserResponse) {
		return http.StatusOK, clients.UserResponse{
			ID:        "user-2",
			UserName:  "jane.smith",
			FirstName: "Jane",
			LastName:  "Smith",
		}
	}))
	handler.Handle("/rest/2.0/userGroups/group-1", testutil.JsonHandlerOut(func(r *http.Request) (int, clients.UserGroupResponse) {
		return http.StatusOK, clients.UserGroupResponse{
			ID:   "group-1",
			Name: "Data Governance Team",
		}
	}))
	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)

	output, err := tools.NewTool(client, false).Handler(t.Context(), tools.Input{
		AssetID: assetId.String(),
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if !output.Found {
		t.Fatalf("Asset not found")
	}

	if len(output.Responsibilities) != 3 {
		t.Fatalf("Expected 3 responsibilities, got: %d", len(output.Responsibilities))
	}

	// Direct user assignment
	if output.Responsibilities[0].RoleName != "Owner" {
		t.Fatalf("Expected role 'Owner', got: '%s'", output.Responsibilities[0].RoleName)
	}
	if !strings.Contains(output.Responsibilities[0].UserName, "john.doe") {
		t.Fatalf("Expected user name to contain 'john.doe', got: '%s'", output.Responsibilities[0].UserName)
	}
	if output.Responsibilities[0].Inherited {
		t.Fatalf("Expected direct assignment (inherited=false), got inherited=true")
	}

	// Direct group assignment
	if output.Responsibilities[1].RoleName != "Business Steward" {
		t.Fatalf("Expected role 'Business Steward', got: '%s'", output.Responsibilities[1].RoleName)
	}
	if output.Responsibilities[1].GroupName != "Data Governance Team" {
		t.Fatalf("Expected group 'Data Governance Team', got: '%s'", output.Responsibilities[1].GroupName)
	}
	if output.Responsibilities[1].Inherited {
		t.Fatalf("Expected direct assignment (inherited=false), got inherited=true")
	}

	// Inherited assignment (baseResource ID differs from asset ID)
	if output.Responsibilities[2].RoleName != "Technical Steward" {
		t.Fatalf("Expected role 'Technical Steward', got: '%s'", output.Responsibilities[2].RoleName)
	}
	if !strings.Contains(output.Responsibilities[2].UserName, "jane.smith") {
		t.Fatalf("Expected user name to contain 'jane.smith', got: '%s'", output.Responsibilities[2].UserName)
	}
	if !output.Responsibilities[2].Inherited {
		t.Fatalf("Expected inherited assignment (inherited=true), got inherited=false")
	}

	if output.ResponsibilitiesStatus != "" {
		t.Fatalf("Expected empty responsibilitiesStatus when responsibilities exist, got: '%s'", output.ResponsibilitiesStatus)
	}
}

func TestGetAssetDetailsWithContextSpecification(t *testing.T) {
	assetId, _ := uuid.NewUUID()
	specId, _ := uuid.NewUUID()
	handler := http.NewServeMux()
	handler.Handle("/graphql/knowledgeGraph/v1", testutil.JsonHandlerInOut(func(_ *http.Request, _ clients.Request) (int, clients.Response) {
		return http.StatusOK, clients.Response{
			Data: &clients.AssetQueryData{
				Assets: []clients.Asset{{ID: assetId.String(), DisplayName: "My Asset"}},
			},
		}
	}))
	handler.Handle("/rest/2.0/responsibilities", testutil.JsonHandlerOut(func(*http.Request) (int, clients.ResponsibilityPagedResponse) {
		return http.StatusOK, clients.ResponsibilityPagedResponse{Limit: 100}
	}))
	handler.HandleFunc("POST /rest/contextEngine/v1/contexts/generate", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("name: My Asset\ntype: Table\n"))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server), true).Handler(t.Context(), tools.Input{
		AssetID:                assetId.String(),
		ContextSpecificationId: specId.String(),
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if !output.Found {
		t.Fatalf("Asset not found")
	}
	if output.AssetContextError != "" {
		t.Fatalf("Unexpected context error: %q", output.AssetContextError)
	}
	if output.AssetContext != "name: My Asset\ntype: Table\n" {
		t.Fatalf("Expected YAML context, got: %q", output.AssetContext)
	}
}

func TestGetAssetDetailsContextSpecFeatureDisabled(t *testing.T) {
	assetId, _ := uuid.NewUUID()
	specId, _ := uuid.NewUUID()
	handler := http.NewServeMux()
	handler.Handle("/graphql/knowledgeGraph/v1", testutil.JsonHandlerInOut(func(_ *http.Request, _ clients.Request) (int, clients.Response) {
		return http.StatusOK, clients.Response{
			Data: &clients.AssetQueryData{
				Assets: []clients.Asset{{ID: assetId.String(), DisplayName: "My Asset"}},
			},
		}
	}))
	handler.Handle("/rest/2.0/responsibilities", testutil.JsonHandlerOut(func(*http.Request) (int, clients.ResponsibilityPagedResponse) {
		return http.StatusOK, clients.ResponsibilityPagedResponse{Limit: 100}
	}))
	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server), false).Handler(t.Context(), tools.Input{
		AssetID:                assetId.String(),
		ContextSpecificationId: specId.String(),
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if !output.Found {
		t.Fatalf("Asset not found")
	}
	if output.AssetContext != "" {
		t.Fatalf("Expected no context when feature disabled, got: %q", output.AssetContext)
	}
	if output.AssetContextError == "" {
		t.Fatalf("Expected AssetContextError when feature is disabled")
	}
}

// TestGetAssetDetailsSurfacesEmptyAssignableAttribute: an Acronym with an empty
// required Definition (only Note has a value) must still surface Definition in
// assignableAttributes as isSet=false, required=true.
func TestGetAssetDetailsSurfacesEmptyAssignableAttribute(t *testing.T) {
	assetId, _ := uuid.NewUUID()
	handler := http.NewServeMux()
	handler.Handle("/graphql/knowledgeGraph/v1", testutil.JsonHandlerInOut(func(_ *http.Request, _ clients.Request) (int, clients.Response) {
		return http.StatusOK, clients.Response{
			Data: &clients.AssetQueryData{
				Assets: []clients.Asset{{
					ID:          assetId.String(),
					DisplayName: "Acronymn_194_reach4",
					StringAttributes: []clients.StringAttribute{
						{Value: "a note", Type: &clients.AttributeType{Name: "Note"}},
					},
				}},
			},
		}
	}))
	handler.Handle("/rest/2.0/responsibilities", testutil.JsonHandlerOut(func(*http.Request) (int, clients.ResponsibilityPagedResponse) {
		return http.StatusOK, clients.ResponsibilityPagedResponse{Limit: 100}
	}))
	handler.HandleFunc("GET /rest/2.0/assignments/asset/"+assetId.String(), func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "assignment-effective",
			"assetType": {"id": "acr", "name": "Acronym"},
			"assignedCharacteristicTypeReferences": [
				{"id": "c1", "minimumOccurrences": 1, "assignedResourceReference": {"id": "def", "name": "Definition", "resourceDiscriminator": "StringAttributeType"}},
				{"id": "c2", "minimumOccurrences": 0, "assignedResourceReference": {"id": "note", "name": "Note", "resourceDiscriminator": "StringAttributeType"}}
			]
		}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server), false).Handler(t.Context(), tools.Input{AssetID: assetId.String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	byName := map[string]tools.AssignableAttribute{}
	for _, a := range output.AssignableAttributes {
		byName[a.Name] = a
	}
	def, ok := byName["Definition"]
	if !ok {
		t.Fatalf("Definition missing from assignableAttributes: %+v", output.AssignableAttributes)
	}
	if def.IsSet {
		t.Errorf("Definition should be reported as empty (isSet=false)")
	}
	if !def.Required {
		t.Errorf("Definition should be reported as required")
	}
	note, ok := byName["Note"]
	if !ok {
		t.Fatalf("Note missing from assignableAttributes")
	}
	if !note.IsSet {
		t.Errorf("Note has a value and should be isSet=true")
	}
}

// DGC returns a Multiple Selection attribute's values as one ';'-joined string.
func TestGetAssetDetailsReturnsMultipleSelectionValues(t *testing.T) {
	assetId, _ := uuid.NewUUID()
	handler := http.NewServeMux()
	handler.HandleFunc("/graphql/knowledgeGraph/v1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data": {"assets": [{
			"id": "` + assetId.String() + `",
			"displayName": "Customer Table",
			"multiValueAttributes": [
				{"stringValues": "PII;Financial", "type": {"name": "Confidentiality"}}
			]
		}]}}`))
	})
	handler.Handle("/rest/2.0/responsibilities", testutil.JsonHandlerOut(func(*http.Request) (int, clients.ResponsibilityPagedResponse) {
		return http.StatusOK, clients.ResponsibilityPagedResponse{Limit: 100}
	}))
	handler.HandleFunc("GET /rest/2.0/assignments/asset/"+assetId.String(), func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "assignment-effective",
			"assetType": {"id": "tbl", "name": "Table"},
			"assignedCharacteristicTypeReferences": [
				{"id": "c1", "minimumOccurrences": 0, "assignedResourceReference": {"id": "conf", "name": "Confidentiality", "resourceDiscriminator": "MultiValueListAttributeType"}}
			]
		}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server), false).Handler(t.Context(), tools.Input{AssetID: assetId.String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.Asset == nil || len(output.Asset.MultiValueAttributes) != 1 {
		t.Fatalf("expected one multi-value attribute, got: %+v", output.Asset)
	}
	attr := output.Asset.MultiValueAttributes[0]
	if attr.Type == nil || attr.Type.Name != "Confidentiality" {
		t.Errorf("expected attribute type Confidentiality, got: %+v", attr.Type)
	}
	if !slices.Equal(attr.Values, []string{"PII", "Financial"}) {
		t.Errorf("expected values [PII Financial], got: %q", attr.Values)
	}

	var entries []tools.AssignableAttribute
	for _, a := range output.AssignableAttributes {
		if a.Name == "Confidentiality" {
			entries = append(entries, a)
		}
	}
	if len(entries) == 0 {
		t.Fatalf("Confidentiality missing from assignableAttributes: %+v", output.AssignableAttributes)
	}
	for _, a := range entries {
		if !a.IsSet {
			t.Errorf("Confidentiality has values and should be isSet=true, got: %+v", a)
		}
	}
}

func TestGetAssetDetailsReturnsSingleValuedAttributesAsSet(t *testing.T) {
	assetId, _ := uuid.NewUUID()
	handler := http.NewServeMux()
	handler.HandleFunc("/graphql/knowledgeGraph/v1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data": {"assets": [{
			"id": "` + assetId.String() + `",
			"displayName": "Customer Table",
			"stringAttributes": [
				{"stringValue": "Gold", "type": {"name": "Tier"}},
				{"stringValue": "Customer master data", "type": {"name": "Description"}}
			],
			"numericAttributes": [{"numericValue": 42.5, "type": {"name": "Row Count"}}],
			"booleanAttributes": [{"booleanValue": true, "type": {"name": "Personal Data"}}],
			"dateAttributes": [{"dateValue": "1700000000000", "type": {"name": "Review Date"}}]
		}]}}`))
	})
	handler.Handle("/rest/2.0/responsibilities", testutil.JsonHandlerOut(func(*http.Request) (int, clients.ResponsibilityPagedResponse) {
		return http.StatusOK, clients.ResponsibilityPagedResponse{Limit: 100}
	}))
	handler.HandleFunc("GET /rest/2.0/assignments/asset/"+assetId.String(), func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "assignment-effective",
			"assetType": {"id": "tbl", "name": "Table"},
			"assignedCharacteristicTypeReferences": [
				{"id": "c1", "minimumOccurrences": 0, "assignedResourceReference": {"id": "tier", "name": "Tier", "resourceDiscriminator": "SingleValueListAttributeType"}},
				{"id": "c2", "minimumOccurrences": 0, "assignedResourceReference": {"id": "desc", "name": "Description", "resourceDiscriminator": "StringAttributeType"}},
				{"id": "c3", "minimumOccurrences": 0, "assignedResourceReference": {"id": "rows", "name": "Row Count", "resourceDiscriminator": "NumericAttributeType"}},
				{"id": "c4", "minimumOccurrences": 0, "assignedResourceReference": {"id": "pd", "name": "Personal Data", "resourceDiscriminator": "BooleanAttributeType"}},
				{"id": "c5", "minimumOccurrences": 0, "assignedResourceReference": {"id": "rev", "name": "Review Date", "resourceDiscriminator": "DateAttributeType"}}
			]
		}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	output, err := tools.NewTool(testutil.NewClient(server), false).Handler(t.Context(), tools.Input{AssetID: assetId.String()})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	asset := output.Asset
	if asset == nil {
		t.Fatalf("expected an asset, got none: %+v", output)
	}
	if len(asset.StringAttributes) != 2 || asset.StringAttributes[0].Value != "Gold" || asset.StringAttributes[1].Value != "Customer master data" {
		t.Errorf("unexpected string attributes: %+v", asset.StringAttributes)
	}
	if len(asset.NumericAttributes) != 1 || asset.NumericAttributes[0].Value != 42.5 {
		t.Errorf("unexpected numeric attributes: %+v", asset.NumericAttributes)
	}
	if len(asset.BooleanAttributes) != 1 || !asset.BooleanAttributes[0].Value {
		t.Errorf("unexpected boolean attributes: %+v", asset.BooleanAttributes)
	}
	if len(asset.DateAttributes) != 1 || asset.DateAttributes[0].Value != "1700000000000" {
		t.Errorf("unexpected date attributes: %+v", asset.DateAttributes)
	}

	if len(output.AssignableAttributes) != 5 {
		t.Fatalf("expected 5 assignable attributes, got: %+v", output.AssignableAttributes)
	}
	for _, a := range output.AssignableAttributes {
		if !a.IsSet {
			t.Errorf("%s has a value and should be isSet=true", a.Name)
		}
	}
}
