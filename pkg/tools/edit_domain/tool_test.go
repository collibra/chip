package edit_domain_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tool "github.com/collibra/chip/pkg/tools/edit_domain"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	domainID = "dddddddd-0000-0000-0000-000000000001"
	commAID  = "cccccccc-0000-0000-0000-00000000000a"
	commBID  = "cccccccc-0000-0000-0000-00000000000b"
	roleID   = "bbbbbbbb-0000-0000-0000-000000000001"
	userID   = "aaaaaaaa-0000-0000-0000-000000000001"
)

type stub struct {
	domain  clients.DomainDetails
	patches []map[string]any
	created []clients.EditAssetCreateResponsibilityRequest
	noMove  bool // simulate a PATCH that returns 200 without moving the domain
}

func newStub() *stub {
	return &stub{domain: clients.DomainDetails{ID: domainID, Name: "Wellness", Description: "old", Community: &clients.ContainerRef{ID: commAID, Name: "HR"}}}
}

func (s *stub) client(t *testing.T) *http.Client {
	t.Helper()
	communities := map[string]clients.CommunityDetails{
		commAID: {ID: commAID, Name: "HR"},
		commBID: {ID: commBID, Name: "Employee Wellness"},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/domains/{id}", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(s.domain)
	})
	mux.HandleFunc("GET /rest/2.0/domains", func(w http.ResponseWriter, r *http.Request) {
		var out []clients.DomainDetails
		if strings.Contains(strings.ToLower(s.domain.Name), strings.ToLower(r.URL.Query().Get("name"))) {
			out = append(out, s.domain)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": out})
	})
	mux.HandleFunc("PATCH /rest/2.0/domains/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.patches = append(s.patches, body)
		if v, ok := body["name"].(string); ok {
			s.domain.Name = v
		}
		if v, ok := body["description"].(string); ok {
			s.domain.Description = v
		}
		if v, ok := body["communityId"].(string); ok && !s.noMove {
			c := communities[v]
			s.domain.Community = &clients.ContainerRef{ID: c.ID, Name: c.Name}
		}
		_ = json.NewEncoder(w).Encode(s.domain)
	})
	mux.HandleFunc("GET /rest/2.0/communities/{id}", func(w http.ResponseWriter, r *http.Request) {
		c, ok := communities[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(c)
	})
	mux.HandleFunc("GET /rest/2.0/communities", func(w http.ResponseWriter, r *http.Request) {
		var out []clients.CommunityDetails
		for _, c := range communities {
			if strings.Contains(strings.ToLower(c.Name), strings.ToLower(r.URL.Query().Get("name"))) {
				out = append(out, c)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": out})
	})
	mux.HandleFunc("GET /rest/2.0/roles", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"total": 1, "results": []clients.EditAssetRole{{ID: roleID, Name: "Steward"}}})
	})
	mux.HandleFunc("POST /rest/2.0/responsibilities", func(w http.ResponseWriter, r *http.Request) {
		var req clients.EditAssetCreateResponsibilityRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.created = append(s.created, req)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(clients.EditAssetResponsibility{ID: "new"})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return testutil.NewClient(server)
}

func run(t *testing.T, s *stub, input tool.Input) tool.Output {
	t.Helper()
	out, err := tool.NewTool(s.client(t)).Handler(t.Context(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return out
}

func TestEditDomain_RenameDescribeAndMove(t *testing.T) {
	s := newStub()
	out := run(t, s, tool.Input{Domain: "wellness", Operations: []tool.Operation{
		{Type: tool.OpSetName, Value: "Staff Wellness"},
		{Type: tool.OpSetDescription, Value: ""},
		{Type: tool.OpSetCommunity, Community: "Employee Wellness"},
	}})
	if out.Status != tool.StatusSuccess {
		t.Fatalf("status = %q: %+v", out.Status, out.Results)
	}
	if out.Domain.Name != "Staff Wellness" || out.Domain.Description != "" || out.Domain.CommunityID != commBID {
		t.Fatalf("snapshot = %+v", out.Domain)
	}
	if out.Results[2].PreviousValue != "HR" || out.Results[2].NewValue != "Employee Wellness" {
		t.Fatalf("move result = %+v", out.Results[2])
	}
}

func TestEditDomain_SilentNoOpMoveIsAnError(t *testing.T) {
	s := newStub()
	s.noMove = true
	out := run(t, s, tool.Input{Domain: domainID, Operations: []tool.Operation{{Type: tool.OpSetCommunity, Community: commBID}}})
	if out.Status != tool.StatusError || !strings.Contains(out.Results[0].Error, "not in community") {
		t.Fatalf("status=%q results=%+v", out.Status, out.Results)
	}
}

func TestEditDomain_FailingOperationDoesNotBlockSiblings(t *testing.T) {
	s := newStub()
	out := run(t, s, tool.Input{Domain: domainID, Operations: []tool.Operation{
		{Type: tool.OpSetCommunity},
		{Type: tool.OpSetCommunity, Community: "No Such Community"},
		{Type: tool.OpSetName, Value: "Renamed"},
	}})
	if out.Status != tool.StatusPartialSuccess {
		t.Fatalf("status = %q: %+v", out.Status, out.Results)
	}
	if out.Results[0].Status != "error" || out.Results[1].Status != "error" || out.Results[2].Status != "success" {
		t.Fatalf("results = %+v", out.Results)
	}
	if len(s.patches) != 1 {
		t.Fatalf("only the valid rename should be written, got %v", s.patches)
	}
}

func TestEditDomain_SetResponsibilityUsesDomainResourceType(t *testing.T) {
	s := newStub()
	out := run(t, s, tool.Input{Domain: domainID, Operations: []tool.Operation{{Type: tool.OpSetResponsibility, Role: "Steward", UserID: userID}}})
	if out.Status != tool.StatusSuccess {
		t.Fatalf("results = %+v", out.Results)
	}
	want := clients.EditAssetCreateResponsibilityRequest{RoleID: roleID, OwnerID: userID, ResourceID: domainID, ResourceType: "Domain"}
	if len(s.created) != 1 || s.created[0] != want {
		t.Fatalf("created = %+v, want %+v", s.created, want)
	}
}

func TestEditDomain_MissingDomainIsAnError(t *testing.T) {
	s := newStub()
	_, err := tool.NewTool(s.client(t)).Handler(t.Context(), tool.Input{Domain: "Nope", Operations: []tool.Operation{{Type: tool.OpSetName, Value: "x"}}})
	if err == nil || !strings.Contains(err.Error(), "no domain matching") {
		t.Fatalf("err = %v", err)
	}
}

func TestEditDomain_EmptyOperationsIsAnError(t *testing.T) {
	if _, err := tool.NewTool(newStub().client(t)).Handler(t.Context(), tool.Input{Domain: domainID}); err == nil {
		t.Fatal("expected error")
	}
}
