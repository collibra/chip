package create_community_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/create_community"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	financeID   = "00000000-0000-0000-0000-000000000001"
	payrollID   = "00000000-0000-0000-0000-000000000002"
	missingUUID = "00000000-0000-0000-0000-0000000000ff"
)

// fakeDGC serves the community endpoints and records every write.
type fakeDGC struct {
	communities []map[string]string
	createCode  int
	posts       []map[string]any
}

func (f *fakeDGC) run(t *testing.T) func(ctx context.Context, in create_community.Input) create_community.Output {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/communities", func(w http.ResponseWriter, r *http.Request) {
		name := strings.ToLower(r.URL.Query().Get("name"))
		var matches []map[string]string
		for _, c := range f.communities {
			if strings.Contains(strings.ToLower(c["name"]), name) {
				matches = append(matches, c)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": matches, "total": len(matches)})
	})
	mux.HandleFunc("POST /rest/2.0/communities", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.posts = append(f.posts, body)
		if f.createCode != 0 {
			w.WriteHeader(f.createCode)
			_, _ = w.Write([]byte(`{"statusCode":409,"errorCode":"communityNameExists","userMessage":"A community with this name already exists."}`))
			return
		}
		resp := map[string]any{"id": "new-id", "name": body["name"], "description": body["description"]}
		if pid, ok := body["parentId"].(string); ok {
			resp["parent"] = map[string]string{"id": pid, "name": "Finance"}
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	tool := create_community.NewTool(testutil.NewClient(server))
	return func(ctx context.Context, in create_community.Input) create_community.Output {
		out, err := tool.Handler(ctx, in)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		return out
	}
}

func newFake() *fakeDGC {
	return &fakeDGC{communities: []map[string]string{
		{"id": financeID, "name": "Finance"},
		{"id": payrollID, "name": "Finance Payroll"},
	}}
}

func TestPreviewDoesNotWrite(t *testing.T) {
	f := newFake()
	out := f.run(t)(context.Background(), create_community.Input{Name: " Wellness ", ParentCommunity: "finance", Description: "d"})
	if out.Status != create_community.StatusPreview {
		t.Fatalf("status = %q (%s), want preview", out.Status, out.Message)
	}
	if len(f.posts) != 0 {
		t.Fatalf("preview wrote: %v", f.posts)
	}
	want := create_community.Preview{Name: "Wellness", Description: "d", ParentCommunityID: financeID, ParentCommunityName: "Finance"}
	if *out.Preview != want {
		t.Errorf("preview = %+v, want %+v", *out.Preview, want)
	}
}

func TestConfirmCreatesNestedCommunity(t *testing.T) {
	f := newFake()
	out := f.run(t)(context.Background(), create_community.Input{Name: "Wellness", ParentCommunity: "Finance", Description: "d", Confirm: true})
	if out.Status != create_community.StatusSuccess {
		t.Fatalf("status = %q (%s)", out.Status, out.Message)
	}
	if len(f.posts) != 1 || f.posts[0]["parentId"] != financeID || f.posts[0]["name"] != "Wellness" || f.posts[0]["description"] != "d" {
		t.Errorf("unexpected write: %v", f.posts)
	}
	if out.Community.ID != "new-id" || out.Community.ParentCommunityID != financeID {
		t.Errorf("unexpected summary: %+v", out.Community)
	}
}

func TestConfirmCreatesTopLevelCommunity(t *testing.T) {
	f := newFake()
	out := f.run(t)(context.Background(), create_community.Input{Name: "Wellness", Confirm: true})
	if out.Status != create_community.StatusSuccess {
		t.Fatalf("status = %q (%s)", out.Status, out.Message)
	}
	if _, has := f.posts[0]["parentId"]; has {
		t.Errorf("top-level community sent a parentId: %v", f.posts[0])
	}
}

func TestParentUUIDPassesThroughWithoutLookup(t *testing.T) {
	f := newFake()
	out := f.run(t)(context.Background(), create_community.Input{Name: "Wellness", ParentCommunity: missingUUID, Confirm: true})
	if out.Status != create_community.StatusSuccess || f.posts[0]["parentId"] != missingUUID {
		t.Fatalf("status = %q (%s), posts = %v", out.Status, out.Message, f.posts)
	}
}

func TestValidationErrorsNeverWrite(t *testing.T) {
	tests := map[string]struct {
		in   create_community.Input
		want string
	}{
		"missing name":     {create_community.Input{Name: "  ", Confirm: true}, "name is required"},
		"name too long":    {create_community.Input{Name: strings.Repeat("x", 256), Confirm: true}, "at most 255"},
		"unknown parent":   {create_community.Input{Name: "W", ParentCommunity: "Nope", Confirm: true}, "no community matching"},
		"ambiguous parent": {create_community.Input{Name: "W", ParentCommunity: "Dup", Confirm: true}, "ambiguous"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := newFake()
			f.communities = append(f.communities,
				map[string]string{"id": "a", "name": "Dup"}, map[string]string{"id": "b", "name": "dup"})
			out := f.run(t)(context.Background(), tc.in)
			if out.Status != create_community.StatusValidationError || !strings.Contains(out.Message, tc.want) {
				t.Errorf("status = %q, message = %q; want validation_error containing %q", out.Status, out.Message, tc.want)
			}
			if len(f.posts) != 0 {
				t.Errorf("wrote despite validation error: %v", f.posts)
			}
		})
	}
}

func TestDownstreamErrorSurfacesCollibraMessage(t *testing.T) {
	f := newFake()
	f.createCode = http.StatusConflict
	out := f.run(t)(context.Background(), create_community.Input{Name: "Wellness", Confirm: true})
	if out.Status != create_community.StatusError || !strings.Contains(out.Message, "already exists") {
		t.Errorf("status = %q, message = %q", out.Status, out.Message)
	}
}
