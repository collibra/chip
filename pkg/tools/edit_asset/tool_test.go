package edit_asset_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/edit_asset"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	testAssetID      = "018d3602-349b-7d85-8032-3942868ffdc2"
	testAssetTypeID  = "00000000-0000-0000-0000-000000031103"
	testDomainID     = "018d3602-70a4-7ebb-9648-8fd5dc099824"
	testDomainTypeID = "00000000-0000-0000-0000-000000000001"

	defAttrTypeID  = "00000000-0000-0000-0000-000000000202"
	noteAttrTypeID = "00000000-0000-0000-0000-000000000203"
	acrAttrTypeID  = "00000000-0000-0000-0000-000000000204"

	defAttrInstanceID = "7a000000-0000-0000-0000-000000000001"
	noteInstanceAID   = "7a000000-0000-0000-0000-000000000002"
	noteInstanceBID   = "7a000000-0000-0000-0000-000000000003"
	acrInstanceID     = "7a000000-0000-0000-0000-000000000004"

	synonymRelTypeID = "00000000-0000-0000-0000-000000007050"
	targetAssetID    = "018d3602-6f34-73af-8621-2dd8cd39c76d"
	testRelationID   = "8b000000-0000-0000-0000-000000000001"

	stewardRoleID        = "9c000000-0000-0000-0000-000000000001"
	testUserID           = "4d250cc5-e583-4640-9874-b93d82c7a6cb"
	testResponsibilityID = "9d000000-0000-0000-0000-000000000001"

	candidateStatusID = "ae000000-0000-0000-0000-000000000001"
	acceptedStatusID  = "ae000000-0000-0000-0000-000000000002"
)

// stub is a mutable config for the mock server: it lets individual tests
// override attribute instances (e.g. to simulate ambiguous or missing ones)
// and capture what the handler under test wrote to the API.
type stub struct {
	attributes               []clients.EditAssetAttributeInstance
	asset                    *clients.EditAssetCore
	attrTypesByID            map[string]clients.EditAssetAssignmentAttributeType
	relationTypes            []clients.EditAssetAssignmentRelationType
	roles                    []clients.EditAssetRole
	statuses                 []clients.EditAssetStatus
	users                    []clients.EditAssetUser
	patchedAssets            []map[string]any
	patchedAttrs             map[string]string
	createdAttrs             []clients.CreateAttributeRequest
	deletedAttrIDs           []string
	createdRelations         []clients.EditAssetCreateRelationRequest
	deletedRelationIDs       []string
	addedTags                [][]string
	createdResponsibilities  []clients.EditAssetCreateResponsibilityRequest
	bulkCreatedAttrs         [][]clients.CreateAttributeRequest
	bulkPatchedAttrs         [][]clients.EditAssetBulkPatchAttributeItem
	bulkCreatedRelations     [][]clients.EditAssetCreateRelationRequest
	bulkAttrFailStatus       int
	bulkRelationFailStatus   int
	tagFailStatus            int
	responsibilityFailStatus int
	relationFailStatus       int
	assetNotFound            bool
}

func newStub() *stub {
	return &stub{
		asset: &clients.EditAssetCore{
			ID:     testAssetID,
			Name:   "Churn Rate",
			Type:   clients.EditAssetTypeRef{ID: testAssetTypeID, Name: "Business Term"},
			Domain: clients.EditAssetDomainRef{ID: testDomainID, Name: "Marketing Glossary"},
		},
		attributes: []clients.EditAssetAttributeInstance{
			{
				ID:    defAttrInstanceID,
				Type:  clients.EditAssetAttributeTypeRef{ID: defAttrTypeID, Name: "Definition"},
				Asset: clients.EditAssetAttributeAssetRef{ID: testAssetID},
				Value: "Old definition text",
			},
			{
				ID:    acrInstanceID,
				Type:  clients.EditAssetAttributeTypeRef{ID: acrAttrTypeID, Name: "Acronym"},
				Asset: clients.EditAssetAttributeAssetRef{ID: testAssetID},
				Value: "CR",
			},
		},
		attrTypesByID: map[string]clients.EditAssetAssignmentAttributeType{
			defAttrTypeID:  {ID: defAttrTypeID, Name: "Definition"},
			noteAttrTypeID: {ID: noteAttrTypeID, Name: "Note"},
			acrAttrTypeID:  {ID: acrAttrTypeID, Name: "Acronym"},
		},
		relationTypes: []clients.EditAssetAssignmentRelationType{
			{
				ID:         synonymRelTypeID,
				Role:       "is synonym of",
				CoRole:     "has synonym",
				SourceType: &clients.EditAssetTypeRef{ID: testAssetTypeID, Name: "Business Term"},
				TargetType: &clients.EditAssetTypeRef{ID: testAssetTypeID, Name: "Business Term"},
			},
		},
		roles: []clients.EditAssetRole{
			{ID: stewardRoleID, Name: "Steward"},
		},
		statuses: []clients.EditAssetStatus{
			{ID: candidateStatusID, Name: "Candidate"},
			{ID: acceptedStatusID, Name: "Accepted"},
		},
		users: []clients.EditAssetUser{
			{ID: testUserID, UserName: "jane.smith", EmailAddress: "jane.smith@example.com"},
		},
		patchedAttrs: map[string]string{},
	}
}

func (s *stub) install(mux *http.ServeMux, t *testing.T) {
	t.Helper()

	mux.HandleFunc("GET /rest/2.0/assets/"+testAssetID, func(w http.ResponseWriter, _ *http.Request) {
		if s.assetNotFound {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(s.asset)
	})

	mux.HandleFunc("PATCH /rest/2.0/assets/"+testAssetID, func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		s.patchedAssets = append(s.patchedAssets, body)
		updated := *s.asset
		if v, ok := body["name"].(string); ok {
			updated.Name = v
		}
		if v, ok := body["displayName"].(string); ok {
			updated.DisplayName = v
		}
		if v, ok := body["statusId"].(string); ok {
			updated.Status = &clients.EditAssetStatusRef{ID: v, Name: "Accepted"}
		}
		s.asset = &updated
		_ = json.NewEncoder(w).Encode(updated)
	})

	mux.HandleFunc("GET /rest/2.0/attributes", func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"total":   len(s.attributes),
			"offset":  0,
			"limit":   100,
			"results": s.attributes,
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("POST /rest/2.0/attributes", func(w http.ResponseWriter, r *http.Request) {
		var req clients.CreateAttributeRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.createdAttrs = append(s.createdAttrs, req)
		resp := clients.EditAssetAttributeInstance{
			ID:    "new-" + req.TypeID,
			Type:  clients.EditAssetAttributeTypeRef{ID: req.TypeID, Name: s.attrTypesByID[req.TypeID].Name},
			Asset: clients.EditAssetAttributeAssetRef{ID: req.AssetID},
			Value: req.Value,
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("PATCH /rest/2.0/attributes/", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/attributes/")
		s.patchedAttrs[id] = body["value"]
		resp := clients.EditAssetAttributeInstance{ID: id, Value: body["value"]}
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("DELETE /rest/2.0/attributes/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/attributes/")
		s.deletedAttrIDs = append(s.deletedAttrIDs, id)
		// Also remove from the in-memory list so subsequent calls in the same
		// test don't still see it.
		remaining := s.attributes[:0]
		for _, a := range s.attributes {
			if a.ID != id {
				remaining = append(remaining, a)
			}
		}
		s.attributes = remaining
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /rest/2.0/domains/"+testDomainID, func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(clients.EditAssetDomainDetails{
			ID:   testDomainID,
			Name: "Marketing Glossary",
			Type: &clients.EditAssetDomainTypeRef{ID: testDomainTypeID, Name: "Business Glossary"},
		})
	})

	mux.HandleFunc("GET /rest/2.0/assignments/assetType/"+testAssetTypeID, func(w http.ResponseWriter, _ *http.Request) {
		// Emit Collibra's actual response shape: top-level array with one
		// assignment, characteristicTypes flattening attribute and relation
		// types via assignedCharacteristicTypeDiscriminator.
		chars := []map[string]any{}
		for _, v := range s.attrTypesByID {
			chars = append(chars, map[string]any{
				"id":                 "attr-char-" + v.ID,
				"minimumOccurrences": 0,
				"assignedCharacteristicTypeDiscriminator": "AttributeType",
				"attributeType": map[string]any{
					"id":           v.ID,
					"name":         v.Name,
					"resourceType": "StringAttributeType",
				},
			})
		}
		for _, rt := range s.relationTypes {
			chars = append(chars, map[string]any{
				"id":                 "rel-char-" + rt.ID,
				"minimumOccurrences": 0,
				"roleDirection":      "TO_TARGET",
				"assignedCharacteristicTypeDiscriminator": "RelationType",
				"relationType": map[string]any{
					"id":         rt.ID,
					"role":       rt.Role,
					"coRole":     rt.CoRole,
					"sourceType": rt.SourceType,
					"targetType": rt.TargetType,
				},
			})
		}
		_ = json.NewEncoder(w).Encode([]map[string]any{{
			"id":        "assignment-1",
			"assetType": map[string]any{"id": testAssetTypeID, "name": "Business Term"},
			"domainTypes": []map[string]any{{
				"id": testDomainTypeID, "name": "Business Glossary",
			}},
			"characteristicTypes": chars,
		}})
	})

	mux.HandleFunc("POST /rest/2.0/relations", func(w http.ResponseWriter, r *http.Request) {
		var req clients.EditAssetCreateRelationRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.createdRelations = append(s.createdRelations, req)
		if s.relationFailStatus != 0 {
			w.WriteHeader(s.relationFailStatus)
			_, _ = w.Write([]byte(`{"message":"simulated relation failure"}`))
			return
		}
		resp := clients.EditAssetRelation{
			ID:     testRelationID,
			Type:   clients.EditAssetTypeRef{ID: req.TypeID, Name: "is synonym of"},
			Source: clients.EditAssetAttributeAssetRef{ID: req.SourceID},
			Target: clients.EditAssetAttributeAssetRef{ID: req.TargetID},
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("DELETE /rest/2.0/relations/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/relations/")
		if id != testRelationID {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		s.deletedRelationIDs = append(s.deletedRelationIDs, id)
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /rest/2.0/assets/"+testAssetID+"/tags", func(w http.ResponseWriter, r *http.Request) {
		// Mimic Collibra's actual contract: required field is "tagNames".
		// If the client sends anything else, the field comes through empty
		// and we 400 the same way the real API does.
		var raw map[string]json.RawMessage
		_ = json.NewDecoder(r.Body).Decode(&raw)
		var names []string
		if rawNames, ok := raw["tagNames"]; ok {
			_ = json.Unmarshal(rawNames, &names)
		}
		if len(names) == 0 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"message":"tagNames may not be null"}`))
			return
		}
		s.addedTags = append(s.addedTags, names)
		if s.tagFailStatus != 0 {
			w.WriteHeader(s.tagFailStatus)
			_, _ = w.Write([]byte(`{"message":"simulated tag failure"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /rest/2.0/roles", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total":   len(s.roles),
			"offset":  0,
			"limit":   1000,
			"results": s.roles,
		})
	})

	mux.HandleFunc("GET /rest/2.0/users", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var matches []clients.EditAssetUser
		username := q.Get("name")
		email := q.Get("emailAddress")
		for _, u := range s.users {
			if username != "" && u.UserName == username {
				matches = append(matches, u)
			}
			if email != "" && u.EmailAddress == email {
				matches = append(matches, u)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total":   len(matches),
			"offset":  0,
			"limit":   1,
			"results": matches,
		})
	})

	mux.HandleFunc("GET /rest/2.0/statuses", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total":   len(s.statuses),
			"offset":  0,
			"limit":   1000,
			"results": s.statuses,
		})
	})

	mux.HandleFunc("POST /rest/2.0/attributes/bulk", func(w http.ResponseWriter, r *http.Request) {
		var items []clients.CreateAttributeRequest
		_ = json.NewDecoder(r.Body).Decode(&items)
		s.bulkCreatedAttrs = append(s.bulkCreatedAttrs, items)
		if s.bulkAttrFailStatus != 0 {
			w.WriteHeader(s.bulkAttrFailStatus)
			_, _ = w.Write([]byte(`{"message":"simulated bulk attr failure"}`))
			return
		}
		resp := make([]clients.EditAssetAttributeInstance, len(items))
		for i, it := range items {
			resp[i] = clients.EditAssetAttributeInstance{
				ID:    "bulk-new-" + it.TypeID,
				Type:  clients.EditAssetAttributeTypeRef{ID: it.TypeID, Name: s.attrTypesByID[it.TypeID].Name},
				Asset: clients.EditAssetAttributeAssetRef{ID: it.AssetID},
				Value: it.Value,
			}
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("PATCH /rest/2.0/attributes/bulk", func(w http.ResponseWriter, r *http.Request) {
		var items []clients.EditAssetBulkPatchAttributeItem
		_ = json.NewDecoder(r.Body).Decode(&items)
		s.bulkPatchedAttrs = append(s.bulkPatchedAttrs, items)
		if s.bulkAttrFailStatus != 0 {
			w.WriteHeader(s.bulkAttrFailStatus)
			_, _ = w.Write([]byte(`{"message":"simulated bulk patch failure"}`))
			return
		}
		resp := make([]clients.EditAssetAttributeInstance, len(items))
		for i, it := range items {
			resp[i] = clients.EditAssetAttributeInstance{ID: it.ID, Value: it.Value}
		}
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("POST /rest/2.0/relations/bulk", func(w http.ResponseWriter, r *http.Request) {
		var items []clients.EditAssetCreateRelationRequest
		_ = json.NewDecoder(r.Body).Decode(&items)
		s.bulkCreatedRelations = append(s.bulkCreatedRelations, items)
		if s.bulkRelationFailStatus != 0 {
			w.WriteHeader(s.bulkRelationFailStatus)
			_, _ = w.Write([]byte(`{"message":"simulated bulk relation failure"}`))
			return
		}
		resp := make([]clients.EditAssetRelation, len(items))
		for i, it := range items {
			resp[i] = clients.EditAssetRelation{
				ID:     "bulk-rel-" + it.TargetID,
				Type:   clients.EditAssetTypeRef{ID: it.TypeID, Name: "is synonym of"},
				Source: clients.EditAssetAttributeAssetRef{ID: it.SourceID},
				Target: clients.EditAssetAttributeAssetRef{ID: it.TargetID},
			}
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("POST /rest/2.0/responsibilities", func(w http.ResponseWriter, r *http.Request) {
		var req clients.EditAssetCreateResponsibilityRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		s.createdResponsibilities = append(s.createdResponsibilities, req)
		if s.responsibilityFailStatus != 0 {
			w.WriteHeader(s.responsibilityFailStatus)
			_, _ = w.Write([]byte(`{"message":"simulated responsibility failure"}`))
			return
		}
		resp := clients.EditAssetResponsibility{
			ID:         testResponsibilityID,
			RoleID:     req.RoleID,
			OwnerID:    req.OwnerID,
			ResourceID: req.ResourceID,
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	})
}

func runTool(t *testing.T, s *stub, in edit_asset.Input) (edit_asset.Output, error) {
	t.Helper()
	mux := http.NewServeMux()
	s.install(mux, t)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	client := testutil.NewClient(srv)
	return edit_asset.NewTool(client).Handler(t.Context(), in)
}

// --- tests --------------------------------------------------------------------

func TestEditAsset_UpdateAttribute_HappyPath(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpUpdateAttribute, AttributeName: "Definition", Value: "New definition",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q, results=%+v", out.Status, out.Results)
	}
	if got := s.patchedAttrs[defAttrInstanceID]; got != "New definition" {
		t.Fatalf("expected PATCH with new value, got %q", got)
	}
	r := out.Results[0]
	if r.PreviousValue != "Old definition text" || r.NewValue != "New definition" {
		t.Fatalf("unexpected diff: prev=%q new=%q", r.PreviousValue, r.NewValue)
	}
}

func TestEditAsset_AddAttribute_HappyPath(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpAddAttribute, AttributeName: "Note", Value: "Reviewed 2026-04",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q", out.Status)
	}
	if len(s.createdAttrs) != 1 || s.createdAttrs[0].TypeID != noteAttrTypeID {
		t.Fatalf("expected POST to create Note attribute, got %+v", s.createdAttrs)
	}
}

func TestEditAsset_RemoveAttribute_HappyPath(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpRemoveAttribute, AttributeName: "Acronym",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q, results=%+v", out.Status, out.Results)
	}
	if len(s.deletedAttrIDs) != 1 || s.deletedAttrIDs[0] != acrInstanceID {
		t.Fatalf("expected DELETE of acronym instance, got %v", s.deletedAttrIDs)
	}
	if out.Results[0].PreviousValue != "CR" {
		t.Fatalf("expected previousValue=CR, got %q", out.Results[0].PreviousValue)
	}
}

func TestEditAsset_UpdateProperty_Name(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpUpdateProperty, Field: "name", Value: "Renamed",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q", out.Status)
	}
	if got := s.patchedAssets[0]["name"]; got != "Renamed" {
		t.Fatalf("expected PATCH name=Renamed, got %v", got)
	}
	if out.Results[0].PreviousValue != "Churn Rate" || out.Results[0].NewValue != "Renamed" {
		t.Fatalf("unexpected diff: %+v", out.Results[0])
	}
}

func TestEditAsset_UpdateProperty_StatusID_ByUUID(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpUpdateProperty, Field: "statusId", Value: acceptedStatusID,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q", out.Status)
	}
	if got := s.patchedAssets[0]["statusId"]; got != acceptedStatusID {
		t.Fatalf("expected PATCH statusId=%s, got %v", acceptedStatusID, got)
	}
}

func TestEditAsset_UpdateProperty_StatusID_ByName(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpUpdateProperty, Field: "statusId", Value: "Candidate",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q, results=%+v", out.Status, out.Results)
	}
	if got := s.patchedAssets[0]["statusId"]; got != candidateStatusID {
		t.Fatalf("expected name 'Candidate' to resolve to %s, got %v", candidateStatusID, got)
	}
}

func TestEditAsset_UpdateProperty_StatusID_UnknownName(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpUpdateProperty, Field: "statusId", Value: "Mayor",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError || !strings.Contains(out.Results[0].Error, "not defined") {
		t.Fatalf("expected unknown-status error, got %+v", out)
	}
	if len(s.patchedAssets) != 0 {
		t.Fatalf("expected no PATCH on unknown status, got %+v", s.patchedAssets)
	}
}

func TestEditAsset_StatusesFetchedOnlyWhenNeeded(t *testing.T) {
	s := newStub()
	mux := http.NewServeMux()
	s.install(mux, t)
	var statusesCalled bool
	mux.HandleFunc("GET /rest/2.0/statuses/", func(w http.ResponseWriter, _ *http.Request) {
		statusesCalled = true
		_ = json.NewEncoder(w).Encode(map[string]any{"total": 0, "results": []any{}})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := testutil.NewClient(srv)
	_, err := edit_asset.NewTool(client).Handler(t.Context(), edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpUpdateAttribute, AttributeName: "Definition", Value: "x",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if statusesCalled {
		t.Fatal("statuses endpoint should not be hit when no statusId update is present")
	}
}

func TestEditAsset_UnknownAttributeName(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpUpdateAttribute, AttributeName: "NotAnAttribute", Value: "x",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError {
		t.Fatalf("expected error, got %q", out.Status)
	}
	if !strings.Contains(out.Results[0].Error, "not valid for asset type") {
		t.Fatalf("expected scoped-assignment error, got %q", out.Results[0].Error)
	}
}

func TestEditAsset_AmbiguousAttributeName(t *testing.T) {
	s := newStub()
	// Two Note instances on the asset
	s.attributes = append(s.attributes,
		clients.EditAssetAttributeInstance{ID: noteInstanceAID, Type: clients.EditAssetAttributeTypeRef{ID: noteAttrTypeID, Name: "Note"}, Asset: clients.EditAssetAttributeAssetRef{ID: testAssetID}, Value: "one"},
		clients.EditAssetAttributeInstance{ID: noteInstanceBID, Type: clients.EditAssetAttributeTypeRef{ID: noteAttrTypeID, Name: "Note"}, Asset: clients.EditAssetAttributeAssetRef{ID: testAssetID}, Value: "two"},
	)
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpUpdateAttribute, AttributeName: "Note", Value: "three",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError {
		t.Fatalf("expected error, got %q", out.Status)
	}
	if !strings.Contains(out.Results[0].Error, "cannot disambiguate") {
		t.Fatalf("expected disambiguation error, got %q", out.Results[0].Error)
	}
	if len(s.patchedAttrs) != 0 {
		t.Fatalf("expected no writes on ambiguous update, saw %+v", s.patchedAttrs)
	}
}

func TestEditAsset_UnsupportedPropertyField(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpUpdateProperty, Field: "typeId", Value: "x",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError || !strings.Contains(out.Results[0].Error, "not supported") {
		t.Fatalf("expected unsupported-field error, got %+v", out)
	}
	if len(s.patchedAssets) != 0 {
		t.Fatalf("expected no PATCH on unsupported field, saw %+v", s.patchedAssets)
	}
}

func TestEditAsset_AssetNotFound(t *testing.T) {
	s := newStub()
	s.assetNotFound = true
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpUpdateAttribute, AttributeName: "Definition", Value: "x",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError || out.Error == "" {
		t.Fatalf("expected top-level error on asset not found, got %+v", out)
	}
}

func TestEditAsset_InvalidAssetID(t *testing.T) {
	s := newStub()
	_, err := runTool(t, s, edit_asset.Input{
		AssetID:    "not-a-uuid",
		Operations: []edit_asset.Operation{{Type: edit_asset.OpUpdateAttribute, AttributeName: "x", Value: "x"}},
	})
	if err == nil {
		t.Fatal("expected UUID validation error, got nil")
	}
}

func TestEditAsset_EmptyOperations(t *testing.T) {
	s := newStub()
	_, err := runTool(t, s, edit_asset.Input{AssetID: testAssetID})
	if err == nil {
		t.Fatal("expected error on empty operations, got nil")
	}
}

func TestEditAsset_PartialSuccess(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{
			{Type: edit_asset.OpUpdateAttribute, AttributeName: "Definition", Value: "new def"},
			{Type: edit_asset.OpUpdateAttribute, AttributeName: "Bogus", Value: "x"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusPartialSuccess {
		t.Fatalf("expected partial_success, got %q", out.Status)
	}
	if out.Results[0].Status != "success" {
		t.Fatalf("expected first op to succeed, got %+v", out.Results[0])
	}
	if out.Results[1].Status != "error" {
		t.Fatalf("expected second op to fail, got %+v", out.Results[1])
	}
	if _, patched := s.patchedAttrs[defAttrInstanceID]; !patched {
		t.Fatal("expected valid op to have run despite sibling failure")
	}
}

func TestEditAsset_AddRelation_HappyPath(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpAddRelation, RelationType: "is synonym of", TargetAssetID: targetAssetID,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q, results=%+v", out.Status, out.Results)
	}
	if len(s.createdRelations) != 1 {
		t.Fatalf("expected one POST to /rest/2.0/relations, got %+v", s.createdRelations)
	}
	got := s.createdRelations[0]
	if got.SourceID != testAssetID || got.TargetID != targetAssetID || got.TypeID != synonymRelTypeID {
		t.Fatalf("unexpected relation payload: %+v", got)
	}
	if out.Results[0].RelationID != testRelationID {
		t.Fatalf("expected RelationID in result, got %q", out.Results[0].RelationID)
	}
}

func TestEditAsset_AddRelation_UnknownType(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpAddRelation, RelationType: "not a real relation", TargetAssetID: targetAssetID,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError {
		t.Fatalf("expected error, got %q", out.Status)
	}
	if !strings.Contains(out.Results[0].Error, "not valid for asset type") {
		t.Fatalf("expected scoped-assignment error, got %q", out.Results[0].Error)
	}
	if len(s.createdRelations) != 0 {
		t.Fatalf("expected no POST on unknown relation type, got %+v", s.createdRelations)
	}
}

func TestEditAsset_AddRelation_CoRoleDoesNotMatch(t *testing.T) {
	s := newStub()
	// "has synonym" is the coRole; we intentionally only match forward roles.
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpAddRelation, RelationType: "has synonym", TargetAssetID: targetAssetID,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError {
		t.Fatalf("expected error for coRole-only match, got %q", out.Status)
	}
}

func TestEditAsset_AddRelation_InvalidTargetUUID(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpAddRelation, RelationType: "is synonym of", TargetAssetID: "not-a-uuid",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError || !strings.Contains(out.Results[0].Error, "UUID") {
		t.Fatalf("expected UUID validation error, got %+v", out)
	}
}

func TestEditAsset_AddRelation_CollibraRejection(t *testing.T) {
	// Simulate Collibra returning a 422, e.g. because the direction is wrong —
	// user assumed source but the relation expects this asset as target.
	s := newStub()
	s.relationFailStatus = http.StatusUnprocessableEntity
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpAddRelation, RelationType: "is synonym of", TargetAssetID: targetAssetID,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError {
		t.Fatalf("expected per-op error on Collibra rejection, got %q", out.Status)
	}
}

func TestEditAsset_RemoveRelation_HappyPath(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpRemoveRelation, RelationID: testRelationID,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q", out.Status)
	}
	if len(s.deletedRelationIDs) != 1 || s.deletedRelationIDs[0] != testRelationID {
		t.Fatalf("expected DELETE of relation, got %v", s.deletedRelationIDs)
	}
}

func TestEditAsset_RemoveRelation_NotFound(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpRemoveRelation, RelationID: "8b000000-0000-0000-0000-00000000dead",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError || !strings.Contains(out.Results[0].Error, "not found") {
		t.Fatalf("expected not-found error, got %+v", out)
	}
}

func TestEditAsset_RemoveRelation_InvalidUUID(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpRemoveRelation, RelationID: "not-a-uuid",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError || !strings.Contains(out.Results[0].Error, "UUID") {
		t.Fatalf("expected UUID validation error, got %+v", out)
	}
}

func TestEditAsset_AddTag_HappyPath(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpAddTag, Tag: "finance",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q, results=%+v", out.Status, out.Results)
	}
	if len(s.addedTags) != 1 || len(s.addedTags[0]) != 1 || s.addedTags[0][0] != "finance" {
		t.Fatalf("expected POST to tags with ['finance'], got %+v", s.addedTags)
	}
	if out.Results[0].NewValue != "finance" {
		t.Fatalf("expected NewValue=finance, got %q", out.Results[0].NewValue)
	}
}

func TestEditAsset_AddTag_Empty(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpAddTag, Tag: "",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError || !strings.Contains(out.Results[0].Error, "tag is required") {
		t.Fatalf("expected 'tag is required' error, got %+v", out)
	}
	if len(s.addedTags) != 0 {
		t.Fatalf("expected no POST on empty tag, got %+v", s.addedTags)
	}
}

func TestEditAsset_SetResponsibility_HappyPath(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpSetResponsibility, Role: "Steward", UserID: testUserID,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q, results=%+v", out.Status, out.Results)
	}
	if len(s.createdResponsibilities) != 1 {
		t.Fatalf("expected one POST to /rest/2.0/responsibilities, got %+v", s.createdResponsibilities)
	}
	got := s.createdResponsibilities[0]
	if got.RoleID != stewardRoleID || got.OwnerID != testUserID || got.ResourceID != testAssetID {
		t.Fatalf("unexpected responsibility payload: %+v", got)
	}
	if out.Results[0].NewValue != testResponsibilityID {
		t.Fatalf("expected responsibility ID in NewValue, got %q", out.Results[0].NewValue)
	}
}

func TestEditAsset_SetResponsibility_UnknownRole(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpSetResponsibility, Role: "Mayor", UserID: testUserID,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError || !strings.Contains(out.Results[0].Error, "not defined") {
		t.Fatalf("expected unknown-role error, got %+v", out)
	}
	if len(s.createdResponsibilities) != 0 {
		t.Fatalf("expected no POST on unknown role, got %+v", s.createdResponsibilities)
	}
}

func TestEditAsset_SetResponsibility_UnknownUserName(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpSetResponsibility, Role: "Steward", UserID: "no.such.user",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError || !strings.Contains(out.Results[0].Error, "no user found") {
		t.Fatalf("expected no-user-found error, got %+v", out)
	}
}

func TestEditAsset_SetResponsibility_ResolvesByUsername(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpSetResponsibility, Role: "Steward", UserID: "jane.smith",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q, results=%+v", out.Status, out.Results)
	}
	if s.createdResponsibilities[0].OwnerID != testUserID {
		t.Fatalf("expected username 'jane.smith' to resolve to %s, got %s", testUserID, s.createdResponsibilities[0].OwnerID)
	}
}

func TestEditAsset_SetResponsibility_ResolvesByEmail(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpSetResponsibility, Role: "Steward", UserID: "jane.smith@example.com",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q, results=%+v", out.Status, out.Results)
	}
	if s.createdResponsibilities[0].OwnerID != testUserID {
		t.Fatalf("expected email to resolve to %s, got %s", testUserID, s.createdResponsibilities[0].OwnerID)
	}
}

// --- Case-insensitive + whitespace + suggestion tests ----------------------

func TestEditAsset_AttributeName_CaseAndWhitespaceInsensitive(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpUpdateAttribute, AttributeName: "  definition ", Value: "matched despite case/whitespace",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q, results=%+v", out.Status, out.Results)
	}
}

func TestEditAsset_StatusName_CaseAndWhitespaceInsensitive(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpUpdateProperty, Field: "statusId", Value: " CANDIDATE ",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q", out.Status)
	}
	if got := s.patchedAssets[0]["statusId"]; got != candidateStatusID {
		t.Fatalf("expected normalized lookup to resolve, got %v", got)
	}
}

func TestEditAsset_RoleName_CaseAndWhitespaceInsensitive(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpSetResponsibility, Role: "steward", UserID: testUserID,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success on case-insensitive role match, got %q", out.Status)
	}
}

func TestEditAsset_ErrorIncludesAvailableAttributes(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpUpdateAttribute, AttributeName: "Bogus", Value: "x",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg := out.Results[0].Error
	if !strings.Contains(msg, "Attributes available:") {
		t.Fatalf("error should suggest available attributes, got %q", msg)
	}
	for _, expected := range []string{"Definition", "Note", "Acronym"} {
		if !strings.Contains(msg, expected) {
			t.Fatalf("error should include %q in suggestion, got %q", expected, msg)
		}
	}
}

func TestEditAsset_ErrorIncludesAvailableStatuses(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpUpdateProperty, Field: "statusId", Value: "NotARealStatus",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	msg := out.Results[0].Error
	if !strings.Contains(msg, "Statuses available:") || !strings.Contains(msg, "Candidate") {
		t.Fatalf("error should suggest available statuses, got %q", msg)
	}
}

func TestEditAsset_SetResponsibility_MissingRole(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpSetResponsibility, UserID: testUserID,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError || !strings.Contains(out.Results[0].Error, "role is required") {
		t.Fatalf("expected 'role is required' error, got %+v", out)
	}
}

func TestEditAsset_SetResponsibility_CollibraRejection(t *testing.T) {
	s := newStub()
	s.responsibilityFailStatus = http.StatusConflict
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpSetResponsibility, Role: "Steward", UserID: testUserID,
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError {
		t.Fatalf("expected per-op error, got %q", out.Status)
	}
}

func TestEditAsset_RolesFetchedOnlyWhenNeeded(t *testing.T) {
	// Request with no set_responsibility op should not fetch /rest/2.0/roles.
	// Verify by observing the request path log on the stub.
	s := newStub()

	mux := http.NewServeMux()
	s.install(mux, t)
	var rolesCalled bool
	// Override to flag when /roles is hit.
	mux.HandleFunc("GET /rest/2.0/roles/", func(w http.ResponseWriter, _ *http.Request) {
		rolesCalled = true
		_ = json.NewEncoder(w).Encode(map[string]any{"total": 0, "results": []any{}})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := testutil.NewClient(srv)
	_, err := edit_asset.NewTool(client).Handler(t.Context(), edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{{
			Type: edit_asset.OpAddTag, Tag: "finance",
		}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rolesCalled {
		t.Fatal("roles endpoint should not be hit when no set_responsibility op is present")
	}
}

// --- Phase 4: bulk batching ---------------------------------------------------

func TestEditAsset_BulkAddAttributes(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{
			{Type: edit_asset.OpAddAttribute, AttributeName: "Note", Value: "one"},
			{Type: edit_asset.OpAddAttribute, AttributeName: "Acronym", Value: "CR"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q, results=%+v", out.Status, out.Results)
	}
	if len(s.bulkCreatedAttrs) != 1 || len(s.bulkCreatedAttrs[0]) != 2 {
		t.Fatalf("expected one bulk POST with 2 items, got %+v", s.bulkCreatedAttrs)
	}
	if len(s.createdAttrs) != 0 {
		t.Fatalf("expected no individual POST /attributes calls, got %+v", s.createdAttrs)
	}
}

func TestEditAsset_SingleAddAttributeUsesIndividualEndpoint(t *testing.T) {
	s := newStub()
	_, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{
			{Type: edit_asset.OpAddAttribute, AttributeName: "Note", Value: "solo"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.bulkCreatedAttrs) != 0 {
		t.Fatalf("expected no bulk POST on single op, got %+v", s.bulkCreatedAttrs)
	}
	if len(s.createdAttrs) != 1 {
		t.Fatalf("expected one individual POST, got %+v", s.createdAttrs)
	}
}

func TestEditAsset_BulkUpdateAttributes(t *testing.T) {
	s := newStub()
	// Need two existing attributes to update.
	s.attributes = append(s.attributes, clients.EditAssetAttributeInstance{
		ID:    noteInstanceAID,
		Type:  clients.EditAssetAttributeTypeRef{ID: noteAttrTypeID, Name: "Note"},
		Asset: clients.EditAssetAttributeAssetRef{ID: testAssetID},
		Value: "old note",
	})
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{
			{Type: edit_asset.OpUpdateAttribute, AttributeName: "Definition", Value: "new def"},
			{Type: edit_asset.OpUpdateAttribute, AttributeName: "Note", Value: "new note"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q", out.Status)
	}
	if len(s.bulkPatchedAttrs) != 1 || len(s.bulkPatchedAttrs[0]) != 2 {
		t.Fatalf("expected one bulk PATCH with 2 items, got %+v", s.bulkPatchedAttrs)
	}
	if len(s.patchedAttrs) != 0 {
		t.Fatalf("expected no individual PATCH /attributes calls, got %+v", s.patchedAttrs)
	}
	if out.Results[0].PreviousValue != "Old definition text" || out.Results[1].PreviousValue != "old note" {
		t.Fatalf("expected previous values preserved per op, got %+v", out.Results)
	}
}

func TestEditAsset_BulkAddRelations(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{
			{Type: edit_asset.OpAddRelation, RelationType: "is synonym of", TargetAssetID: targetAssetID},
			{Type: edit_asset.OpAddRelation, RelationType: "is synonym of", TargetAssetID: "018d3602-aaaa-0000-0000-000000000001"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q", out.Status)
	}
	if len(s.bulkCreatedRelations) != 1 || len(s.bulkCreatedRelations[0]) != 2 {
		t.Fatalf("expected one bulk POST with 2 items, got %+v", s.bulkCreatedRelations)
	}
	if len(s.createdRelations) != 0 {
		t.Fatalf("expected no individual relation POSTs, got %+v", s.createdRelations)
	}
	if out.Results[0].RelationID == "" || out.Results[1].RelationID == "" {
		t.Fatalf("expected RelationID populated per op, got %+v", out.Results)
	}
}

func TestEditAsset_BulkFailureMarksAllOpsFailed(t *testing.T) {
	s := newStub()
	s.bulkAttrFailStatus = http.StatusBadRequest
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{
			{Type: edit_asset.OpAddAttribute, AttributeName: "Note", Value: "one"},
			{Type: edit_asset.OpAddAttribute, AttributeName: "Acronym", Value: "CR"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusError {
		t.Fatalf("expected error, got %q", out.Status)
	}
	for i, r := range out.Results {
		if r.Status != "error" {
			t.Fatalf("op %d should be error on batch failure, got %+v", i, r)
		}
	}
}

func TestEditAsset_BulkOnlyGroupsSameType(t *testing.T) {
	// Two add_attrs (bulk) + one update_attr (individual) + one update_property
	// (individual) + one add_tag (individual).
	s := newStub()
	_, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{
			{Type: edit_asset.OpAddAttribute, AttributeName: "Note", Value: "one"},
			{Type: edit_asset.OpAddAttribute, AttributeName: "Acronym", Value: "CR"},
			{Type: edit_asset.OpUpdateAttribute, AttributeName: "Definition", Value: "new def"},
			{Type: edit_asset.OpUpdateProperty, Field: "name", Value: "Renamed"},
			{Type: edit_asset.OpAddTag, Tag: "finance"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.bulkCreatedAttrs) != 1 || len(s.bulkCreatedAttrs[0]) != 2 {
		t.Fatalf("expected bulk add POST with 2 items, got %+v", s.bulkCreatedAttrs)
	}
	if len(s.bulkPatchedAttrs) != 0 {
		t.Fatalf("expected no bulk PATCH (only 1 update op), got %+v", s.bulkPatchedAttrs)
	}
	if len(s.patchedAttrs) != 1 {
		t.Fatalf("expected one individual PATCH /attributes for the lone update, got %+v", s.patchedAttrs)
	}
	if len(s.patchedAssets) != 1 {
		t.Fatalf("expected one PATCH /assets for update_property, got %+v", s.patchedAssets)
	}
	if len(s.addedTags) != 1 {
		t.Fatalf("expected one tag POST, got %+v", s.addedTags)
	}
}

func TestEditAsset_InvalidOpsExcludedFromBulk(t *testing.T) {
	// Two add_attrs, but one is invalid (unknown attribute). The valid one
	// should run via the individual endpoint (not bulk) since the threshold
	// filters on validated ops.
	s := newStub()
	_, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{
			{Type: edit_asset.OpAddAttribute, AttributeName: "Note", Value: "valid"},
			{Type: edit_asset.OpAddAttribute, AttributeName: "NotValid", Value: "x"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.bulkCreatedAttrs) != 0 {
		t.Fatalf("expected no bulk POST when only 1 op is valid, got %+v", s.bulkCreatedAttrs)
	}
	if len(s.createdAttrs) != 1 {
		t.Fatalf("expected one individual POST for the valid op, got %+v", s.createdAttrs)
	}
}

func TestEditAsset_MultipleValidOperations(t *testing.T) {
	s := newStub()
	out, err := runTool(t, s, edit_asset.Input{
		AssetID: testAssetID,
		Operations: []edit_asset.Operation{
			{Type: edit_asset.OpUpdateAttribute, AttributeName: "Definition", Value: "new def"},
			{Type: edit_asset.OpAddAttribute, AttributeName: "Note", Value: "note"},
			{Type: edit_asset.OpRemoveAttribute, AttributeName: "Acronym"},
			{Type: edit_asset.OpUpdateProperty, Field: "name", Value: "Renamed"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != edit_asset.StatusSuccess {
		t.Fatalf("expected success, got %q, results=%+v", out.Status, out.Results)
	}
	if len(s.patchedAttrs) != 1 || len(s.createdAttrs) != 1 || len(s.deletedAttrIDs) != 1 || len(s.patchedAssets) != 1 {
		t.Fatalf("unexpected call distribution: patched=%d created=%d deleted=%d patchedAsset=%d",
			len(s.patchedAttrs), len(s.createdAttrs), len(s.deletedAttrIDs), len(s.patchedAssets))
	}
}
