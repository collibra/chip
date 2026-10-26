package list_governance_domains_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/tools/list_governance_domains"
	"github.com/collibra/chip/pkg/tools/testutil"
)

func TestDomains_LooksUpTypeThenFiltersDomains(t *testing.T) {
	var domainQuery http.Header
	var typeID, name string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/rest/2.0/domainTypes":
			if r.URL.Query().Get("name") != "Governance Asset Domain" {
				t.Errorf("wrong type lookup: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"results":[{"id":"type-1"}]}`))
		case "/rest/2.0/domains":
			typeID, name = r.URL.Query().Get("typeId"), r.URL.Query().Get("name")
			_ = domainQuery
			_, _ = w.Write([]byte(`{"results":[{"id":"d1","name":"Agent Contracts","community":{"name":"AI Governance"}}]}`))
		}
	}))
	defer srv.Close()
	out, err := list_governance_domains.NewTool(testutil.NewClient(srv)).Handler(t.Context(), list_governance_domains.Input{Name: "contract"})
	if err != nil || out.Status != list_governance_domains.StatusSuccess || len(out.Domains) != 1 || out.Domains[0].ID != "d1" || out.Domains[0].CommunityName != "AI Governance" {
		t.Fatalf("unexpected: %+v (err=%v)", out, err)
	}
	if typeID != "type-1" || name != "contract" {
		t.Errorf("domains query typeId=%q name=%q", typeID, name)
	}
}

func TestDomains_NoDomainTypeOrBadLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"results":[]}`)) }))
	defer srv.Close()
	tool := list_governance_domains.NewTool(testutil.NewClient(srv))
	if out, _ := tool.Handler(t.Context(), list_governance_domains.Input{}); out.Status != list_governance_domains.StatusSuccess || len(out.Domains) != 0 {
		t.Errorf("unexpected: %+v", out)
	}
	if out, _ := tool.Handler(t.Context(), list_governance_domains.Input{Limit: 900}); out.Status != list_governance_domains.StatusValidationError {
		t.Errorf("unexpected: %+v", out)
	}
}
