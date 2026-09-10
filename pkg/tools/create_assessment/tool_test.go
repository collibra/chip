package create_assessment_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tool "github.com/collibra/chip/pkg/tools/create_assessment"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const templateID = "aaaaaaaa-0000-0000-0000-000000000001"

// assessmentsStub serves the endpoints create_assessment touches: the user
// directory (name search + exact email), the template lookup, and the create
// itself, recording the request body that was sent.
type assessmentsStub struct {
	users    []clients.EditAssetUser
	created  *clients.CreateAssessmentRequest
	nameHits int
}

func (s *assessmentsStub) client(t *testing.T) *http.Client {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("GET /rest/2.0/users", func(w http.ResponseWriter, r *http.Request) {
		s.nameHits++
		needle := strings.ToLower(r.URL.Query().Get("name"))
		var matches []clients.EditAssetUser
		for _, u := range s.users {
			first, last := strings.TrimSpace(u.FirstName), strings.TrimSpace(u.LastName)
			for _, field := range []string{u.UserName, first, last,
				strings.TrimSpace(first + " " + last), strings.TrimSpace(last + " " + first)} {
				if field != "" && strings.Contains(strings.ToLower(field), needle) {
					matches = append(matches, u)
					break
				}
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total": len(matches), "results": matches})
	})

	mux.HandleFunc("GET /rest/2.0/users/email/{email}", func(w http.ResponseWriter, r *http.Request) {
		for _, u := range s.users {
			if strings.EqualFold(u.EmailAddress, r.PathValue("email")) {
				_ = json.NewEncoder(w).Encode(u)
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	})

	mux.HandleFunc("GET /rest/assessments/v2/templates", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total":   1,
			"results": []map[string]any{{"id": templateID, "name": "Business Context"}},
		})
	})

	mux.HandleFunc("POST /rest/assessments/v2/assessments", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req clients.CreateAssessmentRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("decoding create body: %v", err)
		}
		s.created = &req
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "assessment-1", "name": "New assessment"})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return testutil.NewClient(server)
}

func directory() []clients.EditAssetUser {
	return []clients.EditAssetUser{
		{ID: "u-jane", UserName: "jane.smith", EmailAddress: "jane.smith@example.com", FirstName: "Jane", LastName: "Smith"},
		{ID: "u-bob", UserName: "bjones", EmailAddress: "bob@example.com", FirstName: "Bob", LastName: "Jones"},
		{ID: "u-dup", UserName: "bjones2", EmailAddress: "bob.jones@example.com", FirstName: "Bob", LastName: "Jones"},
	}
}

// The owner and each assignee accept every form a person can be named in.
func TestCreateAssessment_OwnerAndAssigneesAcceptEveryUserForm(t *testing.T) {
	cases := []struct {
		name  string
		given string
	}{
		{"uuid", "11111111-2222-3333-4444-555555555555"},
		{"email", "jane.smith@example.com"},
		{"username", "jane.smith"},
		{"full name", "Jane Smith"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &assessmentsStub{users: directory()}
			out, err := tool.NewTool(s.client(t)).Handler(t.Context(), tool.Input{
				Template:  "Business Context",
				OwnerID:   tc.given,
				Assignees: []tool.InputAssignee{{ID: tc.given, Type: "USER"}},
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.Error != "" {
				t.Fatalf("unexpected tool error: %s", out.Error)
			}
			want := "u-jane"
			if tc.name == "uuid" {
				want = tc.given // a UUID is used as given
			}
			if s.created == nil || s.created.Owner == nil || s.created.Owner.ID != want {
				t.Fatalf("owner = %+v, want id %q", s.created.Owner, want)
			}
			if len(s.created.Assignees) != 1 || s.created.Assignees[0].ID != want || s.created.Assignees[0].Type != "USER" {
				t.Fatalf("assignees = %+v, want [{%s USER}]", s.created.Assignees, want)
			}
		})
	}
}

// A UUID owner costs no user lookup at all.
func TestCreateAssessment_UUIDOwnerIssuesNoUserLookup(t *testing.T) {
	s := &assessmentsStub{users: directory()}
	if _, err := tool.NewTool(s.client(t)).Handler(t.Context(), tool.Input{
		Template: "Business Context",
		OwnerID:  "11111111-2222-3333-4444-555555555555",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.nameHits != 0 {
		t.Fatalf("expected no user name search, got %d", s.nameHits)
	}
}

// A shared name aborts the call before anything is created.
func TestCreateAssessment_AmbiguousOwnerCreatesNothing(t *testing.T) {
	s := &assessmentsStub{users: directory()}
	_, err := tool.NewTool(s.client(t)).Handler(t.Context(), tool.Input{
		Template: "Business Context",
		OwnerID:  "Bob Jones",
	})
	if err == nil {
		t.Fatal("expected an ambiguity error")
	}
	for _, want := range []string{"ambiguous", "u-bob", "u-dup"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not mention %q", err, want)
		}
	}
	if s.created != nil {
		t.Fatalf("expected no assessment to be created, got %+v", s.created)
	}
}

// A GROUP assignee is not a user, so it still has to be a UUID.
func TestCreateAssessment_GroupAssigneeMustBeAUUID(t *testing.T) {
	s := &assessmentsStub{users: directory()}
	_, err := tool.NewTool(s.client(t)).Handler(t.Context(), tool.Input{
		Template:  "Business Context",
		Assignees: []tool.InputAssignee{{ID: "Data Stewards", Type: "GROUP"}},
	})
	if err == nil {
		t.Fatal("expected a validation error for a named group")
	}
	if !strings.Contains(err.Error(), "UUID") {
		t.Fatalf("error %q does not say a UUID is required", err)
	}
	if s.created != nil {
		t.Fatalf("expected no assessment to be created, got %+v", s.created)
	}
}
