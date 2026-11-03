package edit_community_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	tool "github.com/collibra/chip/pkg/tools/edit_community"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	peopleID  = "cccccccc-0000-0000-0000-000000000001"
	aiID      = "cccccccc-0000-0000-0000-000000000002"
	roleID    = "bbbbbbbb-0000-0000-0000-000000000001"
	userID    = "aaaaaaaa-0000-0000-0000-000000000001"
	respID    = "eeeeeeee-0000-0000-0000-000000000001"
	inheritID = "eeeeeeee-0000-0000-0000-000000000002"
)

type stub struct {
	communities     map[string]*clients.CommunityDetails
	patches         []map[string]any
	ignoreClear     bool // simulate the platform silently ignoring parentId:null
	created         []clients.EditAssetCreateResponsibilityRequest
	deleted         []string
	responsibilites []clients.Responsibility
}

func newStub() *stub {
	return &stub{communities: map[string]*clients.CommunityDetails{
		peopleID: {ID: peopleID, Name: "People", Description: "old", Parent: &clients.ContainerRef{ID: aiID, Name: "AI Community"}},
		aiID:     {ID: aiID, Name: "AI Community"},
	}}
}

func (s *stub) client(t *testing.T) *http.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/communities/{id}", func(w http.ResponseWriter, r *http.Request) {
		c, ok := s.communities[r.PathValue("id")]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(c)
	})
	mux.HandleFunc("GET /rest/2.0/communities", func(w http.ResponseWriter, r *http.Request) {
		needle := strings.ToLower(r.URL.Query().Get("name"))
		var out []*clients.CommunityDetails
		for _, c := range s.communities {
			if strings.Contains(strings.ToLower(c.Name), needle) {
				out = append(out, c)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": out})
	})
	mux.HandleFunc("PATCH /rest/2.0/communities/{id}", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.patches = append(s.patches, body)
		c := s.communities[r.PathValue("id")]
		if v, ok := body["name"].(string); ok {
			c.Name = v
		}
		if v, ok := body["description"].(string); ok {
			c.Description = v
		}
		if v, ok := body["parentId"]; ok {
			switch p := v.(type) {
			case string:
				c.Parent = &clients.ContainerRef{ID: p, Name: s.communities[p].Name}
			case nil:
				if !s.ignoreClear {
					c.Parent = nil
				}
			}
		}
		_ = json.NewEncoder(w).Encode(c)
	})
	mux.HandleFunc("GET /rest/2.0/roles", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"total": 1, "results": []clients.EditAssetRole{{ID: roleID, Name: "Steward"}}})
	})
	mux.HandleFunc("GET /rest/2.0/responsibilities", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(clients.ResponsibilityPagedResponse{Results: s.responsibilites})
	})
	mux.HandleFunc("POST /rest/2.0/responsibilities", func(w http.ResponseWriter, r *http.Request) {
		var req clients.EditAssetCreateResponsibilityRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.created = append(s.created, req)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(clients.EditAssetResponsibility{ID: respID})
	})
	mux.HandleFunc("DELETE /rest/2.0/responsibilities/{id}", func(w http.ResponseWriter, r *http.Request) {
		s.deleted = append(s.deleted, r.PathValue("id"))
		w.WriteHeader(http.StatusNoContent)
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

func TestEditCommunity_RenameAndDescribe(t *testing.T) {
	s := newStub()
	out := run(t, s, tool.Input{Community: "people", Operations: []tool.Operation{
		{Type: tool.OpSetName, Value: "Staff"},
		{Type: tool.OpSetDescription, Value: "new"},
	}})
	if out.Status != tool.StatusSuccess {
		t.Fatalf("status = %q: %+v", out.Status, out.Results)
	}
	if out.Community.Name != "Staff" || out.Community.Description != "new" {
		t.Fatalf("snapshot = %+v", out.Community)
	}
	if out.Results[0].PreviousValue != "People" || out.Results[1].PreviousValue != "old" {
		t.Fatalf("previous values = %+v", out.Results)
	}
}

func TestEditCommunity_SetParentByName(t *testing.T) {
	s := newStub()
	s.communities[peopleID].Parent = nil
	out := run(t, s, tool.Input{Community: peopleID, Operations: []tool.Operation{{Type: tool.OpSetParent, Parent: "AI Community"}}})
	if out.Status != tool.StatusSuccess || out.Community.ParentID != aiID {
		t.Fatalf("status=%q snapshot=%+v results=%+v", out.Status, out.Community, out.Results)
	}
}

func TestEditCommunity_MakeTopLevelSendsExplicitNull(t *testing.T) {
	s := newStub()
	out := run(t, s, tool.Input{Community: peopleID, Operations: []tool.Operation{{Type: tool.OpSetParent, MakeTopLevel: true}}})
	if out.Status != tool.StatusSuccess || out.Community.Parent != "" {
		t.Fatalf("status=%q snapshot=%+v results=%+v", out.Status, out.Community, out.Results)
	}
	if v, ok := s.patches[0]["parentId"]; !ok || v != nil {
		t.Fatalf("expected explicit parentId:null, got %v", s.patches[0])
	}
}

// Collibra answers 200 to a clear-parent request without clearing it (DEV-217539).
func TestEditCommunity_MakeTopLevelSilentNoOpIsAnError(t *testing.T) {
	s := newStub()
	s.ignoreClear = true
	out := run(t, s, tool.Input{Community: peopleID, Operations: []tool.Operation{{Type: tool.OpSetParent, MakeTopLevel: true}}})
	if out.Status != tool.StatusError || !strings.Contains(out.Results[0].Error, "still under parent") {
		t.Fatalf("status=%q results=%+v", out.Status, out.Results)
	}
}

func TestEditCommunity_SetParentRequiresExactlyOne(t *testing.T) {
	for name, op := range map[string]tool.Operation{
		"neither": {Type: tool.OpSetParent},
		"both":    {Type: tool.OpSetParent, Parent: "AI Community", MakeTopLevel: true},
	} {
		t.Run(name, func(t *testing.T) {
			s := newStub()
			out := run(t, s, tool.Input{Community: peopleID, Operations: []tool.Operation{op}})
			if out.Status != tool.StatusError || !strings.Contains(out.Results[0].Error, "exactly one") {
				t.Fatalf("status=%q results=%+v", out.Status, out.Results)
			}
			if len(s.patches) != 0 {
				t.Fatalf("nothing should be written, got %v", s.patches)
			}
		})
	}
}

func TestEditCommunity_OwnParentRejected(t *testing.T) {
	out := run(t, newStub(), tool.Input{Community: peopleID, Operations: []tool.Operation{{Type: tool.OpSetParent, Parent: peopleID}}})
	if out.Status != tool.StatusError || !strings.Contains(out.Results[0].Error, "own parent") {
		t.Fatalf("results=%+v", out.Results)
	}
}

func TestEditCommunity_FailingOperationDoesNotBlockSiblings(t *testing.T) {
	s := newStub()
	out := run(t, s, tool.Input{Community: peopleID, Operations: []tool.Operation{
		{Type: tool.OpSetName, Value: "  "},
		{Type: tool.OpSetDescription, Value: "new"},
		{Type: "bogus"},
	}})
	if out.Status != tool.StatusPartialSuccess {
		t.Fatalf("status = %q: %+v", out.Status, out.Results)
	}
	if out.Results[0].Status != "error" || out.Results[1].Status != "success" || out.Results[2].Status != "error" {
		t.Fatalf("results = %+v", out.Results)
	}
}

func TestEditCommunity_AmbiguousNameReturnsCandidates(t *testing.T) {
	s := newStub()
	s.communities["dup"] = &clients.CommunityDetails{ID: "dup", Name: "people"}
	_, err := tool.NewTool(s.client(t)).Handler(t.Context(), tool.Input{Community: "People", Operations: []tool.Operation{{Type: tool.OpSetName, Value: "x"}}})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "dup") {
		t.Fatalf("err = %v", err)
	}
}

func TestEditCommunity_EmptyOperationsIsAnError(t *testing.T) {
	if _, err := tool.NewTool(newStub().client(t)).Handler(t.Context(), tool.Input{Community: peopleID}); err == nil {
		t.Fatal("expected error")
	}
}

func TestEditCommunity_SetResponsibilityUsesCommunityResourceType(t *testing.T) {
	s := newStub()
	out := run(t, s, tool.Input{Community: peopleID, Operations: []tool.Operation{{Type: tool.OpSetResponsibility, Role: "steward", UserID: userID}}})
	if out.Status != tool.StatusSuccess {
		t.Fatalf("results = %+v", out.Results)
	}
	want := clients.EditAssetCreateResponsibilityRequest{RoleID: roleID, OwnerID: userID, ResourceID: peopleID, ResourceType: "Community"}
	if len(s.created) != 1 || s.created[0] != want {
		t.Fatalf("created = %+v, want %+v", s.created, want)
	}
}

func TestEditCommunity_RemoveResponsibility(t *testing.T) {
	s := newStub()
	s.responsibilites = []clients.Responsibility{
		{ID: inheritID, Role: &clients.ResourceRole{ID: roleID}, Owner: &clients.ResourceRef{ID: "other"}, BaseResource: &clients.ResourceRef{ID: aiID}},
		{ID: respID, Role: &clients.ResourceRole{ID: roleID}, Owner: &clients.ResourceRef{ID: userID}, BaseResource: &clients.ResourceRef{ID: peopleID}},
	}
	out := run(t, s, tool.Input{Community: peopleID, Operations: []tool.Operation{
		{Type: tool.OpRemoveResponsibility, Role: "Steward", UserID: userID},
		{Type: tool.OpRemoveResponsibility, Role: "Steward", UserID: "aaaaaaaa-0000-0000-0000-000000000009"},
	}})
	if len(s.deleted) != 1 || s.deleted[0] != respID {
		t.Fatalf("deleted = %v", s.deleted)
	}
	if out.Status != tool.StatusPartialSuccess || !strings.Contains(out.Results[1].Error, "no \"Steward\" responsibility") {
		t.Fatalf("status=%q results=%+v", out.Status, out.Results)
	}
}

func TestEditCommunity_RemoveInheritedResponsibilityExplainsWhy(t *testing.T) {
	s := newStub()
	s.responsibilites = []clients.Responsibility{
		{ID: inheritID, Role: &clients.ResourceRole{ID: roleID}, Owner: &clients.ResourceRef{ID: userID}, BaseResource: &clients.ResourceRef{ID: aiID}},
	}
	out := run(t, s, tool.Input{Community: peopleID, Operations: []tool.Operation{{Type: tool.OpRemoveResponsibility, Role: "Steward", UserID: userID}}})
	if len(s.deleted) != 0 || !strings.Contains(out.Results[0].Error, "inherited") {
		t.Fatalf("deleted=%v results=%+v", s.deleted, out.Results)
	}
}
