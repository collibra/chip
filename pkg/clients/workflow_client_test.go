package clients

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// This file exists because the workflow client was previously exercised only indirectly, through
// the two tool packages. Anything it does that has no tool-visible effect — a wire tag, a guard
// against a malformed response — was therefore unverified. The tests below target those directly.

const wfID = "11111111-1111-1111-1111-111111111111"

func wfServer(t *testing.T, mux *http.ServeMux) *http.Client {
	t.Helper()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return newTestClient(srv)
}

// TestStartWorkflowInstance_EmptyResponseIsAnErrorNotANilInstance guards a process-killing crash.
// The start endpoints answer with an ARRAY of started instances; an empty one must be refused
// here, because the caller immediately reads instance.ID and nothing in chip or the MCP SDK
// recovers from a panic in a tool handler — one malformed response would take the whole server
// down, not just the call.
func TestStartWorkflowInstance_EmptyResponseIsAnErrorNotANilInstance(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		call       func(*http.Client) (*WorkflowInstance, int, error)
	}{
		{"legacy", "/rest/2.0/workflowInstances", func(c *http.Client) (*WorkflowInstance, int, error) {
			return StartWorkflowInstance(context.Background(), c, StartWorkflowInstanceRequest{WorkflowDefinitionID: wfID})
		}},
		{"json model", "/rest/2.0/internal/workflow/startWithForm", func(c *http.Client) (*WorkflowInstance, int, error) {
			return StartWorkflowInstanceWithForm(context.Background(), c, StartWorkflowInstanceWithFormRequest{WorkflowDefinitionID: wfID})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("POST "+tc.path, func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, http.StatusCreated, []map[string]string{})
			})
			inst, _, err := tc.call(wfServer(t, mux))
			if err == nil {
				t.Fatalf("an empty instance array must be an error; got instance=%v", inst)
			}
			if inst != nil {
				t.Errorf("no instance may be returned alongside the error, got %+v", inst)
			}
		})
	}
}

// TestStartWorkflowInstance_SendsPluralBusinessItemIdsOnTheWire asserts the RAW body. The tool
// tests decode into the very struct the client marshals from, so a renamed json tag round-trips
// perfectly and stays invisible — the same singular/plural class of bug that already made asset
// scoping a silent no-op once, on the query-param side.
func TestStartWorkflowInstance_SendsPluralBusinessItemIdsOnTheWire(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		call       func(*http.Client)
	}{
		{"legacy", "/rest/2.0/workflowInstances", func(c *http.Client) {
			_, _, _ = StartWorkflowInstance(context.Background(), c, StartWorkflowInstanceRequest{
				WorkflowDefinitionID: wfID, BusinessItemIDs: []string{"asset-1"}, BusinessItemType: "ASSET",
			})
		}},
		{"json model", "/rest/2.0/internal/workflow/startWithForm", func(c *http.Client) {
			_, _, _ = StartWorkflowInstanceWithForm(context.Background(), c, StartWorkflowInstanceWithFormRequest{
				WorkflowDefinitionID: wfID, BusinessItemIDs: []string{"asset-1"}, BusinessItemType: "ASSET",
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var raw string
			mux := http.NewServeMux()
			mux.HandleFunc("POST "+tc.path, func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				raw = string(b)
				writeJSON(w, http.StatusCreated, []map[string]string{{"id": "inst-1"}})
			})
			tc.call(wfServer(t, mux))

			var sent map[string]any
			if err := json.Unmarshal([]byte(raw), &sent); err != nil {
				t.Fatalf("bad body %q: %v", raw, err)
			}
			if _, ok := sent["businessItemIds"]; !ok {
				t.Errorf("body must carry the plural key \"businessItemIds\"; a rename is silently ignored by the server: %s", raw)
			}
			if _, ok := sent["workflowDefinitionId"]; !ok {
				t.Errorf("body must carry \"workflowDefinitionId\": %s", raw)
			}
		})
	}
}

// TestGraphQLErrorsArrayIsNeverIgnored: GraphQL answers HTTP 200 even when it failed, and may
// return errors ALONGSIDE partial data. Ignoring the array turns a failure into "this workflow has
// no start form" or "you have no startable workflows" — confident, wrong, and silent.
func TestGraphQLErrorsArrayIsNeverIgnored(t *testing.T) {
	body := `{"errors":[{"message":"ValidationError: field not found"}],"data":{"api":{"workflowDefinitionsGlobal":[],"workflowStartFormJsonModel":null}}}`
	for _, tc := range []struct {
		name string
		call func(*http.Client) error
	}{
		{"global list", func(c *http.Client) error {
			_, _, err := ListGlobalWorkflowDefinitions(context.Background(), c)
			return err
		}},
		{"json start form", func(c *http.Client) error {
			_, err := GetWorkflowStartFormJSONModel(context.Background(), c, wfID)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			})
			if err := tc.call(wfServer(t, mux)); err == nil {
				t.Errorf("a GraphQL errors array must surface as an error, not as an empty result")
			}
		})
	}
}

// TestGetWorkflowStartFormJSONModel_NullModelIsAnError: this is only called for a definition whose
// startFormJsonModelAvailable flag is set, so a null model means the flag and the stored form
// disagree. Returning "no fields" would let the caller submit an empty form and report success.
func TestGetWorkflowStartFormJSONModel_NullModelIsAnError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"api": map[string]any{"workflowStartFormJsonModel": nil}}})
	})
	if _, err := GetWorkflowStartFormJSONModel(context.Background(), wfServer(t, mux), wfID); err == nil {
		t.Errorf("a null form model must be an error, not an empty field list")
	}
}

// TestGetWorkflowStartFormJSONModel_EmptyRowsIsLegitimate is the other half: a form model with no
// rows is a real, valid form that happens to have no fields. It must NOT be treated as the failure
// above.
func TestGetWorkflowStartFormJSONModel_EmptyRowsIsLegitimate(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"api": map[string]any{
			"workflowStartFormJsonModel": `{"rows":[],"metadata":{}}`,
		}}})
	})
	fields, err := GetWorkflowStartFormJSONModel(context.Background(), wfServer(t, mux), wfID)
	if err != nil {
		t.Fatalf("an empty rows array is a legitimate form: %v", err)
	}
	if len(fields) != 0 {
		t.Errorf("expected no fields, got %+v", fields)
	}
}

// TestGraphQLQueriesCarryTheirArguments asserts the request text, which the tool-level mocks never
// read. Without this the global query could lose `enabled: true` or regain the `globalCreate`
// narrowing that was deliberately dropped, and the form query could send the wrong variable name —
// all invisible to a mock that answers regardless of what it was asked.
func TestGraphQLQueriesCarryTheirArguments(t *testing.T) {
	t.Run("global list", func(t *testing.T) {
		var req map[string]any
		mux := http.NewServeMux()
		mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&req)
			writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"api": map[string]any{"workflowDefinitionsGlobal": []any{}}}})
		})
		_, _, _ = ListGlobalWorkflowDefinitions(context.Background(), wfServer(t, mux))

		q, _ := req["query"].(string)
		if !strings.Contains(q, "enabled: true") {
			t.Errorf("query must ask for enabled definitions only: %s", q)
		}
		if strings.Contains(q, "globalCreate") {
			t.Errorf("globalCreate narrowing was deliberately dropped — it excludes workflows a user can start: %s", q)
		}
		for _, field := range []string{"startLabel", "description", "formRequired", "startFormJsonModelAvailable"} {
			if !strings.Contains(q, field) {
				t.Errorf("query must select %q, which the tool surfaces: %s", field, q)
			}
		}
	})

	t.Run("start form", func(t *testing.T) {
		var req map[string]any
		mux := http.NewServeMux()
		mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&req)
			writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"api": map[string]any{"workflowStartFormJsonModel": `{"rows":[]}`}}})
		})
		_, _ = GetWorkflowStartFormJSONModel(context.Background(), wfServer(t, mux), wfID)

		vars, _ := req["variables"].(map[string]any)
		if vars["workflowDefinitionId"] != wfID {
			t.Errorf("the workflow id must travel under the variable name the query declares, got %v", vars)
		}
	})
}

// TestFindWorkflowDefinitions_AllScopeParamsAreSingular extends the assetId regression to the two
// scopes it never covered. A plural name is silently ignored by the server, which then returns the
// full unfiltered, un-authorization-checked list — the tool would present instance-wide results as
// resource-scoped, breaking the one guarantee it exists to provide.
func TestFindWorkflowDefinitions_AllScopeParamsAreSingular(t *testing.T) {
	for _, tc := range []struct {
		param  string
		filter FindWorkflowDefinitionsFilter
	}{
		{"assetId", FindWorkflowDefinitionsFilter{AssetID: "a-1"}},
		{"domainId", FindWorkflowDefinitionsFilter{DomainID: "d-1"}},
		{"communityId", FindWorkflowDefinitionsFilter{CommunityID: "c-1"}},
	} {
		t.Run(tc.param, func(t *testing.T) {
			var q map[string][]string
			mux := http.NewServeMux()
			mux.HandleFunc("GET /rest/2.0/workflowDefinitions", func(w http.ResponseWriter, r *http.Request) {
				q = r.URL.Query()
				writeJSON(w, http.StatusOK, WorkflowDefinitionsPage{})
			})
			_, _, _ = FindWorkflowDefinitions(context.Background(), wfServer(t, mux), tc.filter)

			if len(q[tc.param]) != 1 {
				t.Errorf("query must carry the singular %q, got %v", tc.param, q)
			}
			if _, plural := q[tc.param+"s"]; plural {
				t.Errorf("query sent the plural %qs, which the server ignores: %v", tc.param, q)
			}
		})
	}
}

// TestFindWorkflowDefinitions_PagingParamsUseTheirDocumentedNames: the tool asserts only that
// offset is NOT some value, which an absent parameter also satisfies. A renamed tag would make
// scoped paging silently return page one forever while hasMore keeps saying to page on.
func TestFindWorkflowDefinitions_PagingParamsUseTheirDocumentedNames(t *testing.T) {
	var q map[string][]string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/workflowDefinitions", func(w http.ResponseWriter, r *http.Request) {
		q = r.URL.Query()
		writeJSON(w, http.StatusOK, WorkflowDefinitionsPage{})
	})
	_, _, _ = FindWorkflowDefinitions(context.Background(), wfServer(t, mux), FindWorkflowDefinitionsFilter{
		AssetID: "a-1", Offset: 25, Limit: 10,
	})
	if got := q["offset"]; len(got) != 1 || got[0] != "25" {
		t.Errorf("offset param = %v, want [25]", got)
	}
	if got := q["limit"]; len(got) != 1 || got[0] != "10" {
		t.Errorf("limit param = %v, want [10]", got)
	}
}

// TestToLegacyFormField_CheckAndRadioOptionsUseValueNotLabel covers the legacy wire shapes that no
// tool test exercises. Choice values must be submitted as the option KEY; keying them by the label
// would be rejected by the server — the same defect the enum path has a test for, on its untested
// siblings.
func TestToLegacyFormField_CheckAndRadioOptionsUseValueNotLabel(t *testing.T) {
	for _, shape := range []string{"checkButtons", "radioButtons"} {
		t.Run(shape, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /rest/2.0/workflowDefinitions/workflowDefinition/"+wfID+"/startFormData",
				func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"formProperties":[{"id":"f","name":"F","type":"checkbox","required":true,
					  "`+shape+`":[{"value":"yes","label":"Yes please"}]}]}`)
				})
			fields, err := GetWorkflowStartFormData(context.Background(), wfServer(t, mux), wfID)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(fields) != 1 || len(fields[0].Options) != 1 {
				t.Fatalf("expected one field with one option, got %+v", fields)
			}
			if got := fields[0].Options[0].Key; got != "yes" {
				t.Errorf("option key = %q, want the value %q — the label is not accepted on submit", got, "yes")
			}
			if !fields[0].OptionsExhaustive {
				t.Errorf("a checkbox/radio list is a closed set and must be marked exhaustive")
			}
		})
	}
}
