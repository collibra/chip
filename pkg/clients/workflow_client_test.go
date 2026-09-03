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
			_, _, err := GetWorkflowStartFormJSONModel(context.Background(), c, wfID)
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
	if _, _, err := GetWorkflowStartFormJSONModel(context.Background(), wfServer(t, mux), wfID); err == nil {
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
	fields, _, err := GetWorkflowStartFormJSONModel(context.Background(), wfServer(t, mux), wfID)
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
		_, _, _ = GetWorkflowStartFormJSONModel(context.Background(), wfServer(t, mux), wfID)

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
			fields, _, err := GetWorkflowStartFormData(context.Background(), wfServer(t, mux), wfID)
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

// jsonFormServer answers the GraphQL form query with the given FlowableFormModel JSON.
func jsonFormServer(t *testing.T, model string) *http.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"api": map[string]any{
			"workflowStartFormJsonModel": model,
		}}})
	})
	return wfServer(t, mux)
}

func mustJSONFormFields(t *testing.T, model string) []WorkflowFormField {
	t.Helper()
	fields, _, err := GetWorkflowStartFormJSONModel(context.Background(), jsonFormServer(t, model), wfID)
	if err != nil {
		t.Fatalf("parsing the form model failed: %v", err)
	}
	return fields
}

func findField(fields []WorkflowFormField, id string) *WorkflowFormField {
	for i := range fields {
		if fields[i].ID == id {
			return &fields[i]
		}
	}
	return nil
}

// TestJSONFormContainerIsWalkedByShapeNotOnlyByTypeName is the standing guard for the container
// bug class. Fields nested in a layout container were once never found at all: the form parsed
// clean, reported zero fields, and the workflow was then started with every variable unset.
//
// The list of container type names is the platform's, not ours, and it grows — so a container
// this client has never heard of must still be walked, on the strength of its SHAPE alone. Each
// case below therefore uses a type name that is NOT in jsonFormContainerTypes.
func TestJSONFormContainerIsWalkedByShapeNotOnlyByTypeName(t *testing.T) {
	inner := `{"id":"c1","type":"text","value":"{{buried}}","label":"Buried"}`
	for _, tc := range []struct{ name, extraSettings string }{
		{"layoutDefinition", `{"layoutDefinition":{"rows":[{"cols":[` + inner + `]}]}}`},
		{"sections", `{"sections":[{"id":"s","type":"panel","extraSettings":{"layoutDefinition":{"rows":[{"cols":[` + inner + `]}]}}}]}`},
		{"expandablePanel", `{"expandablePanel":{"id":"p","type":"panel","extraSettings":{"layoutDefinition":{"rows":[{"cols":[` + inner + `]}]}}}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := `{"rows":[{"cols":[{"id":"box","type":"someContainerWeHaveNeverSeen","extraSettings":` + tc.extraSettings + `}]}]}`
			fields := mustJSONFormFields(t, model)
			if len(fields) != 1 || fields[0].ID != "buried" {
				t.Fatalf("the nested field must be found by shape even for an unknown container type, got %+v", fields)
			}
		})
	}
}

// TestJSONFormIgnoredContainerIsSkippedByEveryRoute: `ignore` means the designer marked the
// element as not part of the submitted form. Both routes into a nested panel must honour it —
// expandablePanel used to recurse directly, bypassing the check, so the SAME panel yielded fields
// via one route and none via the other.
func TestJSONFormIgnoredContainerIsSkippedByEveryRoute(t *testing.T) {
	panel := `{"id":"p","type":"panel","ignore":true,"extraSettings":{"layoutDefinition":{"rows":[{"cols":[{"id":"c1","type":"text","value":"{{hidden}}"}]}]}}}`
	for _, tc := range []struct{ name, model string }{
		{"nested directly", `{"rows":[{"cols":[` + panel + `]}]}`},
		{"as expandablePanel", `{"rows":[{"cols":[{"id":"box","type":"panel","extraSettings":{"expandablePanel":` + panel + `}}]}]}`},
		{"as a section", `{"rows":[{"cols":[{"id":"box","type":"tabs","extraSettings":{"sections":[` + panel + `]}}]}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if fields := mustJSONFormFields(t, tc.model); len(fields) != 0 {
				t.Errorf("an ignored container must contribute nothing, got %+v", fields)
			}
		})
	}
}

// TestJSONFormSubformWithNoInlinedLayoutIsReportedUnsupported: a sub-form keeps its fields in a
// separate form definition referenced by extraSettings.formRef. If that layout is not inlined by
// the time this client sees it, silence would present a partial form as a complete one — the
// caller fills in what it can see and the sub-form's variables reach the process unset.
func TestJSONFormSubformWithNoInlinedLayoutIsReportedUnsupported(t *testing.T) {
	model := `{"rows":[{"cols":[{"id":"sub1","type":"subform","label":"Address","extraSettings":{"formRef":"addressForm"}}]}]}`
	fields := mustJSONFormFields(t, model)
	if len(fields) != 1 {
		t.Fatalf("an unresolvable sub-form must be reported, not dropped; got %+v", fields)
	}
	if fields[0].Unsupported == "" {
		t.Errorf("the sub-form placeholder must carry an Unsupported reason, got %+v", fields[0])
	}
}

// ...but when the layout IS inlined, the real fields are what matters — the placeholder must not
// appear alongside them and turn a complete form into an unstartable one.
func TestJSONFormSubformWithInlinedLayoutYieldsItsRealFields(t *testing.T) {
	model := `{"rows":[{"cols":[{"id":"sub1","type":"subform","extraSettings":{"formRef":"addressForm","layoutDefinition":{"rows":[{"cols":[{"id":"c1","type":"text","value":"{{street}}"}]}]}}}]}]}`
	fields := mustJSONFormFields(t, model)
	if len(fields) != 1 || fields[0].ID != "street" {
		t.Fatalf("an inlined sub-form must yield its real fields and no placeholder, got %+v", fields)
	}
	if fields[0].Unsupported != "" {
		t.Errorf("a resolved field must not be marked unsupported: %+v", fields[0])
	}
}

// TestJSONFormDefaultValueIsNotStringOnly: the palette declares a checkbox default as a real JSON
// boolean and a picker's as an array. A string-only accessor dropped both, so a field the form
// showed as pre-filled was submitted empty.
func TestJSONFormDefaultValueIsNotStringOnly(t *testing.T) {
	for _, tc := range []struct{ name, defaultValue, want string }{
		{"string", `"Normal"`, "Normal"},
		{"bool", `true`, "true"},
		{"number", `42`, "42"},
		{"array", `["a","b"]`, "a,b"},
		{"empty array", `[]`, ""},
		{"null", `null`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := `{"rows":[{"cols":[{"id":"c1","type":"text","value":"{{v}}","defaultValue":` + tc.defaultValue + `}]}]}`
			fields := mustJSONFormFields(t, model)
			if len(fields) != 1 {
				t.Fatalf("expected one field, got %+v", fields)
			}
			if fields[0].DefaultValue != tc.want {
				t.Errorf("default %s: got %q, want %q", tc.defaultValue, fields[0].DefaultValue, tc.want)
			}
		})
	}
}

// TestJSONFormMultiValueIsDetectedWithoutTheMultiFlag: the designer writes extraSettings.multi
// only for the select components. A checkbox group or a tags input takes several values with no
// flag at all, and treating one as single-valued rejects every list the caller could legitimately
// offer — with nothing in the response hinting that a list is even accepted.
func TestJSONFormMultiValueIsDetectedWithoutTheMultiFlag(t *testing.T) {
	for _, tc := range []struct {
		name, col string
		want      bool
	}{
		{"checkboxgroup by type", `{"id":"c1","type":"checkboxGroup","value":"{{v}}"}`, true},
		{"tags by type", `{"id":"c1","type":"tags","value":"{{v}}"}`, true},
		{"multiselect by type", `{"id":"c1","type":"multiSelect","value":"{{v}}"}`, true},
		{"select by flag", `{"id":"c1","type":"select","value":"{{v}}","extraSettings":{"multi":true}}`, true},
		{"plain text", `{"id":"c1","type":"text","value":"{{v}}"}`, false},
		{"select without the flag", `{"id":"c1","type":"select","value":"{{v}}","extraSettings":{}}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fields := mustJSONFormFields(t, `{"rows":[{"cols":[`+tc.col+`]}]}`)
			if len(fields) != 1 {
				t.Fatalf("expected one field, got %+v", fields)
			}
			if fields[0].MultiValue != tc.want {
				t.Errorf("MultiValue = %v, want %v", fields[0].MultiValue, tc.want)
			}
		})
	}
}

// TestLegacyFormFieldCarriesWritableAndValue: the legacy wire has `writable` and `value`, and both
// were parsed but never mapped onto the field. The effect was not cosmetic — a read-only field
// looked ordinary, so it was offered to the caller and its value submitted, which the server then
// rejects; and a declared default was invisible, so a form the UI pre-fills came through empty.
func TestLegacyFormFieldCarriesWritableAndValue(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/workflowDefinitions/workflowDefinition/"+wfID+"/startFormData", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"formProperties": []map[string]any{
			{"id": "locked", "name": "Locked", "type": "string", "writable": false, "value": "fixed"},
			{"id": "open", "name": "Open", "type": "string", "writable": true, "value": "preset"},
		}})
	})
	fields, _, err := GetWorkflowStartFormData(context.Background(), wfServer(t, mux), wfID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	locked, open := findField(fields, "locked"), findField(fields, "open")
	if locked == nil || open == nil {
		t.Fatalf("both fields must be present, got %+v", fields)
	}
	if !locked.ReadOnly {
		t.Errorf("writable:false must map to ReadOnly, got %+v", locked)
	}
	if open.ReadOnly {
		t.Errorf("writable:true must not be read-only, got %+v", open)
	}
	if locked.DefaultValue != "fixed" || open.DefaultValue != "preset" {
		t.Errorf("the wire `value` must map to DefaultValue, got %q / %q", locked.DefaultValue, open.DefaultValue)
	}
}

// TestFormFetchReturnsTheHTTPStatus: both form fetchers used to discard the status, so a 403, a
// 404 and a dead connection all reached the caller as one undifferentiated "retry" — advice that
// is wrong for two of the three. The status is what lets the tool tell them apart.
func TestFormFetchReturnsTheHTTPStatus(t *testing.T) {
	for _, status := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("GET /rest/2.0/workflowDefinitions/workflowDefinition/"+wfID+"/startFormData", func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, status, map[string]any{"errorCode": "nope", "userMessage": "denied"})
			})
			mux.HandleFunc("POST /graphql", func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, status, map[string]any{"errorCode": "nope", "userMessage": "denied"})
			})
			c := wfServer(t, mux)
			for name, call := range map[string]func() (int, error){
				"legacy": func() (int, error) {
					_, code, err := GetWorkflowStartFormData(context.Background(), c, wfID)
					return code, err
				},
				"json model": func() (int, error) {
					_, code, err := GetWorkflowStartFormJSONModel(context.Background(), c, wfID)
					return code, err
				},
			} {
				code, err := call()
				if err == nil {
					t.Fatalf("%s: HTTP %d must be an error", name, status)
				}
				if code != status {
					t.Errorf("%s: got status %d, want %d — the caller cannot distinguish failures without it", name, code, status)
				}
			}
		})
	}
}

// TestJSONFormPickerMultiValueUsesItsOwnSpelling. The form palette is really two palettes: the
// generic cloud components write extraSettings.multi, the collibra-* resource pickers write
// extraSettings.multiValue. Reading only the first reported every asset/user/group picker as
// single-valued.
//
// That is not cosmetic. A field reported single-valued is submitted as a bare string, and a
// Groovy String iterates per CHARACTER — an OOTB form with two such pickers failed to start at all
// once one was filled in. Both spellings, therefore, and each pinned separately.
func TestJSONFormPickerMultiValueUsesItsOwnSpelling(t *testing.T) {
	for _, tc := range []struct {
		name, extraSettings string
		want                bool
	}{
		{"picker multiValue", `{"storage":"Id","multiValue":true}`, true},
		{"picker multiValue false", `{"storage":"Id","multiValue":false}`, false},
		{"select multi", `{"multi":true}`, true},
		{"select multi false", `{"multi":false}`, false},
		{"neither", `{"storage":"Id"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := `{"rows":[{"cols":[{"id":"c1","type":"asset","value":"{{picked}}","extraSettings":` + tc.extraSettings + `}]}]}`
			fields := mustJSONFormFields(t, model)
			if len(fields) != 1 {
				t.Fatalf("expected one field, got %+v", fields)
			}
			if fields[0].MultiValue != tc.want {
				t.Errorf("MultiValue = %v, want %v", fields[0].MultiValue, tc.want)
			}
		})
	}
}

// TestJSONFormFullStoragePickerIsUnsupported. A picker's extraSettings.storage says what the
// process actually receives: "Id" (or nothing) means the plain resource id, which this client can
// supply. "Full" means the whole picked resource, and the process reads properties off it — the
// OOTB IssueMove start script does UUID.fromString("${responsibleCommunity.value}").
//
// Handing that a bare id string resolves .value to nothing, and the failure lands inside the
// workflow's transaction as an opaque HTTP 500 after the user approved the preview. Guessing the
// object's shape is not an option either: nothing here knows which properties a given script
// reads. So the field says plainly that it cannot be filled in from here.
func TestJSONFormFullStoragePickerIsUnsupported(t *testing.T) {
	for _, tc := range []struct {
		name, storage   string
		wantUnsupported bool
	}{
		{"full", `"storage":"Full",`, true},
		{"id", `"storage":"Id",`, false},
		{"absent", ``, false},
		{"lowercase id", `"storage":"id",`, false},
		{"some future mode", `"storage":"Reference",`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := `{"rows":[{"cols":[{"id":"c1","type":"community","value":"{{comm}}","extraSettings":{` + tc.storage + `"multiValue":false}}]}]}`
			fields := mustJSONFormFields(t, model)
			if len(fields) != 1 {
				t.Fatalf("expected one field, got %+v", fields)
			}
			if got := fields[0].Unsupported != ""; got != tc.wantUnsupported {
				t.Errorf("Unsupported=%v (%q), want %v", got, fields[0].Unsupported, tc.wantUnsupported)
			}
		})
	}
}

// ...and a reason this client already has for the field must not be overwritten by the storage
// one, which is the less specific of the two.
func TestJSONFormStorageDoesNotMaskAMoreSpecificReason(t *testing.T) {
	model := `{"rows":[{"cols":[{"id":"c1","type":"collibra-fileUpload","value":"{{f}}","designInfo":{"stencilId":"collibra-fileUpload"},"extraSettings":{"storage":"Full"}}]}]}`
	fields := mustJSONFormFields(t, model)
	if len(fields) != 1 {
		t.Fatalf("expected one field, got %+v", fields)
	}
	if !strings.Contains(fields[0].Unsupported, "uploaded file") {
		t.Errorf("the file-upload reason must win over the generic storage one, got %q", fields[0].Unsupported)
	}
}
