package list_agent_contracts_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/tools/list_agent_contracts"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const agentID = "d654ca91-10d5-34bf-8819-78800b81fe23"

func serve(t *testing.T, status int, body string, query *string) *http.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/aiGovernance/v1/agentContracts" {
			http.NotFound(w, r)
			return
		}
		*query = r.URL.RawQuery
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

func TestList_PassesFiltersAndMapsResults(t *testing.T) {
	var query string
	client := serve(t, 200, `{"total":1,"results":[{"id":"c1","name":"Global","layer":"GLOBAL","activeVersionId":"v1","activeVersionNumber":3,"ruleCount":5}]}`, &query)
	out, err := list_agent_contracts.NewTool(client).Handler(t.Context(), list_agent_contracts.Input{Name: " hr ", SortField: "rule_count", SortOrder: "desc", Limit: 10, IncludeTotal: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != list_agent_contracts.StatusSuccess || len(out.Contracts) != 1 || out.Contracts[0].Layer != "GLOBAL" || out.Contracts[0].RuleCount != 5 || out.Total == nil || *out.Total != 1 {
		t.Fatalf("unexpected output: %+v", out)
	}
	for _, want := range []string{"name=hr", "sortField=RULE_COUNT", "sortOrder=DESC", "limit=10", "includeTotal=true"} {
		if !contains(query, want) {
			t.Errorf("query %q lacks %q", query, want)
		}
	}
}

func TestList_AgentWithNoContractIsAnAnswer(t *testing.T) {
	var query string
	client := serve(t, 200, `{"agentId":"`+agentID+`","results":[]}`, &query)
	out, _ := list_agent_contracts.NewTool(client).Handler(t.Context(), list_agent_contracts.Input{AgentID: agentID})
	if out.Status != list_agent_contracts.StatusSuccess || len(out.Contracts) != 0 || out.Message != "No agent contract governs this AI agent." || !contains(query, "agentId="+agentID) {
		t.Errorf("unexpected: %+v query=%q", out, query)
	}
}

func TestList_ValidationAndErrors(t *testing.T) {
	var query string
	client := serve(t, 403, `{"errorCode":"agentContractNotViewable","statusCode":403}`, &query)
	for name, in := range map[string]list_agent_contracts.Input{
		"agent id": {AgentID: "x"}, "sort field": {SortField: "COLOR"}, "sort order": {SortOrder: "UP"}, "limit": {Limit: 501},
	} {
		if out, _ := list_agent_contracts.NewTool(client).Handler(t.Context(), in); out.Status != list_agent_contracts.StatusValidationError {
			t.Errorf("%s: %+v", name, out)
		}
	}
	if query != "" {
		t.Errorf("validation errors must not call Collibra, got query %q", query)
	}
	out, _ := list_agent_contracts.NewTool(client).Handler(t.Context(), list_agent_contracts.Input{AgentID: agentID})
	if out.Status != list_agent_contracts.StatusError || out.HTTPStatus != 403 || out.ErrorCode != "agentContractNotViewable" {
		t.Errorf("unexpected: %+v", out)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0) }

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
