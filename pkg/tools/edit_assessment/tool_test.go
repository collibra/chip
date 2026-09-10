package edit_assessment_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tool "github.com/collibra/chip/pkg/tools/edit_assessment"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const assessmentID = "bbbbbbbb-0000-0000-0000-000000000001"

// stub serves the endpoints edit_assessment touches: the user directory (name
// search + exact email) and the assessment GET/PATCH pair, recording the PATCH
// body so a test can see exactly what would be written.
type stub struct {
	users    []clients.EditAssetUser
	patched  *clients.UpdateAssessmentRequest
	nameHits int
}

func (s *stub) client(t *testing.T) *http.Client {
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

	mux.HandleFunc("GET /rest/assessments/v2/assessments/"+assessmentID, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"id": assessmentID, "name": "Customer Data", "status": "DRAFT"})
	})

	mux.HandleFunc("PATCH /rest/assessments/v2/assessments/"+assessmentID, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req clients.UpdateAssessmentRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("decoding patch body: %v", err)
		}
		s.patched = &req
		_ = json.NewEncoder(w).Encode(map[string]any{"id": assessmentID, "name": "Customer Data", "status": "DRAFT"})
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

// set_owner and set_assignees accept every form a person can be named in.
func TestEditAssessment_OwnerAndAssigneesAcceptEveryUserForm(t *testing.T) {
	cases := []struct {
		name  string
		given string
		want  string
	}{
		{"uuid", "11111111-2222-3333-4444-555555555555", "11111111-2222-3333-4444-555555555555"},
		{"email", "jane.smith@example.com", "u-jane"},
		{"username", "jane.smith", "u-jane"},
		{"full name", "Jane Smith", "u-jane"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &stub{users: directory()}
			out, err := tool.NewTool(s.client(t)).Handler(t.Context(), tool.Input{
				Assessment: assessmentID,
				Operations: []tool.Operation{
					{Type: tool.OpSetOwner, UserID: tc.given},
					{Type: tool.OpSetAssignees, Assignees: []tool.AssigneeInput{{ID: tc.given, Type: "USER"}}},
				},
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if out.Status != tool.StatusSuccess {
				t.Fatalf("expected success, got %q: %+v", out.Status, out.Results)
			}
			if s.patched == nil || s.patched.Owner == nil || s.patched.Owner.ID != tc.want {
				t.Fatalf("owner = %+v, want id %q", s.patched.Owner, tc.want)
			}
			if len(s.patched.Assignees) != 1 || s.patched.Assignees[0].ID != tc.want {
				t.Fatalf("assignees = %+v, want [{%s USER}]", s.patched.Assignees, tc.want)
			}
		})
	}
}

// A UUID owner costs no user lookup.
func TestEditAssessment_UUIDOwnerIssuesNoUserLookup(t *testing.T) {
	s := &stub{users: directory()}
	if _, err := tool.NewTool(s.client(t)).Handler(t.Context(), tool.Input{
		Assessment: assessmentID,
		Operations: []tool.Operation{{Type: tool.OpSetOwner, UserID: "11111111-2222-3333-4444-555555555555"}},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.nameHits != 0 {
		t.Fatalf("expected no user name search, got %d", s.nameHits)
	}
}

// A shared name fails the operation, and the PATCH being atomic, nothing at
// all is written.
func TestEditAssessment_AmbiguousOwnerPatchesNothing(t *testing.T) {
	s := &stub{users: directory()}
	out, err := tool.NewTool(s.client(t)).Handler(t.Context(), tool.Input{
		Assessment: assessmentID,
		Operations: []tool.Operation{
			{Type: tool.OpSetName, Value: "Renamed"},
			{Type: tool.OpSetOwner, UserID: "Bob Jones"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tool.StatusError {
		t.Fatalf("expected an error status, got %q", out.Status)
	}
	for _, want := range []string{"ambiguous", "u-bob", "u-dup", "bjones2"} {
		if !strings.Contains(out.Results[1].Error, want) {
			t.Fatalf("error %q does not mention %q", out.Results[1].Error, want)
		}
	}
	if s.patched != nil {
		t.Fatalf("expected no PATCH, got %+v", s.patched)
	}
}

// A GROUP assignee is not a user, so it still has to be a UUID.
func TestEditAssessment_GroupAssigneeMustBeAUUID(t *testing.T) {
	s := &stub{users: directory()}
	out, err := tool.NewTool(s.client(t)).Handler(t.Context(), tool.Input{
		Assessment: assessmentID,
		Operations: []tool.Operation{{Type: tool.OpSetAssignees, Assignees: []tool.AssigneeInput{{ID: "Data Stewards", Type: "GROUP"}}}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != tool.StatusError || !strings.Contains(out.Results[0].Error, "UUID") {
		t.Fatalf("expected a UUID-required error, got %+v", out.Results)
	}
	if s.patched != nil {
		t.Fatalf("expected no PATCH, got %+v", s.patched)
	}
}
