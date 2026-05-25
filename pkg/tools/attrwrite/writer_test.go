package attrwrite_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/attrwrite"
	"github.com/collibra/chip/pkg/tools/testutil"
)

type fakeAttributeType struct {
	id         string
	stringType string
	failStatus int
}

type fakeServer struct {
	types   map[string]fakeAttributeType
	fetches map[string]int
}

func newFakeServer(types ...fakeAttributeType) *fakeServer {
	s := &fakeServer{
		types:   map[string]fakeAttributeType{},
		fetches: map[string]int{},
	}
	for _, t := range types {
		s.types[t.id] = t
	}
	return s
}

func (s *fakeServer) start(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/attributeTypes/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/attributeTypes/")
		s.fetches[id]++
		at, ok := s.types[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if at.failStatus != 0 {
			w.WriteHeader(at.failStatus)
			_, _ = w.Write([]byte(`{"message":"simulated failure"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":                         at.id,
			"name":                       "Test",
			"publicId":                   "Test",
			"attributeTypeDiscriminator": "StringAttributeType",
			"stringType":                 at.stringType,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

const (
	richID  = "00000000-0000-0000-0000-000000000001"
	plainID = "00000000-0000-0000-0000-000000000002"
	failID  = "00000000-0000-0000-0000-000000000003"
)

func TestPrepareValue_RichText_ConvertsMarkdown(t *testing.T) {
	fs := newFakeServer(fakeAttributeType{id: richID, stringType: "RICH_TEXT"})
	srv := fs.start(t)
	w := attrwrite.New(testutil.NewClient(srv))

	got, converted := w.PrepareValue(t.Context(), richID, "StringAttributeType", "**bold**")
	if !converted {
		t.Fatalf("expected converted=true for RICH_TEXT")
	}
	if !strings.Contains(got, "<strong>bold</strong>") {
		t.Fatalf("expected HTML output, got %q", got)
	}
}

func TestPrepareValue_PlainText_PassesThrough(t *testing.T) {
	fs := newFakeServer(fakeAttributeType{id: plainID, stringType: "PLAIN_TEXT"})
	srv := fs.start(t)
	w := attrwrite.New(testutil.NewClient(srv))

	got, converted := w.PrepareValue(t.Context(), plainID, "StringAttributeType", "**bold**")
	if converted {
		t.Fatalf("expected converted=false for PLAIN_TEXT")
	}
	if got != "**bold**" {
		t.Fatalf("expected raw value, got %q", got)
	}
}

func TestPrepareValue_NonStringKind_SkipsFetch(t *testing.T) {
	fs := newFakeServer()
	srv := fs.start(t)
	w := attrwrite.New(testutil.NewClient(srv))

	got, converted := w.PrepareValue(t.Context(), richID, "NumericAttributeType", "42")
	if converted {
		t.Fatalf("expected converted=false for numeric kind")
	}
	if got != "42" {
		t.Fatalf("expected raw value, got %q", got)
	}
	if fs.fetches[richID] != 0 {
		t.Fatalf("expected no fetch for non-string kind, got %d", fs.fetches[richID])
	}
}

func TestPrepareValue_EmptyTypeID_SkipsFetch(t *testing.T) {
	fs := newFakeServer()
	srv := fs.start(t)
	w := attrwrite.New(testutil.NewClient(srv))

	got, converted := w.PrepareValue(t.Context(), "", "StringAttributeType", "x")
	if converted || got != "x" {
		t.Fatalf("expected raw pass-through, got %q converted=%v", got, converted)
	}
}

func TestPrepareValue_FetchFailure_FallsThrough(t *testing.T) {
	fs := newFakeServer(fakeAttributeType{id: failID, failStatus: http.StatusInternalServerError})
	srv := fs.start(t)
	w := attrwrite.New(testutil.NewClient(srv))

	got, converted := w.PrepareValue(t.Context(), failID, "StringAttributeType", "**raw**")
	if converted {
		t.Fatalf("expected converted=false on fetch failure")
	}
	if got != "**raw**" {
		t.Fatalf("expected raw pass-through on fetch failure, got %q", got)
	}
}

func TestPrepareValue_CachesPerInstance(t *testing.T) {
	fs := newFakeServer(fakeAttributeType{id: richID, stringType: "RICH_TEXT"})
	srv := fs.start(t)
	w := attrwrite.New(testutil.NewClient(srv))

	for i := 0; i < 3; i++ {
		_, _ = w.PrepareValue(t.Context(), richID, "StringAttributeType", "x")
	}
	if fs.fetches[richID] != 1 {
		t.Fatalf("expected exactly one fetch across 3 calls, got %d", fs.fetches[richID])
	}
}

func TestPrepareValue_CachesFailure(t *testing.T) {
	fs := newFakeServer(fakeAttributeType{id: failID, failStatus: http.StatusInternalServerError})
	srv := fs.start(t)
	w := attrwrite.New(testutil.NewClient(srv))

	for i := 0; i < 3; i++ {
		_, _ = w.PrepareValue(t.Context(), failID, "StringAttributeType", "x")
	}
	if fs.fetches[failID] != 1 {
		t.Fatalf("expected failing fetch to be cached (one attempt), got %d", fs.fetches[failID])
	}
}
