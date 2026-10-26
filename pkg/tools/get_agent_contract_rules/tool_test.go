package get_agent_contract_rules_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/tools/get_agent_contract_rules"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	contractID = "0f8c3e2a-6b1d-4c9e-9a47-3d2b5e8f1a60"
	versionID  = "c4a1f7e9-2d3b-4e6a-8f50-1b9c7d2e4a83"
	list       = `{"agentContract":{"id":"` + contractID + `","layer":"GLOBAL"},"agentContractVersion":{"id":"` + versionID + `","versionNumber":4},"results":[{"id":"r1","statement":"Be kind.","rank":1}]}`
)

func serve(t *testing.T) (*http.Client, *[]string) {
	t.Helper()
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch r.URL.Path {
		case "/rest/aiGovernance/v1/agentContracts/" + contractID + "/rules", "/rest/aiGovernance/v1/agentContractVersions/" + versionID + "/rules":
			_, _ = w.Write([]byte(list))
		default:
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"errorCode":"agentContractNotFound"}`))
		}
	}))
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv), &paths
}

func TestRules_ActiveVersionAndExactVersion(t *testing.T) {
	client, paths := serve(t)
	for _, in := range []get_agent_contract_rules.Input{{AgentContractID: contractID}, {VersionID: versionID}} {
		out, err := get_agent_contract_rules.NewTool(client).Handler(t.Context(), in)
		if err != nil || out.Status != get_agent_contract_rules.StatusSuccess || out.VersionNumber != 4 || len(out.Rules) != 1 || out.Rules[0].Statement != "Be kind." || out.Layer != "GLOBAL" {
			t.Fatalf("unexpected: %+v (err=%v)", out, err)
		}
	}
	if len(*paths) != 2 {
		t.Errorf("expected two calls, got %v", *paths)
	}
}

func TestRules_ExactlyOneIdAndNotFound(t *testing.T) {
	client, paths := serve(t)
	for name, in := range map[string]get_agent_contract_rules.Input{
		"none": {}, "both": {AgentContractID: contractID, VersionID: versionID}, "bad": {AgentContractID: "x"},
	} {
		if out, _ := get_agent_contract_rules.NewTool(client).Handler(t.Context(), in); out.Status != get_agent_contract_rules.StatusValidationError {
			t.Errorf("%s: %+v", name, out)
		}
	}
	if len(*paths) != 0 {
		t.Errorf("validation errors must not call Collibra: %v", *paths)
	}
	out, _ := get_agent_contract_rules.NewTool(client).Handler(t.Context(), get_agent_contract_rules.Input{AgentContractID: "11111111-1111-1111-1111-111111111111"})
	if out.Status != get_agent_contract_rules.StatusNotFound || out.HTTPStatus != 404 {
		t.Errorf("unexpected: %+v", out)
	}
}
