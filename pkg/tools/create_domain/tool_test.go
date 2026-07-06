package create_domain_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/create_domain"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestCreateDomain(t *testing.T) {
	communityID, _ := uuid.NewUUID()
	typeID, _ := uuid.NewUUID()
	domainID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/domains", testutil.JsonHandlerInOut(func(r *http.Request, in clients.CreateDomainRequest) (int, clients.DomainDetails) {
		if in.CommunityID != communityID.String() {
			t.Fatalf("unexpected communityId: %s", in.CommunityID)
		}
		return http.StatusCreated, clients.DomainDetails{
			ID:          domainID.String(),
			Name:        in.Name,
			CommunityID: in.CommunityID,
			TypeID:      in.TypeID,
		}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Name:        "postgres-src [Physical Data Dictionary]",
		CommunityID: communityID.String(),
		TypeID:      typeID.String(),
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
	if output.Domain.ID != domainID.String() {
		t.Fatalf("expected id %s, got %s", domainID.String(), output.Domain.ID)
	}
}

func TestCreateDomain_InvalidCommunityID(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))
	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Name:        "bad",
		CommunityID: "not-a-uuid",
		TypeID:      "also-not-a-uuid",
	})
	if err == nil {
		t.Fatalf("expected an error for invalid communityId")
	}
}
