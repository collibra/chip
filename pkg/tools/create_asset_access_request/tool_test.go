package create_asset_access_request_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	tool "github.com/collibra/chip/pkg/tools/create_asset_access_request"
	"github.com/collibra/chip/pkg/tools/testutil"
	"github.com/google/jsonschema-go/jsonschema"
)

// The data access API serves every GraphQL operation from one endpoint, so the fake dispatches
// on the operation name in the request body.
const gqlPath = "/dataAccess/query"

const (
	dataProductID   = "11111111-1111-1111-1111-111111111111"
	outputPortID    = "019d76fb-42c5-7568-8050-eab61c198ec2"
	secondPortID    = "22222222-2222-2222-2222-222222222222"
	tableID         = "33333333-3333-3333-3333-333333333333"
	outputPortRelID = "44444444-4444-4444-4444-444444444444"
	inputPortRelID  = "55555555-5555-5555-5555-555555555555"
	tableTypeID     = "00000000-0000-0000-0000-000000031007"
	outputPortType  = "00000000-0000-0000-0000-000000050004"
	roleID          = "ac-1"
	collibraGroupID = "66666666-6666-6666-6666-666666666666"
	expiry          = "2099-12-31"
)

// fakeCollibra serves the DGC REST resources and the data access GraphQL operations this tool
// touches, and records the create mutation so tests can assert what was sent.
type fakeCollibra struct {
	assets              map[string]string
	relations           map[string]string
	relationTypes       map[string]string
	rolesByAsset        map[string][]string
	groupsByName        map[string][]string
	collibraGroupsByID  map[string]string
	accessControlsError string
	collibraUsersByName map[string]string
	usersByEmail        map[string]string
	createResponse      string
	createRequest       string
}

func newFake() *fakeCollibra {
	return &fakeCollibra{
		assets: map[string]string{
			dataProductID: assetJSON(dataProductID, "Sales Orders", clients.DataProductAssetTypeID, "Data Product"),
			outputPortID:  assetJSON(outputPortID, "Sales Orders — Table", outputPortType, "Data Product Port"),
			secondPortID:  assetJSON(secondPortID, "Sales Orders — API", outputPortType, "Data Product Port"),
			tableID:       assetJSON(tableID, "customer_orders", tableTypeID, "Table"),
		},
		relations: map[string]string{
			dataProductID: relationsJSON(
				relationJSON(outputPortRelID, outputPortID, "Sales Orders — Table"),
				relationJSON(inputPortRelID, tableID, "customer_orders"),
			),
		},
		relationTypes: map[string]string{
			outputPortRelID: relationTypeJSON(outputPortRelID, "DataProductHasOutputPort", "exposes data as"),
			inputPortRelID:  relationTypeJSON(inputPortRelID, "DataProductHasInputPort", "consumes data through"),
		},
		rolesByAsset: map[string][]string{
			outputPortID: {accessControlJSON(roleID, "Sales Orders Consumers", "Grant", "Active")},
		},
		groupsByName: map[string][]string{
			"Finance": {accessControlJSON("group-1", "Finance", "Group", "Active")},
		},
		collibraGroupsByID: map[string]string{
			collibraGroupID: "Finance",
		},
		collibraUsersByName: map[string]string{
			"alice": `{"total":1,"offset":0,"limit":100,"results":[{"id":"dgc-user-1","userName":"alice","emailAddress":"alice@example.com"}]}`,
		},
		usersByEmail: map[string]string{
			"alice@example.com": `{"data":{"userByEmail":{"__typename":"User","id":"da-user-1","name":"Alice","email":"alice@example.com","type":"Human"}}}`,
		},
		createResponse: `{"data":{"createAccessRequest":{"__typename":"AccessRequest","id":"ar-1",` +
			`"name":"Access request: Quarterly revenue reporting","description":"Quarterly revenue reporting. This access request was created by AI.",` +
			`"status":"Created","outcome":"Pending","processingSteps":[]}}}`,
	}
}

func (f *fakeCollibra) server(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/rest/2.0/assets/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/assets/")
		body, ok := f.assets[id]
		if !ok {
			http.Error(w, `{"statusCode":404}`, http.StatusNotFound)
			return
		}
		writeJSON(w, body)
	})

	mux.HandleFunc("/rest/2.0/assetTypes", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"total":2,"offset":0,"limit":1000,"results":[`+
			`{"id":"`+outputPortType+`","publicId":"DataProductPort","name":"Data Product Port"},`+
			`{"id":"`+tableTypeID+`","publicId":"Table","name":"Table"}]}`)
	})

	mux.HandleFunc("/rest/2.0/userGroups/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/userGroups/")
		name, ok := f.collibraGroupsByID[id]
		if !ok {
			http.Error(w, `{"statusCode":404}`, http.StatusNotFound)
			return
		}
		writeJSON(w, fmt.Sprintf(`{"id":%q,"name":%q}`, id, name))
	})

	mux.HandleFunc("/rest/2.0/users", func(w http.ResponseWriter, r *http.Request) {
		body, ok := f.collibraUsersByName[r.URL.Query().Get("name")]
		if !ok {
			body = `{"total":0,"offset":0,"limit":100,"results":[]}`
		}
		writeJSON(w, body)
	})

	mux.HandleFunc("/rest/2.0/relations", func(w http.ResponseWriter, r *http.Request) {
		body, ok := f.relations[r.URL.Query().Get("sourceId")]
		if !ok {
			body = relationsJSON()
		}
		writeJSON(w, body)
	})

	mux.HandleFunc("/rest/2.0/relationTypes/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/relationTypes/")
		body, ok := f.relationTypes[id]
		if !ok {
			http.Error(w, `{"statusCode":404}`, http.StatusNotFound)
			return
		}
		writeJSON(w, body)
	})

	mux.HandleFunc(gqlPath, func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		query := string(raw)
		switch {
		case strings.Contains(query, "query ListAccessControls"):
			writeJSON(w, f.accessControlsResponse(query))
		case strings.Contains(query, "query GetUserByEmail"):
			writeJSON(w, f.userResponse(query))
		case strings.Contains(query, "mutation CreateAccessRequest"):
			f.createRequest = query
			writeJSON(w, f.createResponse)
		default:
			http.Error(w, "unexpected query: "+query, http.StatusBadRequest)
		}
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

// accessControlsResponse answers the asset id embedded in the assetIds filter, so each asset
// can carry its own roles — or none.
func (f *fakeCollibra) accessControlsResponse(query string) string {
	if f.accessControlsError != "" {
		return fmt.Sprintf(`{"data":{"accessControls":{"__typename":"PermissionDeniedError","message":%q}}}`, f.accessControlsError)
	}
	nodes := []string{}
	if strings.Contains(query, `"Group"`) {
		for name, group := range f.groupsByName {
			if strings.Contains(query, name) {
				nodes = group
				break
			}
		}
	} else {
		for assetID, roles := range f.rolesByAsset {
			if strings.Contains(query, assetID) {
				nodes = roles
				break
			}
		}
	}
	edges := make([]string, 0, len(nodes))
	for i, node := range nodes {
		edges = append(edges, fmt.Sprintf(`{"cursor":"c%d","node":%s}`, i, node))
	}
	return `{"data":{"accessControls":{"__typename":"AccessControlConnection",` +
		`"pageInfo":{"hasNextPage":false,"startCursor":null},` +
		`"edges":[` + strings.Join(edges, ",") + `]}}}`
}

// userResponse answers the email embedded in the query variables, so an unknown address
// produces a NotFoundError exactly as the API would.
func (f *fakeCollibra) userResponse(query string) string {
	for email, response := range f.usersByEmail {
		if strings.Contains(query, email) {
			return response
		}
	}
	return `{"data":{"userByEmail":{"__typename":"NotFoundError","message":"No user found for the given email address."}}}`
}

func (f *fakeCollibra) run(t *testing.T, input tool.Input) tool.Output {
	t.Helper()
	ctx := chip.SetCollibraHost(t.Context(), "http://collibra.test")
	output, err := tool.NewTool(testutil.NewClient(f.server(t))).Handler(ctx, input)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	return output
}

func validInput() tool.Input {
	return tool.Input{
		AssetID:   outputPortID,
		Users:     []string{"alice@example.com"},
		Purpose:   "Quarterly revenue reporting",
		ExpiresAt: expiry,
		Name:      "Access request: Quarterly revenue reporting",
	}
}

func TestCreateFromOutputPort(t *testing.T) {
	fake := newFake()
	output := fake.run(t, validInput())

	if output.Error != "" {
		t.Fatalf("Expected no tool error, got: %q", output.Error)
	}
	if output.Status != "created" {
		t.Fatalf("Expected status created, got: %q (%s)", output.Status, output.Message)
	}
	if output.Request == nil || output.Request.ID != "ar-1" {
		t.Fatalf("Expected the created request, got: %+v", output.Request)
	}
	if output.Role == nil || output.Role.ID != roleID {
		t.Fatalf("Expected role %s, got: %+v", roleID, output.Role)
	}
	if output.Asset == nil || output.Asset.ID != outputPortID {
		t.Fatalf("Expected output port %s, got: %+v", outputPortID, output.Asset)
	}
	if len(output.Users) != 1 || output.Users[0].ID != "da-user-1" {
		t.Fatalf("Expected the mapped data access user, got: %+v", output.Users)
	}

	sent := fake.createRequest
	for _, want := range []string{
		`"what":[{"accessControl":{"id":"ac-1"}}]`,
		`"catalogAsset":{"assetId":"` + outputPortID + `","assetTypeId":"` + outputPortType + `"}`,
		`"who":{"users":["da-user-1"]}`,
		`"implementationExpiresAt":"2099-12-31T23:59:59Z"`,
		"This access request was created by AI.",
	} {
		if !strings.Contains(sent, want) {
			t.Fatalf("Expected the create mutation to contain %s, got: %s", want, sent)
		}
	}
	if strings.Contains(sent, `"dataObject"`) {
		t.Fatalf("Expected no data object in the WHAT, got: %s", sent)
	}
}

func TestDataProductWithSeveralPortsAsksForSelection(t *testing.T) {
	fake := newFake()
	fake.relations[dataProductID] = relationsJSON(
		relationJSON(outputPortRelID, outputPortID, "Sales Orders — Table"),
		relationJSON(outputPortRelID, secondPortID, "Sales Orders — API"),
	)

	input := validInput()
	input.AssetID = dataProductID
	output := fake.run(t, input)

	if output.Status != "needs_port_selection" {
		t.Fatalf("Expected needs_port_selection, got: %q (%s)", output.Status, output.Error)
	}
	if len(output.OutputPorts) != 2 {
		t.Fatalf("Expected 2 candidate ports, got: %+v", output.OutputPorts)
	}
	if fake.createRequest != "" {
		t.Fatal("Expected nothing to be created while a port is still to be chosen")
	}
}

func TestDataProductWithOnePortSelectsIt(t *testing.T) {
	fake := newFake()

	input := validInput()
	input.AssetID = dataProductID
	output := fake.run(t, input)

	if output.Status != "created" {
		t.Fatalf("Expected status created, got: %q (%s)", output.Status, output.Error)
	}
	if output.Asset == nil || output.Asset.ID != outputPortID {
		t.Fatalf("Expected the product's only output port, got: %+v", output.Asset)
	}
}

func TestDataProductWithItsOwnRoleIsRequestedDirectly(t *testing.T) {
	fake := newFake()
	fake.rolesByAsset[dataProductID] = []string{accessControlJSON("ac-dp", "Sales Orders Consumers", "Grant", "Active")}
	fake.relations[dataProductID] = relationsJSON(
		relationJSON(outputPortRelID, outputPortID, "Sales Orders — Table"),
		relationJSON(outputPortRelID, secondPortID, "Sales Orders — API"),
	)

	input := validInput()
	input.AssetID = dataProductID
	output := fake.run(t, input)

	if output.Status != "created" {
		t.Fatalf("Expected status created, got: %q (%s)", output.Status, output.Error)
	}
	if output.Asset == nil || output.Asset.ID != dataProductID {
		t.Fatalf("Expected the data product itself to be requested, got: %+v", output.Asset)
	}
	if !strings.Contains(fake.createRequest, `"catalogAsset":{"assetId":"`+dataProductID+`","assetTypeId":"`+clients.DataProductAssetTypeID+`"}`) {
		t.Fatalf("Expected the data product to be the catalog asset, got: %s", fake.createRequest)
	}
}

func TestNamingAPortOverridesADataProductWithItsOwnRole(t *testing.T) {
	fake := newFake()
	fake.rolesByAsset[dataProductID] = []string{accessControlJSON("ac-dp", "Sales Orders Consumers", "Grant", "Active")}

	input := validInput()
	input.AssetID = dataProductID
	input.OutputPortID = outputPortID
	output := fake.run(t, input)

	if output.Status != "created" {
		t.Fatalf("Expected status created, got: %q (%s)", output.Status, output.Error)
	}
	if output.Asset == nil || output.Asset.ID != outputPortID {
		t.Fatalf("Expected the named port to win over the product, got: %+v", output.Asset)
	}
}

func TestPortOfAnotherDataProductIsRejected(t *testing.T) {
	fake := newFake()

	input := validInput()
	input.AssetID = dataProductID
	input.OutputPortID = secondPortID
	output := fake.run(t, input)

	if !strings.Contains(output.Error, "not an output port of data product") {
		t.Fatalf("Expected the port to be rejected, got: %q", output.Error)
	}
	if fake.createRequest != "" {
		t.Fatal("Expected nothing to be created")
	}
}

func TestAnyAssetWithALinkedRoleCanBeRequested(t *testing.T) {
	fake := newFake()
	fake.rolesByAsset[tableID] = []string{accessControlJSON("ac-table", "Customer Orders Readers", "Grant", "Active")}

	input := validInput()
	input.AssetID = tableID
	output := fake.run(t, input)

	if output.Status != "created" {
		t.Fatalf("Expected status created, got: %q (%s)", output.Status, output.Error)
	}
	if output.Asset == nil || output.Asset.ID != tableID {
		t.Fatalf("Expected the table to be the requested asset, got: %+v", output.Asset)
	}
	if !strings.Contains(fake.createRequest, `"catalogAsset":{"assetId":"`+tableID+`","assetTypeId":"`+tableTypeID+`"}`) {
		t.Fatalf("Expected the table to be the catalog asset, got: %s", fake.createRequest)
	}
}

func TestAssetWithoutALinkedRole(t *testing.T) {
	fake := newFake()

	input := validInput()
	input.AssetID = tableID
	output := fake.run(t, input)

	if output.Status != "no_role_linked" {
		t.Fatalf("Expected status no_role_linked, got: %q (%s)", output.Status, output.Error)
	}
	if output.Error != "" {
		t.Fatalf("Expected the status to carry the outcome, not an error, got: %q", output.Error)
	}
	if output.Asset == nil || output.Asset.ID != tableID {
		t.Fatalf("Expected the rejected asset to be reported, got: %+v", output.Asset)
	}
	if len(output.LinkedRoles) != 0 {
		t.Fatalf("Expected no linked roles, got: %+v", output.LinkedRoles)
	}
	if !strings.Contains(output.Message, "has no Data Access role linked") {
		t.Fatalf("Expected the message to explain the outcome, got: %q", output.Message)
	}
	if fake.createRequest != "" {
		t.Fatal("Expected nothing to be created")
	}
}

func TestOutputPortIdOnlyAppliesToADataProduct(t *testing.T) {
	fake := newFake()
	fake.rolesByAsset[tableID] = []string{accessControlJSON("ac-table", "Customer Orders Readers", "Grant", "Active")}

	input := validInput()
	input.AssetID = tableID
	input.OutputPortID = outputPortID
	output := fake.run(t, input)

	if !strings.Contains(output.Error, "outputPortId only applies when assetId is a Data Product") {
		t.Fatalf("Expected outputPortId to be rejected, got: %q", output.Error)
	}
}

func TestUnknownAsset(t *testing.T) {
	fake := newFake()

	input := validInput()
	input.AssetID = "99999999-9999-9999-9999-999999999999"
	output := fake.run(t, input)

	if !strings.Contains(output.Error, "no asset found") {
		t.Fatalf("Expected a not-found error, got: %q", output.Error)
	}
}

func TestPortWithoutRole(t *testing.T) {
	fake := newFake()
	delete(fake.rolesByAsset, outputPortID)

	output := fake.run(t, validInput())

	if output.Status != "no_role_linked" {
		t.Fatalf("Expected status no_role_linked, got: %q (%s)", output.Status, output.Error)
	}
	if fake.createRequest != "" {
		t.Fatal("Expected nothing to be created")
	}
}

func TestRoleLookupPermissionDenied(t *testing.T) {
	fake := newFake()
	fake.accessControlsError = "You are not allowed to list access controls."

	output := fake.run(t, validInput())

	if !strings.Contains(output.Error, "not allowed") {
		t.Fatalf("Expected the permission error to surface, got: %q", output.Error)
	}
	if output.Status != "" {
		t.Fatalf("Expected a failure, not a status, got: %q", output.Status)
	}
}

func TestInactiveRoleIsNotRequestable(t *testing.T) {
	fake := newFake()
	fake.rolesByAsset[outputPortID] = []string{accessControlJSON(roleID, "Sales Orders Consumers", "Grant", "Inactive")}

	output := fake.run(t, validInput())

	if output.Status != "no_role_linked" {
		t.Fatalf("Expected status no_role_linked, got: %q (%s)", output.Status, output.Error)
	}
	if len(output.LinkedRoles) != 1 || output.LinkedRoles[0].State != "Inactive" {
		t.Fatalf("Expected the inactive role to be reported, got: %+v", output.LinkedRoles)
	}
	if !strings.Contains(output.Message, "none of them is an active Grant") {
		t.Fatalf("Expected the message to explain why, got: %q", output.Message)
	}
	if fake.createRequest != "" {
		t.Fatal("Expected nothing to be created")
	}
}

func TestNonGrantAccessControlIsNotRequestable(t *testing.T) {
	fake := newFake()
	fake.rolesByAsset[outputPortID] = []string{accessControlJSON(roleID, "Salary mask", "Mask", "Active")}

	output := fake.run(t, validInput())

	if output.Status != "no_role_linked" {
		t.Fatalf("Expected status no_role_linked, got: %q (%s)", output.Status, output.Error)
	}
	if !strings.Contains(output.Message, "Mask") {
		t.Fatalf("Expected the message to name the unusable access control, got: %q", output.Message)
	}
}

func TestActiveGrantIsPickedFromSeveralLinkedControls(t *testing.T) {
	fake := newFake()
	fake.rolesByAsset[outputPortID] = []string{
		accessControlJSON("ac-mask", "Salary mask", "Mask", "Active"),
		accessControlJSON(roleID, "Sales Orders Consumers", "Grant", "Active"),
	}

	output := fake.run(t, validInput())

	if output.Status != "created" {
		t.Fatalf("Expected status created, got: %q (%s)", output.Status, output.Error)
	}
	if output.Role == nil || output.Role.ID != roleID {
		t.Fatalf("Expected the active grant to be used, got: %+v", output.Role)
	}
}

func TestUserThatDoesNotMapToDataAccess(t *testing.T) {
	fake := newFake()

	input := validInput()
	input.Users = []string{"alice@example.com", "bob@example.com"}
	output := fake.run(t, input)

	if len(output.UnresolvedUsers) != 1 || output.UnresolvedUsers[0].Input != "bob@example.com" {
		t.Fatalf("Expected bob to be unresolved, got: %+v", output.UnresolvedUsers)
	}
	if fake.createRequest != "" {
		t.Fatal("Expected nothing to be created while a user is unresolved")
	}
}

func TestCollibraUsernameIsMappedByEmail(t *testing.T) {
	fake := newFake()

	input := validInput()
	input.Users = []string{"alice"}
	output := fake.run(t, input)

	if output.Status != "created" {
		t.Fatalf("Expected status created, got: %q (%s)", output.Status, output.Error)
	}
	if len(output.Users) != 1 || output.Users[0].ID != "da-user-1" {
		t.Fatalf("Expected the username to map to the data access user, got: %+v", output.Users)
	}
}

func TestUnknownCollibraUsername(t *testing.T) {
	fake := newFake()

	input := validInput()
	input.Users = []string{"nobody"}
	output := fake.run(t, input)

	if len(output.UnresolvedUsers) != 1 || !strings.Contains(output.UnresolvedUsers[0].Reason, "no Collibra user with username") {
		t.Fatalf("Expected the username to be unresolved, got: %+v", output.UnresolvedUsers)
	}
	if fake.createRequest != "" {
		t.Fatal("Expected nothing to be created")
	}
}

func TestDuplicateUsersAreCollapsed(t *testing.T) {
	fake := newFake()

	input := validInput()
	input.Users = []string{"alice", "alice@example.com"}
	output := fake.run(t, input)

	if output.Status != "created" {
		t.Fatalf("Expected status created, got: %q (%s)", output.Status, output.Error)
	}
	if len(output.Users) != 1 {
		t.Fatalf("Expected one data access user, got: %+v", output.Users)
	}
}

func TestGroupByName(t *testing.T) {
	fake := newFake()

	input := validInput()
	input.Users = nil
	input.Groups = []string{"Finance"}
	output := fake.run(t, input)

	if output.Status != "created" {
		t.Fatalf("Expected status created, got: %q (%s)", output.Status, output.Error)
	}
	if len(output.Groups) != 1 || output.Groups[0].ID != "group-1" {
		t.Fatalf("Expected the group to be mapped, got: %+v", output.Groups)
	}
	if !strings.Contains(fake.createRequest, `"who":{"accessControls":["group-1"]}`) {
		t.Fatalf("Expected the group to be the WHO, got: %s", fake.createRequest)
	}
}

func TestGroupByCollibraID(t *testing.T) {
	fake := newFake()

	input := validInput()
	input.Users = nil
	input.Groups = []string{collibraGroupID}
	output := fake.run(t, input)

	if output.Status != "created" {
		t.Fatalf("Expected status created, got: %q (%s)", output.Status, output.Error)
	}
	if len(output.Groups) != 1 || output.Groups[0].Name != "Finance" {
		t.Fatalf("Expected the Collibra group id to resolve by name, got: %+v", output.Groups)
	}
}

func TestUsersAndGroupsTogether(t *testing.T) {
	fake := newFake()

	input := validInput()
	input.Groups = []string{"Finance"}
	output := fake.run(t, input)

	if output.Status != "created" {
		t.Fatalf("Expected status created, got: %q (%s)", output.Status, output.Error)
	}
	if !strings.Contains(fake.createRequest, `"who":{"users":["da-user-1"],"accessControls":["group-1"]}`) {
		t.Fatalf("Expected both beneficiaries in the WHO, got: %s", fake.createRequest)
	}
}

func TestGroupThatDoesNotMapToDataAccess(t *testing.T) {
	fake := newFake()

	input := validInput()
	input.Groups = []string{"Marketing"}
	output := fake.run(t, input)

	if len(output.UnresolvedGroups) != 1 || output.UnresolvedGroups[0].Input != "Marketing" {
		t.Fatalf("Expected the group to be unresolved, got: %+v", output.UnresolvedGroups)
	}
	if fake.createRequest != "" {
		t.Fatal("Expected nothing to be created while a group is unresolved")
	}
}

func TestAmbiguousGroupName(t *testing.T) {
	fake := newFake()
	fake.groupsByName["Finance"] = []string{
		accessControlJSON("group-1", "Finance", "Group", "Active"),
		accessControlJSON("group-2", "Finance", "Group", "Active"),
	}

	input := validInput()
	input.Groups = []string{"Finance"}
	output := fake.run(t, input)

	if len(output.UnresolvedGroups) != 1 || !strings.Contains(output.UnresolvedGroups[0].Reason, "several Data Access groups") {
		t.Fatalf("Expected the ambiguity to be reported, got: %+v", output.UnresolvedGroups)
	}
	if fake.createRequest != "" {
		t.Fatal("Expected nothing to be created")
	}
}

func TestPurposeIsMandatory(t *testing.T) {
	for name, purpose := range map[string]string{"empty": "", "blank": "   "} {
		t.Run(name, func(t *testing.T) {
			fake := newFake()
			input := validInput()
			input.Purpose = purpose
			output := fake.run(t, input)

			if !strings.Contains(output.Error, "purpose is required") {
				t.Fatalf("Expected the missing purpose to be reported, got: %q", output.Error)
			}
			if fake.createRequest != "" {
				t.Fatal("Expected nothing to be created without a purpose")
			}
		})
	}
}

func TestPurposeIsRequiredInTheInputSchema(t *testing.T) {
	schema, err := jsonschema.For[tool.Input](nil)
	if err != nil {
		t.Fatalf("Expected a schema, got: %v", err)
	}
	if !slices.Contains(schema.Required, "purpose") {
		t.Fatalf("Expected purpose to be required, got: %v", schema.Required)
	}
}

func TestNoBeneficiaries(t *testing.T) {
	fake := newFake()

	input := validInput()
	input.Users = nil
	output := fake.run(t, input)

	if !strings.Contains(output.Error, "at least one beneficiary is required") {
		t.Fatalf("Expected the missing beneficiaries to be reported, got: %q", output.Error)
	}
}

func TestMissingNameIsSuggested(t *testing.T) {
	fake := newFake()

	input := validInput()
	input.Name = ""
	output := fake.run(t, input)

	if output.Status != "needs_name_confirmation" {
		t.Fatalf("Expected needs_name_confirmation, got: %q (%s)", output.Status, output.Error)
	}
	if output.SuggestedName != "Access request: Quarterly revenue reporting" {
		t.Fatalf("Unexpected suggestion: %q", output.SuggestedName)
	}
	if fake.createRequest != "" {
		t.Fatal("Expected nothing to be created before the name is confirmed")
	}
}

func TestExpiresAtIsRequiredAndMustBeFuture(t *testing.T) {
	tests := map[string]struct {
		value string
		want  string
	}{
		"empty":       {value: "", want: "expiresAt is required"},
		"unparseable": {value: "next friday", want: "not a valid date"},
		"past":        {value: "2001-01-01", want: "in the past"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fake := newFake()
			input := validInput()
			input.ExpiresAt = test.value
			output := fake.run(t, input)

			if !strings.Contains(output.Error, test.want) {
				t.Fatalf("Expected an error containing %q, got: %q", test.want, output.Error)
			}
			if fake.createRequest != "" {
				t.Fatal("Expected nothing to be created")
			}
		})
	}
}

func TestExpiresAtAcceptsTimestamp(t *testing.T) {
	fake := newFake()

	input := validInput()
	input.ExpiresAt = "2099-06-30T17:00:00Z"
	output := fake.run(t, input)

	if output.Status != "created" {
		t.Fatalf("Expected status created, got: %q (%s)", output.Status, output.Error)
	}
	if output.ExpiresAt != "2099-06-30T17:00:00Z" {
		t.Fatalf("Unexpected expiry: %q", output.ExpiresAt)
	}
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, body)
}

func assetJSON(id, name, typeID, typeName string) string {
	return fmt.Sprintf(`{"id":%q,"name":%q,"displayName":%q,"type":{"id":%q,"name":%q}}`, id, name, name, typeID, typeName)
}

func relationsJSON(relations ...string) string {
	return fmt.Sprintf(`{"total":%d,"offset":0,"limit":1000,"results":[%s]}`, len(relations), strings.Join(relations, ","))
}

func relationJSON(typeID, targetID, targetName string) string {
	return fmt.Sprintf(`{"id":"rel-%s","type":{"id":%q},"target":{"id":%q,"name":%q,"displayName":%q}}`,
		targetID, typeID, targetID, targetName, targetName)
}

func relationTypeJSON(id, publicID, role string) string {
	return fmt.Sprintf(`{"id":%q,"publicId":%q,"role":%q,"coRole":""}`, id, publicID, role)
}

// accessControlJSON is one node of a ListAccessControls page.
func accessControlJSON(id, name, action, state string) string {
	return fmt.Sprintf(`{"id":%q,"name":%q,"action":%q,"state":%q,"description":"","external":false,`+
		`"notInternalizable":false,"whatUnknown":false,"whoUnknown":false,`+
		`"category":{"id":"cat-1","name":"Read","namePlural":"Reads","isSystem":true,"isDefault":true}}`,
		id, name, action, state)
}
