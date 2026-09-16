package get_data_access_control_details_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	tool "github.com/collibra/chip/pkg/tools/get_data_access_control_details"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// The data access SDK posts all queries to the same GraphQL endpoint (<host>/dataAccess/query).
// Tests dispatch on the operation name embedded in the query body, and on the requested id for
// the operations that are used for both the access control itself and its group owners.
const gqlPath = "/dataAccess/query"

const accessControlResp = `{"data":{"accessControl":{"__typename":"AccessControl",` +
	`"id":"ac-1","name":"Analysts","description":"Read access for analysts","state":"ACTIVE","action":"GRANT",` +
	`"createdAt":"2024-01-01T00:00:00Z","modifiedAt":"2024-02-01T00:00:00Z","category":null,"namingHint":null,` +
	`"policyRule":null,"external":false,"whatAbacRules":[],"whoAbacRules":[],"notInternalizable":false,` +
	`"complete":null,"locks":[],"syncData":[],"whatUnknown":false,"whoUnknown":false}}}`

const groupResp = `{"data":{"accessControl":{"__typename":"AccessControl",` +
	`"id":"group-1","name":"Data Stewards","description":"","state":"ACTIVE","action":"GROUP",` +
	`"createdAt":"2024-01-01T00:00:00Z","modifiedAt":"2024-02-01T00:00:00Z","category":null,"namingHint":null,` +
	`"policyRule":null,"external":false,"whatAbacRules":[],"whoAbacRules":[],"notInternalizable":false,` +
	`"complete":null,"locks":[],"syncData":[],"whatUnknown":false,"whoUnknown":false}}}`

const emptyWhatResp = `{"data":{"accessControl":{"__typename":"AccessControl","whatAccessControls":` +
	`{"__typename":"AccessWhatAccessControlItemConnection","pageInfo":{"hasNextPage":false,"startCursor":null},"edges":[]}}}}`

const emptyWhoResp = `{"data":{"accessControl":{"__typename":"AccessControl","whoList":` +
	`{"__typename":"AccessWhoItemConnection","pageInfo":{"hasNextPage":false,"startCursor":null},"edges":[]}}}}`

// roleAssignmentsResp holds one user owner, one group owner, and one user owner that cannot be
// resolved (see userNotFoundResp).
const roleAssignmentsResp = `{"data":{"accessControl":{"__typename":"AccessControl","roleAssignments":` +
	`{"__typename":"RoleAssignmentConnection","pageInfo":{"hasNextPage":false,"startCursor":null},"edges":[` +
	`{"cursor":"c1","node":{"id":"ra-1","role":{"id":"OwnerRole","name":"Owner","description":""},` +
	`"on":{"__typename":"AccessControl","id":"ac-1"},"to":{"__typename":"User","id":"u-1"}}},` +
	`{"cursor":"c2","node":{"id":"ra-2","role":{"id":"OwnerRole","name":"Owner","description":""},` +
	`"on":{"__typename":"AccessControl","id":"ac-1"},"to":{"__typename":"AccessControl","id":"group-1"}}},` +
	`{"cursor":"c3","node":{"id":"ra-3","role":{"id":"OwnerRole","name":"Owner","description":""},` +
	`"on":{"__typename":"AccessControl","id":"ac-1"},"to":{"__typename":"User","id":"u-gone"}}}` +
	`]}}}}`

const roleAssignmentsDeniedResp = `{"data":{"accessControl":{"__typename":"PermissionDeniedError",` +
	`"message":"not allowed to read role assignments"}}}`

const userResp = `{"data":{"user":{"__typename":"User","id":"u-1","name":"Ada Lovelace",` +
	`"email":"ada@example.com","type":"HUMAN"}}}`

const userNotFoundResp = `{"data":{"user":{"__typename":"NotFoundError","message":"no such user"}}}`

// newGQLServer returns a test server answering the operations issued by GetDataAccessControl.
// roleAssignmentsBody is the response for the owner role assignment listing.
func newGQLServer(t *testing.T, roleAssignmentsBody string) *httptest.Server {
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
		case strings.Contains(q, "query ListRoleAssignmentsOnAccessControl"):
			_, _ = io.WriteString(w, roleAssignmentsBody)
		case strings.Contains(q, "query GetAccessControlWhatAccessControls"):
			_, _ = io.WriteString(w, emptyWhatResp)
		case strings.Contains(q, "query GetAccessControlWhoList"):
			_, _ = io.WriteString(w, emptyWhoResp)
		case strings.Contains(q, "query GetAccessControl"):
			if strings.Contains(q, `"id":"group-1"`) {
				_, _ = io.WriteString(w, groupResp)
				return
			}
			_, _ = io.WriteString(w, accessControlResp)
		case strings.Contains(q, "query GetUser"):
			if strings.Contains(q, `"id":"u-gone"`) {
				_, _ = io.WriteString(w, userNotFoundResp)
				return
			}
			_, _ = io.WriteString(w, userResp)
		default:
			http.Error(w, "unexpected query: "+q, http.StatusBadRequest)
		}
	})
	return httptest.NewServer(handler)
}

func TestGetDataAccessControlDetails_OwnersIncludeUsersAndGroups(t *testing.T) {
	server := newGQLServer(t, roleAssignmentsResp)
	defer server.Close()

	ctx := chip.SetCollibraHost(t.Context(), "http://collibra.test")
	output, err := tool.NewTool(testutil.NewClient(server)).Handler(ctx, tool.DataAccessControlInput{
		ID: "ac-1",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if !output.Found || output.AccessControl == nil {
		t.Fatalf("Expected the access control to be found, got: %+v", output)
	}

	owners := output.AccessControl.Owners
	// The unresolvable user (u-gone) is skipped, not reported.
	if len(owners) != 2 {
		t.Fatalf("Expected 2 resolved owners, got %d: %+v", len(owners), owners)
	}
	if owners[0].Type != "User" || owners[0].ID != "u-1" || owners[0].Name != "Ada Lovelace" {
		t.Fatalf("Unexpected user owner: %+v", owners[0])
	}
	if owners[0].Email == nil || *owners[0].Email != "ada@example.com" {
		t.Fatalf("Expected the user owner to carry an email, got: %+v", owners[0])
	}
	if owners[1].Type != "Group" || owners[1].ID != "group-1" || owners[1].Name != "Data Stewards" {
		t.Fatalf("Unexpected group owner: %+v", owners[1])
	}
	if owners[1].Email != nil {
		t.Fatalf("Expected no email on the group owner, got: %q", *owners[1].Email)
	}
}

func TestGetDataAccessControlDetails_OwnerLookupFailureKeepsDetails(t *testing.T) {
	server := newGQLServer(t, roleAssignmentsDeniedResp)
	defer server.Close()

	ctx := chip.SetCollibraHost(t.Context(), "http://collibra.test")
	output, err := tool.NewTool(testutil.NewClient(server)).Handler(ctx, tool.DataAccessControlInput{
		ID: "ac-1",
	})
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	if !output.Found || output.AccessControl == nil {
		t.Fatalf("Expected the access control to be found despite the owner lookup failing, got: %+v", output)
	}
	if output.AccessControl.Name != "Analysts" {
		t.Fatalf("Unexpected access control name: %q", output.AccessControl.Name)
	}
	if len(output.AccessControl.Owners) != 0 {
		t.Fatalf("Expected no owners, got: %+v", output.AccessControl.Owners)
	}
}
