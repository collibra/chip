package create_domain_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/create_domain"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	communityID  = "00000000-0000-0000-0000-000000000001"
	glossaryType = "00000000-0000-0000-0000-000000000010"
	dictType     = "00000000-0000-0000-0000-000000000011"
)

// fakeDGC serves the lookup endpoints and records every domain write.
type fakeDGC struct {
	posts []map[string]any
}

func (f *fakeDGC) run(t *testing.T) func(in create_domain.Input) create_domain.Output {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/communities", func(w http.ResponseWriter, r *http.Request) {
		results := []map[string]string{}
		if strings.Contains("employee wellness", strings.ToLower(r.URL.Query().Get("name"))) {
			results = append(results, map[string]string{"id": communityID, "name": "Employee Wellness"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"results": results})
	})
	mux.HandleFunc("GET /rest/2.0/domainTypes", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]string{
			{"id": glossaryType, "name": "Glossary"},
			{"id": dictType, "name": "Physical Data Dictionary"},
		}})
	})
	mux.HandleFunc("POST /rest/2.0/domains", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.posts = append(f.posts, body)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "new-id", "name": body["name"],
			"community": map[string]string{"id": communityID, "name": "Employee Wellness"},
			"type":      map[string]string{"id": glossaryType, "name": "Glossary"},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	tool := create_domain.NewTool(testutil.NewClient(server))
	return func(in create_domain.Input) create_domain.Output {
		out, err := tool.Handler(context.Background(), in)
		if err != nil {
			t.Fatalf("handler returned error: %v", err)
		}
		return out
	}
}

func TestPreviewResolvesNamesAndDoesNotWrite(t *testing.T) {
	f := &fakeDGC{}
	out := f.run(t)(create_domain.Input{Name: "Terms", Community: "employee wellness", DomainType: "glossary", Description: "d"})
	if out.Status != create_domain.StatusPreview {
		t.Fatalf("status = %q (%s), want preview", out.Status, out.Message)
	}
	if len(f.posts) != 0 {
		t.Fatalf("preview wrote: %v", f.posts)
	}
	want := create_domain.Preview{Name: "Terms", Description: "d", CommunityID: communityID, CommunityName: "Employee Wellness", DomainTypeID: glossaryType, DomainTypeName: "Glossary"}
	if *out.Preview != want {
		t.Errorf("preview = %+v, want %+v", *out.Preview, want)
	}
}

func TestConfirmCreatesDomainWithResolvedIDs(t *testing.T) {
	f := &fakeDGC{}
	out := f.run(t)(create_domain.Input{Name: "Terms", Community: "Employee Wellness", DomainType: "Glossary", Description: "d", Confirm: true})
	if out.Status != create_domain.StatusSuccess {
		t.Fatalf("status = %q (%s)", out.Status, out.Message)
	}
	if len(f.posts) != 1 {
		t.Fatalf("writes = %v, want exactly one", f.posts)
	}
	p := f.posts[0]
	if p["communityId"] != communityID || p["typeId"] != glossaryType || p["name"] != "Terms" || p["description"] != "d" {
		t.Errorf("unexpected write: %v", p)
	}
	if out.Domain.ID != "new-id" || out.Domain.DomainType != "Glossary" || out.Domain.CommunityName != "Employee Wellness" {
		t.Errorf("unexpected summary: %+v", out.Domain)
	}
}

func TestUUIDsPassThrough(t *testing.T) {
	f := &fakeDGC{}
	out := f.run(t)(create_domain.Input{Name: "Terms", Community: communityID, DomainType: dictType, Confirm: true})
	if out.Status != create_domain.StatusSuccess || f.posts[0]["typeId"] != dictType || f.posts[0]["communityId"] != communityID {
		t.Fatalf("status = %q (%s), posts = %v", out.Status, out.Message, f.posts)
	}
}

func TestValidationErrorsNeverWrite(t *testing.T) {
	tests := map[string]struct {
		in   create_domain.Input
		want string
	}{
		"missing name":        {create_domain.Input{Community: "c", DomainType: "t"}, "name is required"},
		"name too long":       {create_domain.Input{Name: strings.Repeat("x", 256), Community: "c", DomainType: "t"}, "at most 255"},
		"missing community":   {create_domain.Input{Name: "n", DomainType: "t"}, "community is required"},
		"missing domain type": {create_domain.Input{Name: "n", Community: "c"}, "domainType is required"},
		"unknown community":   {create_domain.Input{Name: "n", Community: "Nope", DomainType: "Glossary", Confirm: true}, "no community matching"},
		"unknown domain type": {create_domain.Input{Name: "n", Community: "Employee Wellness", DomainType: "Nope", Confirm: true}, "Glossary, Physical Data Dictionary"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &fakeDGC{}
			out := f.run(t)(tc.in)
			if out.Status != create_domain.StatusValidationError || !strings.Contains(out.Message, tc.want) {
				t.Errorf("status = %q, message = %q; want validation_error containing %q", out.Status, out.Message, tc.want)
			}
			if len(f.posts) != 0 {
				t.Errorf("wrote despite validation error: %v", f.posts)
			}
		})
	}
}
