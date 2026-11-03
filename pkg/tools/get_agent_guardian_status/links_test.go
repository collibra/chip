package get_agent_guardian_status_test

import (
	"net/http"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/tools/get_agent_guardian_status"
)

func TestGetAgentGuardianStatus_ReturnsLinksToTheAgentsPages(t *testing.T) {
	f := newFakeGuardian(t, http.StatusOK, overview)
	ctx := chip.SetCollibraHost(t.Context(), "https://example.collibra.com/")
	out, err := get_agent_guardian_status.NewTool(f.client).Handler(ctx, get_agent_guardian_status.Input{AIAgentID: aiAgentID})
	if err != nil {
		t.Fatal(err)
	}
	l := out.Links
	if l == nil {
		t.Fatal("no links returned")
	}
	host := "https://example.collibra.com"
	want := map[string]string{
		"agent":          host + "/asset/" + aiAgentID,
		"agentVersion":   host + "/asset/5e6f7a8b-9c0d-4e1f-8a2b-3c4d5e6f7a8b",
		"contractTab":    host + "/asset/" + aiAgentID + "?tabbar=Contract",
		"guardianTab":    host + "/asset/" + aiAgentID + "?tabbar=Guardian",
		"allContracts":   host + "/ai-governance/agent-contracts",
		"guardianAgents": host + "/ai-governance/guardian-agents",
	}
	got := map[string]string{
		"agent": l.Agent, "agentVersion": l.AgentVersion, "contractTab": l.ContractTab,
		"guardianTab": l.GuardianTab, "allContracts": l.AllContracts, "guardianAgents": l.GuardianAgents,
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: got %q, want %q", k, got[k], v)
		}
	}
	if len(l.Contracts) != 2 || l.Contracts[1].URL != host+"/ai-governance/agent-contracts/3c4d5e6f-7a8b-4c9d-8e0f-1a2b3c4d5e6f" || l.Contracts[1].Name != "Support contract" {
		t.Errorf("contract links: %+v", l.Contracts)
	}
}

func TestGetAgentGuardianStatus_NoLinksWithoutTheCollibraAddress(t *testing.T) {
	f := newFakeGuardian(t, http.StatusOK, overview)
	if out := run(t, f, aiAgentID); out.Links != nil {
		t.Errorf("links without a known address: %+v", out.Links)
	}
}
