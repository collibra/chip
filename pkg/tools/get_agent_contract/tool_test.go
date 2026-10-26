package get_agent_contract_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/tools/get_agent_contract"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const id = "0f8c3e2a-6b1d-4c9e-9a47-3d2b5e8f1a60"

func TestGet_SuccessNotFoundAndValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rest/aiGovernance/v1/agentContracts/"+id {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"errorCode":"agentContractNotFound"}`))
			return
		}
		_, _ = w.Write([]byte(`{"id":"` + id + `","name":"HR","layer":"SPECIALIZED","activeVersionId":"v","activeVersionNumber":2,"ruleCount":3,"governedAgentCount":7}`))
	}))
	defer srv.Close()
	tool := get_agent_contract.NewTool(testutil.NewClient(srv))

	out, err := tool.Handler(t.Context(), get_agent_contract.Input{AgentContractID: id})
	if err != nil || out.Status != get_agent_contract.StatusSuccess || out.Name != "HR" || out.RuleCount != 3 || out.GovernedAgentCount == nil || *out.GovernedAgentCount != 7 {
		t.Fatalf("unexpected: %+v (err=%v)", out, err)
	}
	if out, _ := tool.Handler(t.Context(), get_agent_contract.Input{AgentContractID: "11111111-1111-1111-1111-111111111111"}); out.Status != get_agent_contract.StatusNotFound {
		t.Errorf("expected not_found, got %+v", out)
	}
	if out, _ := tool.Handler(t.Context(), get_agent_contract.Input{AgentContractID: "nope"}); out.Status != get_agent_contract.StatusValidationError {
		t.Errorf("expected validation_error, got %+v", out)
	}
}
