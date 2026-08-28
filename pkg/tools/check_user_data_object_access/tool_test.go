package check_user_data_object_access_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	tool "github.com/collibra/chip/pkg/tools/check_user_data_object_access"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// The data access SDK posts all queries to the same GraphQL endpoint (<host>/dataAccess/query).
// Tests dispatch on the operation name embedded in the query body.
const gqlPath = "/dataAccess/query"

const currentUserResp = `{"data":{"currentUser":{"id":"user-1","name":"Alice","email":"alice@example.com","type":"Human"}}}`

const accessResp = `{"data":{"dataObject":{"distinctAccess":{"__typename":"GroupedDataAccessReturnItemConnection",` +
	`"pageInfo":{"hasNextPage":false,"startCursor":null},` +
	`"edges":[{"cursor":"a1","node":{"permissions":["SELECT"],"globalPermissions":["READ"],"expiresAt":null,` +
	`"user":{"id":"user-1","name":"Alice","email":"alice@example.com","type":"Human"},` +
	`"nearestAccessControls":[{"id":"ac-1","name":"Analysts","action":"Grant","state":"Active",` +
	`"category":{"id":"cat-1","name":"Read","namePlural":"Reads","isSystem":true,"isDefault":true}}]}}]}}}}`

func dataObjectResp(id, name, fullName string) string {
	return `{"data":{"dataObject":{"id":"` + id + `","name":"` + name + `","fullName":"` + fullName +
		`","type":"table","dataType":null,"deleted":false,"description":"","dataSource":{"id":"ds-1"},"applicablePermissions":[]}}}`
}

// emptyDataObjectResp mimics the GraphQL response for an ID that matches no data object: the
// dataObject field is null, which genqlient decodes to a zero-value object (empty id).
const emptyDataObjectResp = `{"data":{"dataObject":null}}`

// newGQLServer returns a test server that answers the three operations the tool relies on. The
// GetDataObject response is supplied by the caller so each test can control ID resolution.
func newGQLServer(t *testing.T, getObjectResp string) *httptest.Server {
	t.Helper()
	handler := http.NewServeMux()
	handler.HandleFunc(gqlPath, func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		q := string(body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(q, "query CurrentUser"):
			_, _ = io.WriteString(w, currentUserResp)
		case strings.Contains(q, "query GetDataObjectAccessList"):
			_, _ = io.WriteString(w, accessResp)
		case strings.Contains(q, "query GetDataObject"):
			_, _ = io.WriteString(w, getObjectResp)
		default:
			http.Error(w, "unexpected query: "+q, http.StatusBadRequest)
		}
	})
	return httptest.NewServer(handler)
}

func TestCheckUserDataObjectAccess_HasAccess(t *testing.T) {
	server := newGQLServer(t, dataObjectResp("do-1", "customers", "db.public.customers"))
	defer server.Close()

	ctx := chip.SetCollibraHost(t.Context(), "http://collibra.test")
	output, err := tool.NewTool(testutil.NewClient(server)).Handler(ctx, tool.Input{
		ObjectIds: []string{"do-1"},
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if output.Error != "" {
		t.Fatalf("Expected no tool error, got: %q", output.Error)
	}
	if output.Message != "" {
		t.Fatalf("Expected no message, got: %q", output.Message)
	}
	if output.Result == nil {
		t.Fatal("Expected a result")
	}
	if output.Result.User == nil || output.Result.User.ID != "user-1" {
		t.Fatalf("Expected user-1, got: %+v", output.Result.User)
	}
	if len(output.Result.Unresolved) != 0 {
		t.Fatalf("Expected no unresolved names, got: %+v", output.Result.Unresolved)
	}
	if len(output.Result.Results) != 1 {
		t.Fatalf("Expected 1 result, got: %d", len(output.Result.Results))
	}

	res := output.Result.Results[0]
	if res.DataObject == nil || res.DataObject.ID != "do-1" {
		t.Fatalf("Unexpected resolved object: %+v", res.DataObject)
	}
	if res.Access == nil || !res.Access.HasAccess {
		t.Fatalf("Expected hasAccess true, got: %+v", res.Access)
	}
	if len(res.Access.Permissions) != 1 || res.Access.Permissions[0] != "SELECT" {
		t.Fatalf("Expected permissions [SELECT], got: %v", res.Access.Permissions)
	}
	if len(res.Access.GlobalPermissions) != 1 || res.Access.GlobalPermissions[0] != "READ" {
		t.Fatalf("Expected globalPermissions [READ], got: %v", res.Access.GlobalPermissions)
	}
	if len(res.Access.Roles) != 1 {
		t.Fatalf("Expected 1 role, got: %d", len(res.Access.Roles))
	}
	role := res.Access.Roles[0]
	if role.ID != "ac-1" || role.Name != "Analysts" || role.Action != "Grant" || role.State != "Active" {
		t.Fatalf("Unexpected role: %+v", role)
	}
	if role.Category == nil || role.Category.Name != "Read" {
		t.Fatalf("Expected role category Read, got: %+v", role.Category)
	}
}

func TestCheckUserDataObjectAccess_UnresolvedNotFound(t *testing.T) {
	server := newGQLServer(t, emptyDataObjectResp)
	defer server.Close()

	ctx := chip.SetCollibraHost(t.Context(), "http://collibra.test")
	output, err := tool.NewTool(testutil.NewClient(server)).Handler(ctx, tool.Input{
		ObjectIds: []string{"ghost"},
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if output.Result == nil {
		t.Fatal("Expected a result")
	}
	if len(output.Result.Results) != 0 {
		t.Fatalf("Expected no resolved results, got: %d", len(output.Result.Results))
	}
	if len(output.Result.Unresolved) != 1 {
		t.Fatalf("Expected 1 unresolved ID, got: %+v", output.Result.Unresolved)
	}
	if output.Result.Unresolved[0].ID != "ghost" || output.Result.Unresolved[0].Reason != "not_found" {
		t.Fatalf("Unexpected unresolved entry: %+v", output.Result.Unresolved[0])
	}
	if output.Message == "" {
		t.Fatal("Expected a message asking the user to correct or drop the ID")
	}
}

func TestCheckUserDataObjectAccess_RequiresIDs(t *testing.T) {
	output, err := tool.NewTool(nil).Handler(t.Context(), tool.Input{ObjectIds: []string{"  "}})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if output.Error == "" {
		t.Fatal("Expected an error when no object IDs are supplied")
	}
}
