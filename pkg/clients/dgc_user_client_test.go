package clients_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/uuid"
)

func TestFindUsers(t *testing.T) {
	adminID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/2.0/users", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("name") != "Admin" {
			t.Fatalf("unexpected name query param: %s", r.URL.Query().Get("name"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":1,"offset":0,"limit":20,"results":[{"id":"` + adminID.String() + `","userName":"Admin","emailAddress":"admin@example.com","enabled":true}]}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	response, err := clients.FindUsers(t.Context(), client, clients.FindUsersQueryParams{Name: "Admin", Limit: 20})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if response.Total != 1 || len(response.Results) != 1 {
		t.Fatalf("unexpected response: %+v", response)
	}
	if response.Results[0].ID != adminID.String() {
		t.Fatalf("expected id %s, got %s", adminID.String(), response.Results[0].ID)
	}
}

func TestGetUser(t *testing.T) {
	userID, _ := uuid.NewUUID()

	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/2.0/users/"+userID.String(), func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"` + userID.String() + `","userName":"jdoe","firstName":"Jane","lastName":"Doe","enabled":true}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	user, err := clients.GetUser(t.Context(), client, userID.String())
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if user.UserName != "jdoe" || user.FirstName != "Jane" {
		t.Fatalf("unexpected user: %+v", user)
	}
}
