package tools_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools"
	"github.com/google/uuid"
)

func TestGetAssetDetails(t *testing.T) {
	assetId, _ := uuid.NewUUID()
	handler := http.NewServeMux()
	handler.Handle("/graphql/knowledgeGraph/v1", JsonHandlerInOut(func(httpRequest *http.Request, request clients.Request) (int, clients.Response) {
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
	handler.Handle("/rest/2.0/responsibilities", JsonHandlerOut(func(r *http.Request) (int, clients.ResponsibilityPagedResponse) {
		return http.StatusOK, clients.ResponsibilityPagedResponse{
			Total:  0,
			Offset: 0,
			Limit:  100,
		}
	}))
	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)

	output, err := tools.NewAssetDetailsTool(client).Handler(t.Context(), tools.AssetDetailsInput{
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

func TestGetAssetDetailsWithResponsibilities(t *testing.T) {
	assetId, _ := uuid.NewUUID()
	domainId := "domain-123"
	handler := http.NewServeMux()
	handler.Handle("/graphql/knowledgeGraph/v1", JsonHandlerInOut(func(httpRequest *http.Request, request clients.Request) (int, clients.Response) {
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
	handler.Handle("/rest/2.0/responsibilities", JsonHandlerOut(func(r *http.Request) (int, clients.ResponsibilityPagedResponse) {
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
	handler.Handle("/rest/2.0/users/user-1", JsonHandlerOut(func(r *http.Request) (int, clients.UserResponse) {
		return http.StatusOK, clients.UserResponse{
			ID:        "user-1",
			UserName:  "john.doe",
			FirstName: "John",
			LastName:  "Doe",
		}
	}))
	handler.Handle("/rest/2.0/users/user-2", JsonHandlerOut(func(r *http.Request) (int, clients.UserResponse) {
		return http.StatusOK, clients.UserResponse{
			ID:        "user-2",
			UserName:  "jane.smith",
			FirstName: "Jane",
			LastName:  "Smith",
		}
	}))
	handler.Handle("/rest/2.0/userGroups/group-1", JsonHandlerOut(func(r *http.Request) (int, clients.UserGroupResponse) {
		return http.StatusOK, clients.UserGroupResponse{
			ID:   "group-1",
			Name: "Data Governance Team",
		}
	}))
	server := httptest.NewServer(handler)
	defer server.Close()

	client := newClient(server)

	output, err := tools.NewAssetDetailsTool(client).Handler(t.Context(), tools.AssetDetailsInput{
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
