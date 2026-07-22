package create_community_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tools "github.com/collibra/chip/pkg/tools/create_community"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestCreateCommunity(t *testing.T) {
	communityID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.Handle("POST /rest/2.0/communities", testutil.JsonHandlerInOut(func(r *http.Request, in clients.CreateCommunityRequest) (int, clients.CommunityDetails) {
		if in.Name != "Local Development" {
			t.Fatalf("unexpected name: %s", in.Name)
		}
		return http.StatusCreated, clients.CommunityDetails{ID: communityID.String(), Name: in.Name, Description: in.Description}
	}))

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Name:        "Local Development",
		Description: "Local e2e test environment",
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
	if output.Community.ID != communityID.String() {
		t.Fatalf("expected id %s, got %s", communityID.String(), output.Community.ID)
	}
}

func TestCreateCommunity_Failure(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("POST /rest/2.0/communities", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: "bad"})
	if err != nil {
		t.Fatalf("expected no error (failure reported via Output), got: %v", err)
	}
	if output.Success {
		t.Fatalf("expected failure")
	}
}
