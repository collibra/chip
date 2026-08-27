package start_collibra_workflow_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/start_collibra_workflow"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// handlers configures the mocked workflow endpoints. A nil handler for a request that arrives
// fails the test — that lets each test assert an endpoint was (or wasn't) reached. globalPermissions,
// currentUser and userResponsibilities default to permissive stand-ins (see newServer) so existing
// tests don't need to know about the authorization plumbing unless they're specifically testing it.
type handlers struct {
	definitionByID       func(w http.ResponseWriter, r *http.Request, workflowDefinitionID string)
	listDefinitions      func(w http.ResponseWriter, r *http.Request) // GET /rest/2.0/workflowDefinitions (with or without assetIds)
	startFormData        func(w http.ResponseWriter, r *http.Request, workflowDefinitionID string)
	start                func(w http.ResponseWriter, r *http.Request)
	globalPermissions    func(w http.ResponseWriter, r *http.Request)
	currentUser          func(w http.ResponseWriter, r *http.Request)
	userResponsibilities func(w http.ResponseWriter, r *http.Request)
}

func newServer(t *testing.T, h handlers) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/2.0/workflowDefinitions/workflowDefinition/", func(w http.ResponseWriter, r *http.Request) {
		if h.startFormData == nil {
			t.Errorf("unexpected start-form-data call: %s", r.URL)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/rest/2.0/workflowDefinitions/workflowDefinition/"), "/startFormData")
		h.startFormData(w, r, id)
	})
	mux.HandleFunc("/rest/2.0/workflowDefinitions/", func(w http.ResponseWriter, r *http.Request) {
		if h.definitionByID == nil {
			t.Errorf("unexpected definition-by-id call: %s", r.URL)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/workflowDefinitions/")
		h.definitionByID(w, r, id)
	})
	mux.HandleFunc("/rest/2.0/workflowDefinitions", func(w http.ResponseWriter, r *http.Request) {
		if h.listDefinitions == nil {
			t.Errorf("unexpected list-definitions call: %s", r.URL)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		h.listDefinitions(w, r)
	})
	mux.HandleFunc("/rest/2.0/workflowInstances", func(w http.ResponseWriter, r *http.Request) {
		if h.start == nil {
			t.Errorf("unexpected start call: %s", r.URL)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		h.start(w, r)
	})
	mux.HandleFunc("/rest/2.0/users/current/globalPermissions", func(w http.ResponseWriter, r *http.Request) {
		if h.globalPermissions != nil {
			h.globalPermissions(w, r)
			return
		}
		jsonHandler(http.StatusOK, map[string]any{"globalPermissions": []string{"WORKFLOW_START"}})(w, r)
	})
	mux.HandleFunc("/rest/2.0/users/current", func(w http.ResponseWriter, r *http.Request) {
		if h.currentUser != nil {
			h.currentUser(w, r)
			return
		}
		jsonHandler(http.StatusOK, map[string]any{"id": currentUserID, "userName": "test.user"})(w, r)
	})
	mux.HandleFunc("/rest/2.0/responsibilities", func(w http.ResponseWriter, r *http.Request) {
		if h.userResponsibilities != nil {
			h.userResponsibilities(w, r)
			return
		}
		jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{}})(w, r)
	})
	return httptest.NewServer(mux)
}

func jsonHandler(code int, body any) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		if body != nil {
			_ = json.NewEncoder(w).Encode(body)
		}
	}
}

func run(t *testing.T, server *httptest.Server, in tools.Input) tools.Output {
	t.Helper()
	out, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), in)
	if err != nil {
		t.Fatalf("handler returned a Go error (should surface via Output): %v", err)
	}
	return out
}

const (
	businessTermWorkflowID  = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	accessRequestWorkflowID = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	assetID                 = "11111111-1111-1111-1111-111111111111"
	currentUserID           = "22222222-2222-2222-2222-222222222222"
)

func writeNotFound(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`{"statusCode":404,"errorCode":"workflowNotFoundById","userMessage":"not found"}`))
}

// notFoundHandler adapts writeNotFound to the definitionByID signature.
func notFoundHandler(w http.ResponseWriter, r *http.Request, _ string) {
	writeNotFound(w, r)
}

// ---- input validation (no HTTP calls should happen) ----

func TestMalformedWorkflowDefinitionIDIsValidationError(t *testing.T) {
	server := newServer(t, handlers{}) // no endpoint should be hit
	defer server.Close()

	out := run(t, server, tools.Input{WorkflowDefinitionID: "not-a-uuid"})
	if out.Status != tools.StatusValidationError {
		t.Fatalf("expected validation_error for a malformed workflowDefinitionId, got %q (%s)", out.Status, out.Message)
	}
}

func TestMalformedBusinessItemIDIsValidationError(t *testing.T) {
	server := newServer(t, handlers{})
	defer server.Close()

	out := run(t, server, tools.Input{BusinessItemID: "not-a-uuid"})
	if out.Status != tools.StatusValidationError {
		t.Fatalf("expected validation_error for a malformed businessItemId, got %q (%s)", out.Status, out.Message)
	}
}

// ---- discovery ----

func TestDiscoveryWithNoInputsListsInstanceWideWorkflows(t *testing.T) {
	server := newServer(t, handlers{
		listDefinitions: func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("assetIds"); got != "" {
				t.Errorf("expected an instance-wide listing with no assetIds, got %q", got)
			}
			jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{
				{"id": businessTermWorkflowID, "name": "Propose New Business Term", "processId": "intakeBusinessTerm", "enabled": true, "formRequired": true, "businessItemResourceType": "GLOBAL"},
			}})(w, r)
		},
	})
	defer server.Close()

	out := run(t, server, tools.Input{})
	if out.Status != tools.StatusOptions {
		t.Fatalf("expected options for an instance-wide discovery call, got %q (%s)", out.Status, out.Message)
	}
	if len(out.Options) != 1 || out.Options[0].WorkflowDefinitionID != businessTermWorkflowID {
		t.Fatalf("expected the one GLOBAL workflow, got %+v", out.Options)
	}
}

func TestDiscoveryWithBusinessItemIDListsApplicableWorkflows(t *testing.T) {
	server := newServer(t, handlers{
		listDefinitions: func(w http.ResponseWriter, r *http.Request) {
			if got := r.URL.Query().Get("assetIds"); got != assetID {
				t.Errorf("expected assetIds=%s, got %q", assetID, got)
			}
			jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{
				{"id": accessRequestWorkflowID, "name": "Request Assets Access", "description": "Use this flow to request access to all assets referenced in this shopping cart.", "processId": "RequestDataSetsAccess", "enabled": true, "formRequired": true, "businessItemResourceType": "ASSET"},
				{"id": "cccccccc-cccc-cccc-cccc-cccccccccccc", "name": "Disabled Custom Flow", "processId": "customDisabled", "enabled": false, "formRequired": false, "businessItemResourceType": "ASSET"},
			}})(w, r)
		},
	})
	defer server.Close()

	out := run(t, server, tools.Input{BusinessItemID: assetID})
	if out.Status != tools.StatusOptions {
		t.Fatalf("expected options, got %q (%s)", out.Status, out.Message)
	}
	if len(out.Options) != 1 || out.Options[0].WorkflowDefinitionID != accessRequestWorkflowID {
		t.Fatalf("expected only the enabled workflow, got %+v", out.Options)
	}
	if out.Options[0].Description != "Use this flow to request access to all assets referenced in this shopping cart." {
		t.Errorf("expected the workflow's description surfaced so the agent can pick correctly, got %q", out.Options[0].Description)
	}
	if !out.Options[0].FormRequired {
		t.Errorf("expected formRequired surfaced")
	}
}

func TestDiscoveryExcludesUserMissingWorkflowStartPermission(t *testing.T) {
	server := newServer(t, handlers{
		listDefinitions: func(w http.ResponseWriter, r *http.Request) {
			jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{
				{"id": businessTermWorkflowID, "name": "Propose New Business Term", "processId": "intakeBusinessTerm", "enabled": true, "formRequired": true, "businessItemResourceType": "GLOBAL"},
			}})(w, r)
		},
		globalPermissions: jsonHandler(http.StatusOK, map[string]any{"globalPermissions": []string{"SOME_OTHER_PERMISSION"}}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{})
	if out.Status != tools.StatusOptions {
		t.Fatalf("expected options (possibly empty), got %q (%s)", out.Status, out.Message)
	}
	if len(out.Options) != 0 {
		t.Fatalf("expected the workflow excluded when the user lacks WORKFLOW_START, got %+v", out.Options)
	}
}

func TestDiscoveryAnnotatesUncertainAuthorizationRatherThanHiding(t *testing.T) {
	server := newServer(t, handlers{
		listDefinitions: func(w http.ResponseWriter, r *http.Request) {
			// Has WORKFLOW_START (default mock), but the definition has real startRoles and is
			// not registeredUserAccessible, and the user holds none of them directly — per the
			// documented inheritance blind spot this must be kept and annotated, not dropped.
			jsonHandler(http.StatusOK, map[string]any{"results": []map[string]any{
				{"id": businessTermWorkflowID, "name": "Custom Approval", "processId": "customApproval", "enabled": true, "formRequired": false,
					"businessItemResourceType": "GLOBAL", "registeredUserAccessible": false,
					"startRoles": []map[string]any{{"id": "role-1", "name": "Business Steward"}}},
			}})(w, r)
		},
	})
	defer server.Close()

	out := run(t, server, tools.Input{})
	if out.Status != tools.StatusOptions || len(out.Options) != 1 {
		t.Fatalf("expected the uncertain workflow kept, got %q %+v (%s)", out.Status, out.Options, out.Message)
	}
	if out.Options[0].AuthorizationNote == "" {
		t.Errorf("expected a non-empty authorizationNote for an unconfirmed-but-not-denied case")
	}
}

// ---- resolving a specific workflow ----

func TestUnknownWorkflowIDIsValidationError(t *testing.T) {
	server := newServer(t, handlers{definitionByID: notFoundHandler})
	defer server.Close()

	out := run(t, server, tools.Input{WorkflowDefinitionID: accessRequestWorkflowID})
	if out.Status != tools.StatusValidationError {
		t.Fatalf("expected validation_error when the workflow doesn't exist, got %q (%s)", out.Status, out.Message)
	}
}

func TestDisabledWorkflowIsValidationError(t *testing.T) {
	server := newServer(t, handlers{
		definitionByID: func(w http.ResponseWriter, r *http.Request, _ string) {
			jsonHandler(http.StatusOK, map[string]any{
				"id": businessTermWorkflowID, "name": "Custom Intake Flow", "processId": "customIntake",
				"enabled": false, "formRequired": false, "businessItemResourceType": "GLOBAL",
			})(w, r)
		},
	})
	defer server.Close()

	out := run(t, server, tools.Input{WorkflowDefinitionID: businessTermWorkflowID})
	if out.Status != tools.StatusValidationError {
		t.Fatalf("expected validation_error for a disabled workflow, got %q (%s)", out.Status, out.Message)
	}
}

func TestDirectStartExcludesUserMissingWorkflowStartPermission(t *testing.T) {
	server := newServer(t, handlers{
		definitionByID: func(w http.ResponseWriter, r *http.Request, _ string) {
			jsonHandler(http.StatusOK, map[string]any{
				"id": businessTermWorkflowID, "name": "Propose New Business Term", "processId": "intakeBusinessTerm",
				"enabled": true, "formRequired": false, "businessItemResourceType": "GLOBAL",
			})(w, r)
		},
		globalPermissions: jsonHandler(http.StatusOK, map[string]any{"globalPermissions": []string{}}),
		// start is nil -> the test fails if the tool tries to start despite the missing permission.
	})
	defer server.Close()

	out := run(t, server, tools.Input{WorkflowDefinitionID: businessTermWorkflowID, Confirm: true})
	if out.Status != tools.StatusValidationError {
		t.Fatalf("expected validation_error when the user lacks WORKFLOW_START, got %q (%s)", out.Status, out.Message)
	}
}

func TestFormRequiredWorkflowNeedsFormFields(t *testing.T) {
	server := newServer(t, handlers{
		definitionByID: func(w http.ResponseWriter, r *http.Request, _ string) {
			jsonHandler(http.StatusOK, map[string]any{
				"id": accessRequestWorkflowID, "name": "Request Assets Access", "processId": "RequestDataSetsAccess",
				"enabled": true, "formRequired": true, "businessItemResourceType": "ASSET",
			})(w, r)
		},
		startFormData: func(w http.ResponseWriter, r *http.Request, id string) {
			if id != accessRequestWorkflowID {
				t.Errorf("expected start form requested for %s, got %q", accessRequestWorkflowID, id)
			}
			jsonHandler(http.StatusOK, map[string]any{"formProperties": []map[string]any{
				{"id": "usageRequestReason", "name": "Why do you need access?", "type": "textarea", "required": true},
			}})(w, r)
		},
		// start is nil -> the test fails if the tool starts before the form is filled in.
	})
	defer server.Close()

	out := run(t, server, tools.Input{WorkflowDefinitionID: accessRequestWorkflowID, BusinessItemID: assetID, Confirm: true})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input listing the form fields, got %q (%s)", out.Status, out.Message)
	}
	if len(out.FormFields) != 1 || out.FormFields[0].ID != "usageRequestReason" {
		t.Fatalf("expected the form field described, got %+v", out.FormFields)
	}
	if len(out.MissingFields) != 1 || out.MissingFields[0] != "usageRequestReason" {
		t.Fatalf("expected usageRequestReason reported missing, got %v", out.MissingFields)
	}
}

func TestFormRequiredWorkflowStartsOnceFieldsSupplied(t *testing.T) {
	var startedBody map[string]any
	server := newServer(t, handlers{
		definitionByID: func(w http.ResponseWriter, r *http.Request, _ string) {
			jsonHandler(http.StatusOK, map[string]any{
				"id": accessRequestWorkflowID, "name": "Request Assets Access", "processId": "RequestDataSetsAccess",
				"enabled": true, "formRequired": true, "businessItemResourceType": "ASSET",
			})(w, r)
		},
		startFormData: func(w http.ResponseWriter, r *http.Request, _ string) {
			jsonHandler(http.StatusOK, map[string]any{"formProperties": []map[string]any{
				{"id": "usageRequestReason", "name": "Why do you need access?", "type": "textarea", "required": true},
			}})(w, r)
		},
		start: func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&startedBody)
			jsonHandler(http.StatusCreated, []map[string]any{{"id": "instance-3"}})(w, r)
		},
	})
	defer server.Close()

	out := run(t, server, tools.Input{
		WorkflowDefinitionID: accessRequestWorkflowID,
		BusinessItemID:       assetID,
		FormProperties:       map[string]string{"usageRequestReason": "Q3 reporting"},
		Confirm:              true,
	})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("expected success once all required fields are supplied, got %q (%s)", out.Status, out.Message)
	}
	formProps, _ := startedBody["formProperties"].(map[string]any)
	if formProps["usageRequestReason"] != "Q3 reporting" {
		t.Errorf("expected formProperties sent through to the start request, got %v", startedBody["formProperties"])
	}
}

func TestScopedWorkflowWithoutBusinessItemIDNeedsInput(t *testing.T) {
	server := newServer(t, handlers{
		definitionByID: func(w http.ResponseWriter, r *http.Request, _ string) {
			jsonHandler(http.StatusOK, map[string]any{
				"id": "dddddddd-dddd-dddd-dddd-dddddddddddd", "name": "Custom Approval", "processId": "customApproval",
				"enabled": true, "formRequired": false, "businessItemResourceType": "ASSET",
			})(w, r)
		},
	})
	defer server.Close()

	out := run(t, server, tools.Input{WorkflowDefinitionID: "dddddddd-dddd-dddd-dddd-dddddddddddd"})
	if out.Status != tools.StatusNeedsInput {
		t.Fatalf("expected needs_input for a missing businessItemId, got %q (%s)", out.Status, out.Message)
	}
}

// ---- preview / confirm ----

func TestFullySpecifiedDefaultsToPreview(t *testing.T) {
	server := newServer(t, handlers{
		definitionByID: func(w http.ResponseWriter, r *http.Request, _ string) {
			jsonHandler(http.StatusOK, map[string]any{
				"id": businessTermWorkflowID, "name": "Propose New Business Term", "processId": "intakeBusinessTerm",
				"enabled": true, "formRequired": false, "businessItemResourceType": "GLOBAL",
			})(w, r)
		},
		// start is nil -> the test fails if confirm=false still starts the workflow.
	})
	defer server.Close()

	out := run(t, server, tools.Input{WorkflowDefinitionID: businessTermWorkflowID})
	if out.Status != tools.StatusPreview {
		t.Fatalf("expected preview when confirm is omitted, got %q (%s)", out.Status, out.Message)
	}
	if out.Name != "Propose New Business Term" {
		t.Errorf("expected the resolved name echoed, got %q", out.Name)
	}
}

func TestConfirmTrueStartsGlobalWorkflow(t *testing.T) {
	var startedBody map[string]any
	server := newServer(t, handlers{
		definitionByID: func(w http.ResponseWriter, r *http.Request, _ string) {
			jsonHandler(http.StatusOK, map[string]any{
				"id": businessTermWorkflowID, "name": "Propose New Business Term", "processId": "intakeBusinessTerm",
				"enabled": true, "formRequired": false, "businessItemResourceType": "GLOBAL",
			})(w, r)
		},
		start: func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&startedBody)
			jsonHandler(http.StatusCreated, []map[string]any{{"id": "instance-1"}})(w, r)
		},
	})
	defer server.Close()

	out := run(t, server, tools.Input{WorkflowDefinitionID: businessTermWorkflowID, Confirm: true})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("expected success, got %q (%s)", out.Status, out.Message)
	}
	if out.WorkflowInstanceID != "instance-1" {
		t.Errorf("expected the started instance id surfaced, got %q", out.WorkflowInstanceID)
	}
	if startedBody["workflowDefinitionId"] != businessTermWorkflowID {
		t.Errorf("expected the resolved definition id sent to start, got %v", startedBody["workflowDefinitionId"])
	}
	if _, hasItems := startedBody["businessItemIds"]; hasItems {
		t.Errorf("expected no businessItemIds sent for a GLOBAL workflow, got %v", startedBody["businessItemIds"])
	}
}

func TestConfirmTrueStartsAssetScopedWorkflow(t *testing.T) {
	var startedBody map[string]any
	server := newServer(t, handlers{
		definitionByID: func(w http.ResponseWriter, r *http.Request, _ string) {
			jsonHandler(http.StatusOK, map[string]any{
				"id": accessRequestWorkflowID, "name": "Request Assets Access", "processId": "RequestDataSetsAccess",
				"enabled": true, "formRequired": false, "businessItemResourceType": "ASSET",
			})(w, r)
		},
		start: func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&startedBody)
			jsonHandler(http.StatusCreated, []map[string]any{{"id": "instance-2"}})(w, r)
		},
	})
	defer server.Close()

	out := run(t, server, tools.Input{WorkflowDefinitionID: accessRequestWorkflowID, BusinessItemID: assetID, Confirm: true})
	if out.Status != tools.StatusSuccess {
		t.Fatalf("expected success, got %q (%s)", out.Status, out.Message)
	}
	if startedBody["businessItemType"] != "ASSET" {
		t.Errorf("expected businessItemType ASSET sent, got %v", startedBody["businessItemType"])
	}
	ids, _ := startedBody["businessItemIds"].([]any)
	if len(ids) != 1 || ids[0] != assetID {
		t.Errorf("expected businessItemIds=[%s], got %v", assetID, startedBody["businessItemIds"])
	}
}

func TestStartPermissionDeniedSurfacesError(t *testing.T) {
	server := newServer(t, handlers{
		definitionByID: func(w http.ResponseWriter, r *http.Request, _ string) {
			jsonHandler(http.StatusOK, map[string]any{
				"id": businessTermWorkflowID, "name": "Propose New Business Term", "processId": "intakeBusinessTerm",
				"enabled": true, "formRequired": false, "businessItemResourceType": "GLOBAL",
			})(w, r)
		},
		start: jsonHandler(http.StatusForbidden, map[string]any{"statusCode": 403, "errorCode": "forbidden", "userMessage": "not allowed"}),
	})
	defer server.Close()

	out := run(t, server, tools.Input{WorkflowDefinitionID: businessTermWorkflowID, Confirm: true})
	if out.Status != tools.StatusError {
		t.Fatalf("expected error on a 403 from start, got %q (%s)", out.Status, out.Message)
	}
	if !strings.Contains(out.Message, "403") {
		t.Errorf("expected the 403 surfaced in the message, got %q", out.Message)
	}
}
