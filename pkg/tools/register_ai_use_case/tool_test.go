package register_ai_use_case_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/collibra/chip/pkg/tools/register_ai_use_case"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const domainID = "01a1b100-f494-72c0-b04b-cdf13c68a3eb"

type fake struct {
	client *http.Client
	calls  atomic.Int32
	body   map[string]any
}

func newFake(t *testing.T, status int, response string) *fake {
	t.Helper()
	f := &fake{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/users/current", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"user-1"}`))
	})
	mux.HandleFunc("POST /rest/aiGovernance/v1/aiUseCases/register", func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &f.body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.client = testutil.NewClient(srv)
	return f
}

func input(confirm bool) register_ai_use_case.Input {
	return register_ai_use_case.Input{Name: " Loan assessment ", DomainID: domainID, Description: " Assesses loan requests. ", Confirm: confirm}
}

func TestRegister_PreviewSendsNothing(t *testing.T) {
	f := newFake(t, http.StatusCreated, `{"id":"01a1e57b-9669-77b3-9a08-4e0ea48e7e82"}`)
	out, err := register_ai_use_case.NewTool(f.client).Handler(t.Context(), input(false))
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != register_ai_use_case.StatusPreview || out.Preview == nil || f.calls.Load() != 0 {
		t.Fatalf("expected a preview with no call, got %+v (calls=%d)", out, f.calls.Load())
	}
	if out.Preview.Name != "Loan assessment" || out.Preview.Description != "Assesses loan requests." {
		t.Errorf("preview not trimmed: %+v", out.Preview)
	}
}

func TestRegister_ConfirmSendsTheRequestAndReturnsTheID(t *testing.T) {
	f := newFake(t, http.StatusCreated, `{"id":"01a1e57b-9669-77b3-9a08-4e0ea48e7e82"}`)
	out, _ := register_ai_use_case.NewTool(f.client).Handler(t.Context(), input(true))
	if out.Status != register_ai_use_case.StatusSuccess || out.AiUseCaseID != "01a1e57b-9669-77b3-9a08-4e0ea48e7e82" {
		t.Fatalf("unexpected: %+v", out)
	}
	owners, _ := f.body["ownerIds"].([]any)
	if len(owners) != 1 || owners[0] != "user-1" {
		t.Errorf("the current user must be the owner, sent ownerIds = %v", f.body["ownerIds"])
	}
	if f.body["name"] != "Loan assessment" || f.body["domainId"] != domainID || f.body["description"] != "Assesses loan requests." {
		t.Errorf("sent body = %v", f.body)
	}
}

func TestRegister_ForbiddenIsNotRetriedAndNamesTheRoles(t *testing.T) {
	f := newFake(t, http.StatusForbidden, `{"statusCode":403,"titleMessage":"Not authorized","userMessage":"You are not allowed."}`)
	out, _ := register_ai_use_case.NewTool(f.client).Handler(t.Context(), input(true))
	if out.Status != register_ai_use_case.StatusError || out.HTTPStatus != http.StatusForbidden || f.calls.Load() != 1 {
		t.Fatalf("unexpected: %+v (calls=%d)", out, f.calls.Load())
	}
	if out.Guidance == "" || !strings.Contains(out.Guidance, "AI Business User") {
		t.Errorf("guidance should name the roles: %q", out.Guidance)
	}
}

func TestRegister_InvalidInputSendsNothing(t *testing.T) {
	f := newFake(t, http.StatusCreated, `{}`)
	tool := register_ai_use_case.NewTool(f.client)
	for _, in := range []register_ai_use_case.Input{{Name: " ", DomainID: domainID, Confirm: true}, {Name: "x", DomainID: "not-a-uuid", Confirm: true}} {
		if out, _ := tool.Handler(t.Context(), in); out.Status != register_ai_use_case.StatusValidationError {
			t.Errorf("input %+v: want validation_error, got %+v", in, out)
		}
	}
	if f.calls.Load() != 0 {
		t.Errorf("expected no call, got %d", f.calls.Load())
	}
}
