package get_assessment_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	tool "github.com/collibra/chip/pkg/tools/get_assessment"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const limitError = "limit must be between 1 and 50; got "

// listStub serves the list endpoint and records the limit query parameter of
// each request, so a test can tell whether the API was called and with what.
type listStub struct {
	calls    int
	limits   []string
	hasLimit []bool
}

func (s *listStub) client(t *testing.T) *http.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/assessments/v2/assessments", func(w http.ResponseWriter, r *http.Request) {
		s.calls++
		s.limits = append(s.limits, r.URL.Query().Get("limit"))
		s.hasLimit = append(s.hasLimit, r.URL.Query().Has("limit"))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"id": "assessment-1", "name": "First"},
				{"id": "assessment-2", "name": "Second"},
			},
		})
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return testutil.NewClient(server)
}

func TestGetAssessment_LimitOutOfRangeIsRejectedBeforeAnyCall(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit int
		want  string
	}{
		{"zero", 0, limitError + "0"},
		{"negative", -1, limitError + "-1"},
		{"above max", 51, limitError + "51"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &listStub{}
			_, err := tool.NewTool(s.client(t)).Handler(t.Context(), tool.Input{Limit: chip.Ptr(tc.limit)})
			if err == nil || err.Error() != tc.want {
				t.Fatalf("want error %q, got %v", tc.want, err)
			}
			if s.calls != 0 {
				t.Fatalf("want no API call for an invalid limit, got %d", s.calls)
			}
		})
	}
}

func TestGetAssessment_LimitInRangeIsSentToTheAPI(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit int
		want  string
	}{
		{"min", 1, "1"},
		{"max", 50, "50"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &listStub{}
			out, err := tool.NewTool(s.client(t)).Handler(t.Context(), tool.Input{Limit: chip.Ptr(tc.limit)})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !out.Found || len(out.Assessments) != 2 {
				t.Fatalf("want the listed assessments, got %+v", out)
			}
			if s.calls != 1 || s.limits[0] != tc.want {
				t.Fatalf("want one call with limit=%s, got calls=%d limits=%v", tc.want, s.calls, s.limits)
			}
		})
	}
}

func TestGetAssessment_NoLimitLeavesTheAPIDefault(t *testing.T) {
	s := &listStub{}
	out, err := tool.NewTool(s.client(t)).Handler(t.Context(), tool.Input{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Found || len(out.Assessments) != 2 {
		t.Fatalf("want the listed assessments, got %+v", out)
	}
	if s.calls != 1 || s.hasLimit[0] {
		t.Fatalf("want one call without a limit parameter, got calls=%d limits=%v", s.calls, s.limits)
	}
}

// The MCP server decodes tool arguments from JSON, so an explicit "limit": 0
// must decode to a set limit, not to "not set".
func TestGetAssessment_ExplicitZeroLimitInArgumentsIsRejected(t *testing.T) {
	var input tool.Input
	if err := json.Unmarshal([]byte(`{"limit": 0}`), &input); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	s := &listStub{}
	_, err := tool.NewTool(s.client(t)).Handler(t.Context(), input)
	if err == nil || err.Error() != limitError+"0" {
		t.Fatalf("want error %q, got %v", limitError+"0", err)
	}
	if s.calls != 0 {
		t.Fatalf("want no API call, got %d", s.calls)
	}
}
