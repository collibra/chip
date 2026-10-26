package update_agent_contract_rules_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/collibra/chip/pkg/tools/update_agent_contract_rules"
)

const contractID = "0f8c3e2a-6b1d-4c9e-9a47-3d2b5e8f1a60"

func rules(versionID string, number int, statements ...string) string {
	var items []string
	for i, s := range statements {
		items = append(items, `{"id":"r`+string(rune('a'+i))+`","statement":"`+s+`","rank":`+string(rune('1'+i))+`}`)
	}
	return `{"agentContract":{"id":"` + contractID + `","layer":"SPECIALIZED"},"agentContractVersion":{"id":"` + versionID + `","versionNumber":` + string(rune('0'+number)) + `},"results":[` + strings.Join(items, ",") + `]}`
}

type fake struct {
	client  *http.Client
	puts    atomic.Int32
	putBody string
}

// newFake serves the current rules on GET; PUT answers putStatus with putBody.
func newFake(t *testing.T, current string, putStatus int, putResponse string) *fake {
	t.Helper()
	f := &fake{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/aiGovernance/v1/agentContracts/{id}/rules", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(current))
	})
	mux.HandleFunc("PUT /rest/aiGovernance/v1/agentContracts/{id}/rules", func(w http.ResponseWriter, r *http.Request) {
		f.puts.Add(1)
		b, _ := io.ReadAll(r.Body)
		f.putBody = string(b)
		w.WriteHeader(putStatus)
		_, _ = w.Write([]byte(putResponse))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.client = testutil.NewClient(srv)
	return f
}

func TestUpdate_PreviewDiffsAgainstActiveVersionAndSendsNothing(t *testing.T) {
	f := newFake(t, rules("v1", 1, "keep me", "drop me"), 200, "")
	out, err := update_agent_contract_rules.NewTool(f.client).Handler(t.Context(),
		update_agent_contract_rules.Input{AgentContractID: contractID, Statements: []string{"keep  me", "brand new"}})
	if err != nil {
		t.Fatal(err)
	}
	p := out.Preview
	if out.Status != update_agent_contract_rules.StatusPreview || p == nil || f.puts.Load() != 0 {
		t.Fatalf("expected preview and no PUT, got %+v", out)
	}
	if p.CurrentVersionNumber != 1 || p.UnchangedCount != 1 || len(p.Added) != 1 || p.Added[0] != "brand new" || len(p.Removed) != 1 || p.Removed[0] != "drop me" || p.NoChange {
		t.Errorf("wrong diff: %+v", p)
	}
}

func TestUpdate_PreviewFlagsIdenticalSet(t *testing.T) {
	f := newFake(t, rules("v1", 1, "a", "b"), 200, "")
	out, _ := update_agent_contract_rules.NewTool(f.client).Handler(t.Context(),
		update_agent_contract_rules.Input{AgentContractID: contractID, Statements: []string{"a", "b"}})
	if out.Preview == nil || !out.Preview.NoChange {
		t.Fatalf("expected noChange, got %+v", out)
	}
	// Same statements in another order is a change.
	out, _ = update_agent_contract_rules.NewTool(f.client).Handler(t.Context(),
		update_agent_contract_rules.Input{AgentContractID: contractID, Statements: []string{"b", "a"}})
	if out.Preview == nil || out.Preview.NoChange {
		t.Fatalf("reordering must not be noChange, got %+v", out)
	}
}

func TestUpdate_ConfirmReportsNewVersion(t *testing.T) {
	f := newFake(t, rules("v1", 1, "a"), 200, rules("v2", 2, "a", "b"))
	out, err := update_agent_contract_rules.NewTool(f.client).Handler(t.Context(),
		update_agent_contract_rules.Input{AgentContractID: contractID, Statements: []string{"a", "b"}, Confirm: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != update_agent_contract_rules.StatusSuccess || out.NewVersion == nil || !*out.NewVersion || out.VersionNumber != 2 || len(out.Rules) != 2 {
		t.Fatalf("unexpected output: %+v", out)
	}
	if !strings.Contains(f.putBody, `"statements":["a","b"]`) {
		t.Errorf("unexpected body: %s", f.putBody)
	}
}

func TestUpdate_ConfirmWithSameRulesCreatesNoVersion(t *testing.T) {
	f := newFake(t, rules("v1", 1, "a"), 200, rules("v1", 1, "a"))
	out, _ := update_agent_contract_rules.NewTool(f.client).Handler(t.Context(),
		update_agent_contract_rules.Input{AgentContractID: contractID, Statements: []string{"a"}, Confirm: true})
	if out.Status != update_agent_contract_rules.StatusSuccess || out.NewVersion == nil || *out.NewVersion {
		t.Fatalf("expected success without a new version, got %+v", out)
	}
}

func TestUpdate_ErrorsAreExplainedAndNotRetried(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"forbidden":  {403, `{"errorCode":"authorizationFailed"}`, "permission"},
		"feature":    {403, `{"errorCode":"agentContractsFeatureDisabled"}`, "AI Command Center"},
		"concurrent": {409, `{"errorCode":"concurrentAgentContractUpdate"}`, "Read the contract's rules again"},
	} {
		f := newFake(t, rules("v1", 1, "a"), tc.status, tc.body)
		out, _ := update_agent_contract_rules.NewTool(f.client).Handler(t.Context(),
			update_agent_contract_rules.Input{AgentContractID: contractID, Statements: []string{"b"}, Confirm: true})
		if out.Status != update_agent_contract_rules.StatusError || out.HTTPStatus != tc.status || !strings.Contains(out.Message+" "+out.Guidance, tc.want) {
			t.Errorf("%s: unexpected output %+v", name, out)
		}
		if f.puts.Load() != 1 {
			t.Errorf("%s: expected one PUT, got %d", name, f.puts.Load())
		}
	}
}

func TestUpdate_ValidationAndNotFound(t *testing.T) {
	f := newFake(t, rules("v1", 1, "a"), 200, "")
	out, _ := update_agent_contract_rules.NewTool(f.client).Handler(t.Context(), update_agent_contract_rules.Input{AgentContractID: "x", Statements: []string{"a"}})
	if out.Status != update_agent_contract_rules.StatusValidationError {
		t.Errorf("bad id: %+v", out)
	}
	out, _ = update_agent_contract_rules.NewTool(f.client).Handler(t.Context(), update_agent_contract_rules.Input{AgentContractID: contractID})
	if out.Status != update_agent_contract_rules.StatusValidationError {
		t.Errorf("no statements: %+v", out)
	}
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	out, _ = update_agent_contract_rules.NewTool(testutil.NewClient(srv)).Handler(t.Context(),
		update_agent_contract_rules.Input{AgentContractID: contractID, Statements: []string{"a"}})
	if out.Status != update_agent_contract_rules.StatusNotFound {
		t.Errorf("404: %+v", out)
	}
}
