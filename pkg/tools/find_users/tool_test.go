package find_users_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/find_users"
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
		if r.URL.Query().Get("limit") != "20" {
			t.Fatalf("expected default limit 20, got: %s", r.URL.Query().Get("limit"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":1,"offset":0,"limit":20,"results":[{"id":"` + adminID.String() + `","userName":"Admin","emailAddress":"admin@example.com","enabled":true}]}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Name: "Admin"})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output.Total != 1 || len(output.Users) != 1 {
		t.Fatalf("unexpected output: %+v", output)
	}
	if output.Users[0].ID != adminID.String() {
		t.Fatalf("expected id %s, got %s", adminID.String(), output.Users[0].ID)
	}
}

func TestFindUsers_CustomLimit(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("GET /rest/2.0/users", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("limit") != "5" {
			t.Fatalf("expected limit 5, got: %s", r.URL.Query().Get("limit"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total":0,"offset":0,"limit":5,"results":[]}`))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Limit: 5})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(output.Users) != 0 {
		t.Fatalf("expected no users, got: %+v", output.Users)
	}
}
