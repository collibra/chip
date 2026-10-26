package create_agent_contract_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/collibra/chip/pkg/tools/create_agent_contract"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const domainID = "4d250cc5-e583-4640-9874-b93d82c7a6cb"

type fake struct {
	client *http.Client
	calls  atomic.Int32
	body   string
}

func newFake(t *testing.T, status int, response string) *fake {
	t.Helper()
	f := &fake{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /rest/aiGovernance/v1/agentContracts", func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		b, _ := io.ReadAll(r.Body)
		f.body = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.client = testutil.NewClient(srv)
	return f
}

func input(confirm bool) create_agent_contract.Input {
	return create_agent_contract.Input{
		Layer: "specialized", Name: " HR agents ", DomainID: domainID, Confirm: confirm,
		Statements: []string{"  An agent must not  send data outside.  ", "Every message must say it is automated."},
	}
}

const created = `{"id":"0f8c3e2a-6b1d-4c9e-9a47-3d2b5e8f1a60","name":"HR agents","layer":"SPECIALIZED","activeVersionId":"c4a1f7e9-2d3b-4e6a-8f50-1b9c7d2e4a83","activeVersionNumber":1,"ruleCount":2,"lastModifiedBy":"7b2e9d14-3a6f-4c81-b5e0-9f4d2a7c1e38"}`

func TestCreate_PreviewSendsNothingAndShowsNormalizedStatements(t *testing.T) {
	f := newFake(t, http.StatusCreated, created)
	out, err := create_agent_contract.NewTool(f.client).Handler(t.Context(), input(false))
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != create_agent_contract.StatusPreview || out.Preview == nil || f.calls.Load() != 0 {
		t.Fatalf("expected a preview with no call, got %+v (calls=%d)", out, f.calls.Load())
	}
	if out.Preview.Layer != "SPECIALIZED" || out.Preview.Name != "HR agents" || out.Preview.Statements[0] != "An agent must not send data outside." {
		t.Errorf("preview not normalized: %+v", out.Preview)
	}
}

func TestCreate_ConfirmPostsBodyAndReturnsIds(t *testing.T) {
	f := newFake(t, http.StatusCreated, created)
	out, err := create_agent_contract.NewTool(f.client).Handler(t.Context(), input(true))
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != create_agent_contract.StatusSuccess || out.AgentContractID == "" || out.VersionNumber != 1 || out.RuleCount != 2 || out.CreatedBy != "7b2e9d14-3a6f-4c81-b5e0-9f4d2a7c1e38" {
		t.Fatalf("unexpected output: %+v", out)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(f.body), &sent); err != nil {
		t.Fatal(err)
	}
	if sent["layer"] != "SPECIALIZED" || sent["domainId"] != domainID || len(sent["statements"].([]any)) != 2 {
		t.Errorf("unexpected request body: %s", f.body)
	}
}

func TestCreate_ValidationErrorsSendNothing(t *testing.T) {
	f := newFake(t, http.StatusCreated, created)
	cases := map[string]func(*create_agent_contract.Input){
		"layer":     func(i *create_agent_contract.Input) { i.Layer = "LOCAL" },
		"name":      func(i *create_agent_contract.Input) { i.Name = "  " },
		"domain":    func(i *create_agent_contract.Input) { i.DomainID = "nope" },
		"empty":     func(i *create_agent_contract.Input) { i.Statements = nil },
		"blank":     func(i *create_agent_contract.Input) { i.Statements = []string{"   "} },
		"duplicate": func(i *create_agent_contract.Input) { i.Statements = []string{"a  b", "a b"} },
		"too long":  func(i *create_agent_contract.Input) { i.Statements = []string{strings.Repeat("x", 2001)} },
		"too many":  func(i *create_agent_contract.Input) { i.Statements = make([]string, 201) },
	}
	for name, mutate := range cases {
		in := input(true)
		mutate(&in)
		out, err := create_agent_contract.NewTool(f.client).Handler(t.Context(), in)
		if err != nil || out.Status != create_agent_contract.StatusValidationError {
			t.Errorf("%s: expected validation_error, got %+v (err=%v)", name, out, err)
		}
	}
	if f.calls.Load() != 0 {
		t.Errorf("validation errors must not call Collibra, got %d calls", f.calls.Load())
	}
}

func TestCreate_RejectionsAreExplainedAndNotRetried(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"forbidden":     {403, `{"statusCode":403,"errorCode":"authorizationFailed","userMessage":"no"}`, "permission"},
		"feature off":   {403, `{"statusCode":403,"errorCode":"agentContractsFeatureDisabled"}`, "AI Command Center"},
		"global exists": {409, `{"statusCode":409,"errorCode":"globalAgentContractAlreadyExists"}`, "global agent contract already exists"},
		"unauthorized":  {401, `{"statusCode":401,"errorCode":"authenticationFailed"}`, "sign in"},
	} {
		f := newFake(t, tc.status, tc.body)
		out, err := create_agent_contract.NewTool(f.client).Handler(t.Context(), input(true))
		if err != nil || out.Status != create_agent_contract.StatusError || out.HTTPStatus != tc.status {
			t.Errorf("%s: unexpected output %+v (err=%v)", name, out, err)
			continue
		}
		if !strings.Contains(strings.ToLower(out.Message+" "+out.Guidance), strings.ToLower(tc.want)) {
			t.Errorf("%s: message/guidance %q / %q do not mention %q", name, out.Message, out.Guidance, tc.want)
		}
		if f.calls.Load() != 1 {
			t.Errorf("%s: expected exactly one call, got %d", name, f.calls.Load())
		}
	}
}
