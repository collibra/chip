package create_asset_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/create_asset"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// --- Test fixtures ---
//
// All tests share a "Business Term in Glossary" world by default and override
// only what they exercise. The fixture set mirrors the live DGC payload
// shapes (same field names and discriminators), so a wire-format regression
// here is also a wire-format regression in production.

const (
	btTypeID         = "00000000-0000-0000-0000-000000011001"
	btTypePublicID   = "BusinessTerm"
	btTypeName       = "Business Term"
	glossaryDomainID = "00000000-0000-0000-0000-000000099001"
	glossaryDomain   = "My Glossary"
	glossaryTypeID   = "00000000-0000-0000-0000-000000010001"
	glossaryTypeName = "Glossary"
	defAttrID        = "00000000-0000-0000-0000-000000000202"
	defAttrName      = "Definition"
	defAttrPublicID  = "Definition"
	noteAttrID       = "00000000-0000-0000-0000-0000000003116"
	noteAttrName     = "Note"
	candidateID      = "00000000-0000-0000-0000-000000005008"
	candidateName    = "Candidate"
)

// mockDGC bundles a typical Collibra mock with overrideable behavior. The
// zero-value fields use sensible defaults so each test body only needs to
// declare what it cares about.
type mockDGC struct {
	t                 *testing.T
	mu                sync.Mutex
	createdAssets     []clients.CreateAssetRequest
	createdAttributes []clients.CreateAttributeRequest

	// Overrides — when set, replace the corresponding default behavior.
	assetTypeByName  map[string][]assetTypeRow // case-insensitive prefix as Collibra returns it
	domainByName     map[string][]domainRow    // case-insensitive prefix
	dupResults       []asssetSearchRow
	defStringType    string // value to return on /attributeTypes/{def}; default "RICH_TEXT"
	noteStringType   string // value to return on /attributeTypes/{note}; default "PLAIN_TEXT"
	createAssetCode  int    // override status; default 201
	createAttrCode   int    // override status; default 201
	noAssignments    bool   // /assignments/assetType/{id} returns [] (asset type has no assignment anywhere)
	emptyDomainTypes bool   // the default assignment lists empty domainTypes (creatable nowhere, sub-case b)
	domainTypeOther  bool   // the glossary domain resolves to a non-Glossary type, so the assignment doesn't govern it (not-here)

	// extraAssignments are appended to the default Business Term assignment
	// in the /assignments/assetType/{id} response (e.g. a scoped assignment).
	extraAssignments []map[string]any
}

type assetTypeRow struct {
	ID       string `json:"id"`
	PublicID string `json:"publicId"`
	Name     string `json:"name"`
}
type domainRow struct {
	ID   string                           `json:"id"`
	Name string                           `json:"name"`
	Type *clients.PrepareCreateDomainType `json:"type,omitempty"`
}
type asssetSearchRow struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func newMockDGC(t *testing.T) *mockDGC {
	return &mockDGC{t: t}
}

// server boots an httptest server preloaded with the BusinessTerm-in-Glossary
// world; t.Cleanup tears it down.
func (m *mockDGC) server() *httptest.Server {
	mux := http.NewServeMux()

	// /assetTypes/{idOrPublicIdPath}
	mux.HandleFunc("GET /rest/2.0/assetTypes/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/rest/2.0/assetTypes/")
		// publicId path: /assetTypes/publicId/{publicId}
		if strings.HasPrefix(path, "publicId/") {
			pid := strings.TrimPrefix(path, "publicId/")
			if pid == btTypePublicID {
				writeJSON(w, http.StatusOK, assetTypeRow{ID: btTypeID, PublicID: btTypePublicID, Name: btTypeName})
				return
			}
			http.NotFound(w, r)
			return
		}
		// id path
		if path == btTypeID {
			writeJSON(w, http.StatusOK, assetTypeRow{ID: btTypeID, PublicID: btTypePublicID, Name: btTypeName})
			return
		}
		http.NotFound(w, r)
	})

	// /assetTypes (list & search by name)
	mux.HandleFunc("GET /rest/2.0/assetTypes", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		var results []assetTypeRow
		if name != "" {
			if rows, ok := m.assetTypeByName[strings.ToLower(name)]; ok {
				results = rows
			} else if strings.EqualFold(name, btTypeName) {
				results = []assetTypeRow{{ID: btTypeID, PublicID: btTypePublicID, Name: btTypeName}}
			}
		} else {
			results = []assetTypeRow{
				{ID: btTypeID, PublicID: btTypePublicID, Name: btTypeName},
				{ID: "00000000-0000-0000-0000-000000011002", PublicID: "Code", Name: "Code"},
				{ID: "00000000-0000-0000-0000-000000011003", PublicID: "Column", Name: "Column"},
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": results, "total": len(results)})
	})

	// glossaryDomainType is the type the glossary domain resolves to. With
	// domainTypeOther it becomes a non-Glossary type, so the Glossary-typed
	// assignment no longer governs the domain (the not-here branch).
	glossaryDomainType := &clients.PrepareCreateDomainType{ID: glossaryTypeID, Name: glossaryTypeName}
	if m.domainTypeOther {
		glossaryDomainType = &clients.PrepareCreateDomainType{ID: "00000000-0000-0000-0000-000000010099", Name: "Other Domain Type"}
	}

	// /domains/{id}
	mux.HandleFunc("GET /rest/2.0/domains/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/domains/")
		if id == glossaryDomainID {
			writeJSON(w, http.StatusOK, domainRow{
				ID:   glossaryDomainID,
				Name: glossaryDomain,
				Type: glossaryDomainType,
			})
			return
		}
		http.NotFound(w, r)
	})

	// /domains (list & search by name)
	mux.HandleFunc("GET /rest/2.0/domains", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		var results []domainRow
		if name != "" {
			if rows, ok := m.domainByName[strings.ToLower(name)]; ok {
				results = rows
			} else if strings.EqualFold(name, glossaryDomain) {
				results = []domainRow{{ID: glossaryDomainID, Name: glossaryDomain, Type: glossaryDomainType}}
			}
		} else {
			results = []domainRow{
				{ID: glossaryDomainID, Name: glossaryDomain, Type: &clients.PrepareCreateDomainType{ID: glossaryTypeID, Name: glossaryTypeName}},
				{ID: "00000000-0000-0000-0000-0000000099002", Name: "Other Domain", Type: &clients.PrepareCreateDomainType{ID: "00000000-0000-0000-0000-000000010099", Name: "Other Domain Type"}},
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": results, "total": len(results)})
	})

	// /assignments/assetType/{id}
	mux.HandleFunc("GET /rest/2.0/assignments/assetType/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/assignments/assetType/")
		if m.noAssignments || id != btTypeID {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		// domainTypes governs creatability: normally lists Glossary; with
		// emptyDomainTypes the assignment admits no domain type at all, so the
		// type is creatable nowhere (nowhere sub-case b).
		domainTypes := []map[string]string{{"id": glossaryTypeID, "name": glossaryTypeName}}
		if m.emptyDomainTypes {
			domainTypes = []map[string]string{}
		}
		payload := []map[string]any{
			{
				"id":          "assignment-bt",
				"domainTypes": domainTypes,
				"assignedCharacteristicTypeReferences": []map[string]any{
					{
						"id": "ref-def",
						"assignedResourceReference": map[string]string{
							"id": defAttrID, "name": defAttrName, "resourceType": "StringAttributeType", "resourceDiscriminator": "StringAttributeType",
						},
						"assignedResourcePublicId": defAttrPublicID,
						"minimumOccurrences":       1,
					},
					{
						"id": "ref-note",
						"assignedResourceReference": map[string]string{
							"id": noteAttrID, "name": noteAttrName, "resourceType": "StringAttributeType", "resourceDiscriminator": "StringAttributeType",
						},
						"assignedResourcePublicId": "Note",
						"minimumOccurrences":       0,
					},
				},
				"characteristicTypes": []any{},
			},
		}
		payload = append(payload, m.extraAssignments...)
		writeJSON(w, http.StatusOK, payload)
	})

	// /attributeTypes/{id} — used to pull stringType for RICH_TEXT detection
	mux.HandleFunc("GET /rest/2.0/attributeTypes/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/attributeTypes/")
		switch id {
		case defAttrID:
			st := m.defStringType
			if st == "" {
				st = "RICH_TEXT"
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"id":                         defAttrID,
				"name":                       defAttrName,
				"publicId":                   defAttrPublicID,
				"attributeTypeDiscriminator": "StringAttributeType",
				"stringType":                 st,
			})
		case noteAttrID:
			st := m.noteStringType
			if st == "" {
				st = "PLAIN_TEXT"
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"id":                         noteAttrID,
				"name":                       noteAttrName,
				"publicId":                   "Note",
				"attributeTypeDiscriminator": "StringAttributeType",
				"stringType":                 st,
			})
		default:
			http.NotFound(w, r)
		}
	})

	// /statuses
	mux.HandleFunc("GET /rest/2.0/statuses", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"results": []any{
				map[string]string{"id": candidateID, "name": candidateName},
				map[string]string{"id": "00000000-0000-0000-0000-000000005009", "name": "Accepted"},
			},
			"total": 2,
		})
	})

	// /assets (duplicate search & POST)
	mux.HandleFunc("/rest/2.0/assets", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]any{"results": m.dupResults, "total": len(m.dupResults)})
		case http.MethodPost:
			var req clients.CreateAssetRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			m.mu.Lock()
			m.createdAssets = append(m.createdAssets, req)
			m.mu.Unlock()
			code := m.createAssetCode
			if code == 0 {
				code = http.StatusCreated
			}
			if code != http.StatusCreated {
				w.WriteHeader(code)
				_, _ = w.Write([]byte(`{"message":"forced error"}`))
				return
			}
			writeJSON(w, http.StatusCreated, clients.CreateAssetResponse{
				ID:          "asset-uuid-1",
				Name:        req.Name,
				DisplayName: req.DisplayName,
				Type:        clients.CreateAssetTypeRef{ID: req.TypeID, Name: btTypeName},
				Domain:      clients.CreateAssetDomainRef{ID: req.DomainID, Name: glossaryDomain},
			})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// /attributes (POST)
	mux.HandleFunc("POST /rest/2.0/attributes", func(w http.ResponseWriter, r *http.Request) {
		var req clients.CreateAttributeRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		m.mu.Lock()
		m.createdAttributes = append(m.createdAttributes, req)
		m.mu.Unlock()
		code := m.createAttrCode
		if code == 0 {
			code = http.StatusCreated
		}
		if code != http.StatusCreated {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"message":"forced attr error"}`))
			return
		}
		writeJSON(w, http.StatusCreated, clients.CreateAttributeResponse{
			ID:    "attr-1",
			Type:  clients.CreateAttributeTypeRef{ID: req.TypeID, Name: defAttrName},
			Asset: clients.CreateAttributeAssetRef{ID: req.AssetID},
			Value: req.Value,
		})
	})

	srv := httptest.NewServer(mux)
	m.t.Cleanup(srv.Close)
	return srv
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func newClient(t *testing.T, m *mockDGC) (*http.Client, *mockDGC) {
	srv := m.server()
	return testutil.NewClient(srv), m
}

// --- Tests ---

func TestCreateAsset_HappyPathByDisplayName(t *testing.T) {
	c, m := newClient(t, newMockDGC(t))

	out, err := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:       "Customer",
		AssetType:  btTypeName,
		Domain:     glossaryDomain,
		Attributes: []create_asset.InputAttribute{{Name: defAttrName, Value: "A customer."}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != create_asset.StatusSuccess {
		t.Fatalf("status: want success, got %q (msg=%s)", out.Status, out.Message)
	}
	if out.Asset == nil || out.Asset.ID == "" {
		t.Fatalf("expected asset summary with id, got %#v", out.Asset)
	}
	if len(m.createdAssets) != 1 {
		t.Fatalf("expected 1 POST /assets, got %d", len(m.createdAssets))
	}
	if got := m.createdAssets[0].TypeID; got != btTypeID {
		t.Errorf("expected typeId resolved to %q, got %q", btTypeID, got)
	}
	if got := m.createdAssets[0].DomainID; got != glossaryDomainID {
		t.Errorf("expected domainId resolved to %q, got %q", glossaryDomainID, got)
	}
}

func TestCreateAsset_HappyPathByPublicID(t *testing.T) {
	c, _ := newClient(t, newMockDGC(t))
	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:       "Customer",
		AssetType:  btTypePublicID,
		Domain:     glossaryDomain,
		Attributes: []create_asset.InputAttribute{{Name: defAttrName, Value: "A customer."}},
	})
	if out.Status != create_asset.StatusSuccess {
		t.Fatalf("publicId resolution: want success, got %q (%s)", out.Status, out.Message)
	}
}

func TestCreateAsset_HappyPathByUUID(t *testing.T) {
	c, _ := newClient(t, newMockDGC(t))
	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:       "Customer",
		AssetType:  btTypeID,
		Domain:     glossaryDomainID,
		Attributes: []create_asset.InputAttribute{{Name: defAttrName, Value: "A customer."}},
	})
	if out.Status != create_asset.StatusSuccess {
		t.Fatalf("uuid resolution: want success, got %q (%s)", out.Status, out.Message)
	}
}

func TestCreateAsset_AssetTypeNotResolved_IncludesSuggestionsAndLicenseHint(t *testing.T) {
	c, _ := newClient(t, newMockDGC(t))
	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:      "Customer",
		AssetType: "AI Use Case",
		Domain:    glossaryDomain,
	})
	if out.Status != create_asset.StatusValidationError {
		t.Fatalf("want validation_error, got %q", out.Status)
	}
	if !strings.Contains(out.Message, "Asset types available:") {
		t.Errorf("expected suggestion list in message, got %q", out.Message)
	}
	if !strings.Contains(out.Message, "module may not be enabled") {
		t.Errorf("expected license hint in message, got %q", out.Message)
	}
}

func TestCreateAsset_DomainNotResolved_IncludesSuggestions(t *testing.T) {
	c, _ := newClient(t, newMockDGC(t))
	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:      "Customer",
		AssetType: btTypeName,
		Domain:    "Definitely Not Real",
	})
	if out.Status != create_asset.StatusValidationError {
		t.Fatalf("want validation_error, got %q", out.Status)
	}
	if !strings.Contains(out.Message, "domains available:") {
		t.Errorf("expected domain suggestions, got %q", out.Message)
	}
	if !strings.Contains(out.Message, "Glossary") {
		t.Errorf("expected suggestions filtered to Glossary domains, got %q", out.Message)
	}
}

func TestCreateAsset_DuplicateGate_DefaultBlocks(t *testing.T) {
	m := newMockDGC(t)
	m.dupResults = []asssetSearchRow{{ID: "existing-1", Name: "Customer"}}
	c, m := newClient(t, m)

	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:      "Customer",
		AssetType: btTypeName,
		Domain:    glossaryDomain,
	})
	if out.Status != create_asset.StatusDuplicateFound {
		t.Fatalf("want duplicate_found, got %q (%s)", out.Status, out.Message)
	}
	if len(m.createdAssets) != 0 {
		t.Errorf("expected no POST /assets when duplicate gate triggers, got %d", len(m.createdAssets))
	}
}

func TestCreateAsset_DuplicateGate_AllowDuplicateBypasses(t *testing.T) {
	m := newMockDGC(t)
	m.dupResults = []asssetSearchRow{{ID: "existing-1", Name: "Customer"}}
	c, m := newClient(t, m)

	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:           "Customer",
		AssetType:      btTypeName,
		Domain:         glossaryDomain,
		AllowDuplicate: true,
		Attributes:     []create_asset.InputAttribute{{Name: defAttrName, Value: "A customer."}},
	})
	if out.Status != create_asset.StatusSuccess {
		t.Fatalf("want success with allowDuplicate, got %q (%s)", out.Status, out.Message)
	}
	if len(m.createdAssets) != 1 {
		t.Errorf("expected POST /assets when allowDuplicate=true, got %d", len(m.createdAssets))
	}
}

func TestCreateAsset_AttributeByName_RichTextConvertedToHTML(t *testing.T) {
	c, m := newClient(t, newMockDGC(t))
	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:      "Customer",
		AssetType: btTypeName,
		Domain:    glossaryDomain,
		Attributes: []create_asset.InputAttribute{
			{Name: defAttrName, Value: "A **person** who buys things."},
		},
	})
	if out.Status != create_asset.StatusSuccess {
		t.Fatalf("want success, got %q (%s)", out.Status, out.Message)
	}
	if len(out.AttributeResults) != 1 {
		t.Fatalf("want 1 attribute result, got %d", len(out.AttributeResults))
	}
	r := out.AttributeResults[0]
	if r.Status != "success" {
		t.Errorf("attribute result status: want success, got %q (err=%s)", r.Status, r.Error)
	}
	if !r.ConvertedFromMd {
		t.Errorf("expected ConvertedFromMd=true for RICH_TEXT attribute")
	}
	if !strings.Contains(r.WrittenValue, "<strong>person</strong>") {
		t.Errorf("expected markdown→html conversion in WrittenValue, got %q", r.WrittenValue)
	}
	if len(m.createdAttributes) != 1 {
		t.Fatalf("expected 1 POST /attributes, got %d", len(m.createdAttributes))
	}
	if !strings.Contains(m.createdAttributes[0].Value, "<strong>person</strong>") {
		t.Errorf("converted HTML should be on the wire, got %q", m.createdAttributes[0].Value)
	}
}

func TestCreateAsset_AttributeByName_PlainTextNotConverted(t *testing.T) {
	c, m := newClient(t, newMockDGC(t))
	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:      "Customer",
		AssetType: btTypeName,
		Domain:    glossaryDomain,
		Attributes: []create_asset.InputAttribute{
			{Name: noteAttrName, Value: "**Bold** but plain-text attribute."},
			{Name: defAttrName, Value: "A customer."},
		},
	})
	if out.Status != create_asset.StatusSuccess {
		t.Fatalf("want success, got %q", out.Status)
	}
	r := out.AttributeResults[0]
	if r.ConvertedFromMd {
		t.Errorf("plain-text attribute should NOT be converted, got ConvertedFromMd=true")
	}
	if got := m.createdAttributes[0].Value; strings.Contains(got, "<strong>") {
		t.Errorf("plain-text attribute value should be untouched, got %q", got)
	}
}

func TestCreateAsset_AttributeByTypeID_Resolves(t *testing.T) {
	c, m := newClient(t, newMockDGC(t))
	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:      "Customer",
		AssetType: btTypeName,
		Domain:    glossaryDomain,
		Attributes: []create_asset.InputAttribute{
			{TypeID: defAttrID, Value: "A customer."},
			{TypeID: noteAttrID, Value: "Plain note."},
		},
	})
	if out.Status != create_asset.StatusSuccess {
		t.Fatalf("want success, got %q (%s)", out.Status, out.Message)
	}
	if got := m.createdAttributes[1].TypeID; got != noteAttrID {
		t.Errorf("expected typeId %q, got %q", noteAttrID, got)
	}
}

func TestCreateAsset_UnknownAttributeName_ReturnsValidationError(t *testing.T) {
	c, _ := newClient(t, newMockDGC(t))
	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:      "Customer",
		AssetType: btTypeName,
		Domain:    glossaryDomain,
		Attributes: []create_asset.InputAttribute{
			{Name: "Bogus Field", Value: "x"},
		},
	})
	if out.Status != create_asset.StatusValidationError {
		t.Fatalf("want validation_error, got %q", out.Status)
	}
	if !strings.Contains(out.Message, "Attributes available:") {
		t.Errorf("expected attribute suggestions in message, got %q", out.Message)
	}
}

func TestCreateAsset_StatusByName(t *testing.T) {
	c, m := newClient(t, newMockDGC(t))
	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:       "Customer",
		AssetType:  btTypeName,
		Domain:     glossaryDomain,
		Status:     candidateName,
		Attributes: []create_asset.InputAttribute{{Name: defAttrName, Value: "A customer."}},
	})
	if out.Status != create_asset.StatusSuccess {
		t.Fatalf("want success, got %q", out.Status)
	}
	if got := m.createdAssets[0].StatusID; got != candidateID {
		t.Errorf("expected status resolved to %q, got %q", candidateID, got)
	}
}

func TestCreateAsset_StatusUnknown_ReturnsValidationError(t *testing.T) {
	c, _ := newClient(t, newMockDGC(t))
	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:       "Customer",
		AssetType:  btTypeName,
		Domain:     glossaryDomain,
		Status:     "Nonexistent Status",
		Attributes: []create_asset.InputAttribute{{Name: defAttrName, Value: "A customer."}},
	})
	if out.Status != create_asset.StatusValidationError {
		t.Fatalf("want validation_error, got %q", out.Status)
	}
	if !strings.Contains(out.Message, "Statuses available:") {
		t.Errorf("expected status suggestions, got %q", out.Message)
	}
}

func TestCreateAsset_AttributeFailure_PreservesAssetSuccess(t *testing.T) {
	m := newMockDGC(t)
	m.createAttrCode = http.StatusNotFound
	c, _ := newClient(t, m)

	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:      "Customer",
		AssetType: btTypeName,
		Domain:    glossaryDomain,
		Attributes: []create_asset.InputAttribute{
			{Name: defAttrName, Value: "anything"},
		},
	})
	if out.Status != create_asset.StatusSuccess {
		t.Fatalf("asset itself should still be success, got %q (%s)", out.Status, out.Message)
	}
	if len(out.AttributeResults) != 1 || out.AttributeResults[0].Status != "error" {
		t.Errorf("expected per-attribute error in AttributeResults, got %#v", out.AttributeResults)
	}
}

// nowhere sub-case (a): no assignment located anywhere in the type's
// hierarchy → "can't be created in any domain on this instance."
func TestCreateAsset_NoAssignmentAnywhere_NowhereBranch(t *testing.T) {
	m := newMockDGC(t)
	m.noAssignments = true
	c, m := newClient(t, m)

	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:      "Test",
		AssetType: btTypeName,
		Domain:    glossaryDomain,
	})
	if out.Status != create_asset.StatusValidationError {
		t.Fatalf("want validation_error, got %q (%s)", out.Status, out.Message)
	}
	if !strings.Contains(out.Message, "can't be created in any domain") {
		t.Errorf("expected nowhere-branch message, got %q", out.Message)
	}
	if len(m.createdAssets) != 0 {
		t.Errorf("no create must be attempted when the type is not creatable, got %d", len(m.createdAssets))
	}
}

// nowhere sub-case (b): a level is located but every assignment on it has empty
// domainTypes → the same "can't be created in any domain" verdict.
func TestCreateAsset_AllEmptyDomainTypes_NowhereBranch(t *testing.T) {
	m := newMockDGC(t)
	m.emptyDomainTypes = true
	c, m := newClient(t, m)

	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:      "Test",
		AssetType: btTypeName,
		Domain:    glossaryDomain,
	})
	if out.Status != create_asset.StatusValidationError {
		t.Fatalf("want validation_error, got %q (%s)", out.Status, out.Message)
	}
	if !strings.Contains(out.Message, "can't be created in any domain") {
		t.Errorf("expected nowhere-branch message, got %q", out.Message)
	}
	if len(m.createdAssets) != 0 {
		t.Errorf("no create must be attempted, got %d", len(m.createdAssets))
	}
}

// The two nowhere sub-cases (no assignment anywhere; all-empty domainTypes at
// the located level) are deliberately collapsed into ONE message.
func TestCreateAsset_BothNowhereSubcases_IdenticalMessage(t *testing.T) {
	run := func(configure func(*mockDGC)) string {
		m := newMockDGC(t)
		configure(m)
		c, _ := newClient(t, m)
		out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
			Name: "Test", AssetType: btTypeName, Domain: glossaryDomain,
		})
		return out.Message
	}
	noAssignment := run(func(m *mockDGC) { m.noAssignments = true })
	allEmpty := run(func(m *mockDGC) { m.emptyDomainTypes = true })
	if noAssignment != allEmpty {
		t.Errorf("nowhere sub-cases must produce identical messages:\n  no-assignment: %q\n  all-empty:     %q", noAssignment, allEmpty)
	}
}

// not-here branch: the domain's type is not governed by the type's assignment,
// but the type IS creatable somewhere → "isn't allowed in domain ...". No
// create is attempted, and the allowed-type list must not leak into the text.
func TestCreateAsset_NotHereBranch_NoCreate(t *testing.T) {
	m := newMockDGC(t)
	m.domainTypeOther = true
	c, m := newClient(t, m)

	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:      "Customer",
		AssetType: btTypeName,
		Domain:    glossaryDomain,
	})
	if out.Status != create_asset.StatusValidationError {
		t.Fatalf("want validation_error, got %q (%s)", out.Status, out.Message)
	}
	if !strings.Contains(out.Message, "isn't allowed in domain") {
		t.Errorf("expected not-here-branch message, got %q", out.Message)
	}
	if !strings.Contains(out.Message, "Pick a different asset type, or a different domain") {
		t.Errorf("expected not-here recovery hint, got %q", out.Message)
	}
	if len(m.createdAssets) != 0 {
		t.Errorf("no create must be attempted for a not-allowed type, got %d", len(m.createdAssets))
	}
	// (No-list-leak is asserted in TestNotAllowedMessage_ParityAcrossCallers,
	// which uses a distinctive allowed-type name; here "Glossary" collides with
	// the domain name "My Glossary".)
}

const (
	acronymTypeID       = "00000000-0000-0000-0000-000000011003"
	acronymTypeName     = "Acronym"
	acronymTypePublicID = "Acronym"
)

// newAcronymSubtypeClient boots a mock DGC for the Acronym → BusinessTerm
// subtype world: Acronym has NO assignment of its own, so resolution walks up
// to the parent BusinessTerm, whose assignment lists the Glossary domain type
// and the required (min:1) Definition attribute. We mock both nodes to mirror
// the live shape. Shared by the walk-up and parent-required-attribute tests.
func newAcronymSubtypeClient(t *testing.T) *http.Client {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/assetTypes/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/rest/2.0/assetTypes/")
		switch path {
		case "publicId/" + acronymTypePublicID, acronymTypeID:
			writeJSON(w, http.StatusOK, map[string]any{
				"id":       acronymTypeID,
				"publicId": acronymTypePublicID,
				"name":     acronymTypeName,
				"parent": map[string]any{
					"id":   btTypeID,
					"name": btTypeName,
				},
			})
		case "publicId/" + btTypePublicID, btTypeID:
			writeJSON(w, http.StatusOK, map[string]any{
				"id":       btTypeID,
				"publicId": btTypePublicID,
				"name":     btTypeName,
			})
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("GET /rest/2.0/assetTypes", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"results": []any{}, "total": 0})
	})
	mux.HandleFunc("GET /rest/2.0/domains/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/domains/")
		if id == glossaryDomainID {
			writeJSON(w, http.StatusOK, map[string]any{
				"id":   glossaryDomainID,
				"name": glossaryDomain,
				"type": map[string]string{"id": glossaryTypeID, "name": glossaryTypeName},
			})
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("GET /rest/2.0/domains", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"results": []any{
			map[string]any{
				"id":   glossaryDomainID,
				"name": glossaryDomain,
				"type": map[string]string{"id": glossaryTypeID, "name": glossaryTypeName},
			},
		}, "total": 1})
	})
	// Acronym has no assignment of its own, so resolution walks up to its
	// parent BusinessTerm, whose assignment lists the Glossary domainType and
	// the canonical "Definition" attribute. That single ancestor level is used
	// whole — there is no cross-level union.
	mux.HandleFunc("GET /rest/2.0/assignments/assetType/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/assignments/assetType/")
		switch id {
		case acronymTypeID:
			writeJSON(w, http.StatusOK, []any{})
		case btTypeID:
			writeJSON(w, http.StatusOK, []map[string]any{{
				"id":          "asgn-bt",
				"domainTypes": []map[string]string{{"id": glossaryTypeID, "name": glossaryTypeName}},
				"assignedCharacteristicTypeReferences": []map[string]any{
					{
						"id": "ref-def",
						"assignedResourceReference": map[string]string{
							"id": defAttrID, "name": defAttrName,
							"resourceType": "StringAttributeType", "resourceDiscriminator": "StringAttributeType",
						},
						"assignedResourcePublicId": "Definition",
						"minimumOccurrences":       1,
					},
				},
				"characteristicTypes": []any{},
			}})
		default:
			writeJSON(w, http.StatusOK, []any{})
		}
	})
	mux.HandleFunc("GET /rest/2.0/attributeTypes/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/attributeTypes/")
		if id == defAttrID {
			writeJSON(w, http.StatusOK, map[string]any{
				"id":                         defAttrID,
				"name":                       defAttrName,
				"publicId":                   "Definition",
				"attributeTypeDiscriminator": "StringAttributeType",
				"stringType":                 "RICH_TEXT",
			})
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("GET /rest/2.0/statuses", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"results": []any{}, "total": 0})
	})
	mux.HandleFunc("/rest/2.0/assets", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]any{"results": []any{}, "total": 0})
		case http.MethodPost:
			var req clients.CreateAssetRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			writeJSON(w, http.StatusCreated, clients.CreateAssetResponse{
				ID:          "asset-acronym-1",
				Name:        req.Name,
				DisplayName: req.DisplayName,
				Type:        clients.CreateAssetTypeRef{ID: req.TypeID, Name: acronymTypeName},
				Domain:      clients.CreateAssetDomainRef{ID: req.DomainID, Name: glossaryDomain},
			})
		}
	})
	mux.HandleFunc("POST /rest/2.0/attributes", func(w http.ResponseWriter, r *http.Request) {
		var req clients.CreateAttributeRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		writeJSON(w, http.StatusCreated, clients.CreateAttributeResponse{
			ID:    "attr-1",
			Type:  clients.CreateAttributeTypeRef{ID: req.TypeID, Name: defAttrName},
			Asset: clients.CreateAttributeAssetRef{ID: req.AssetID},
			Value: req.Value,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return testutil.NewClient(srv)
}

// Resolving Acronym + Glossary should walk up to BusinessTerm (Acronym's
// first assigned ancestor), find Glossary in its allowed types, and use that
// level's characteristics whole.
func TestCreateAsset_Subtype_InheritsParentDomainTypes(t *testing.T) {
	c := newAcronymSubtypeClient(t)

	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:      "MRR",
		AssetType: acronymTypeName,
		Domain:    glossaryDomain,
		Attributes: []create_asset.InputAttribute{
			{Name: defAttrName, Value: "Monthly Recurring Revenue"},
		},
	})
	if out.Status != create_asset.StatusSuccess {
		t.Fatalf("subtype Acronym in Glossary should succeed via parent walk, got status=%q msg=%q", out.Status, out.Message)
	}
	if len(out.AttributeResults) != 1 || out.AttributeResults[0].Status != "success" {
		t.Errorf("expected the ancestor's Definition attribute to resolve via walk-up, got %#v", out.AttributeResults)
	}
}

// An attribute required only on a PARENT asset type's assignment must not
// block the create: Definition is required (min:1) on
// BusinessTerm, but Acronym's own assignment doesn't list it, so creating
// an Acronym without it succeeds — matching the Core API and the UI.
func TestCreateAsset_ParentRequiredAttribute_DoesNotBlockCreate(t *testing.T) {
	c := newAcronymSubtypeClient(t)

	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:      "MRR",
		AssetType: acronymTypeName,
		Domain:    glossaryDomain,
		// No attributes: the parent's required Definition is omitted.
	})
	if out.Status != create_asset.StatusSuccess {
		t.Fatalf("parent-required attribute must not block create, got %q (%s)", out.Status, out.Message)
	}
}

// Definition is required (min:1) on BusinessTerm's OWN assignment, so the
// gate applies. Contrast with TestCreateAsset_ParentRequiredAttribute_
// DoesNotBlockCreate, where the requirement lives on a parent type only.
func TestCreateAsset_MissingRequiredAttribute_ReturnsValidationError(t *testing.T) {
	c, _ := newClient(t, newMockDGC(t))
	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:      "Customer",
		AssetType: btTypeName,
		Domain:    glossaryDomain,
		// Definition is required (minimumOccurrences: 1) but omitted.
	})
	if out.Status != create_asset.StatusValidationError {
		t.Fatalf("want validation_error for missing required attribute, got %q (%s)", out.Status, out.Message)
	}
	if !strings.Contains(out.Message, "Definition") {
		t.Errorf("expected missing attribute name in message, got %q", out.Message)
	}
}

func TestCreateAsset_RequiredFieldsMissing(t *testing.T) {
	c, _ := newClient(t, newMockDGC(t))
	cases := []create_asset.Input{
		{},
		{Name: "x"},
		{Name: "x", AssetType: btTypeName},
	}
	for i, in := range cases {
		out, err := create_asset.NewTool(c).Handler(t.Context(), in)
		if err != nil {
			t.Errorf("[%d] unexpected error: %v", i, err)
		}
		if out.Status != create_asset.StatusValidationError {
			t.Errorf("[%d] want validation_error for %#v, got %q", i, in, out.Status)
		}
	}
}

// Regression for the 2026-07-20 incident: Business Term carries, next to its
// global assignment, an assignment scoped to a "Pricebooks" scope covering
// other domains. Both share the Glossary domain type, and the scoped
// assignment's required Pricebook attributes were unioned into the required
// set, blocking every Business Term create in ordinary glossary domains.
// An assignment whose scope does not cover the target domain must be invisible.
func TestCreateAsset_ScopedAssignmentElsewhere_DoesNotBlockCreate(t *testing.T) {
	m := newMockDGC(t)
	m.extraAssignments = []map[string]any{{
		"id": "assignment-pricebooks",
		"domainTypes": []map[string]string{
			{"id": glossaryTypeID, "name": glossaryTypeName},
		},
		"scope": map[string]any{
			"id":   "00000000-0000-0000-0000-000000077001",
			"name": "Pricebooks",
			"domains": []map[string]string{
				{"id": "00000000-0000-0000-0000-000000099777", "name": "Pricebook 4"},
			},
		},
		"assignedCharacteristicTypeReferences": []map[string]any{
			{
				"id": "ref-pb-premier",
				"assignedResourceReference": map[string]string{
					"id":                    "00000000-0000-0000-0000-000000000777",
					"name":                  "Pricebook 4 - Premier Package",
					"resourceType":          "StringAttributeType",
					"resourceDiscriminator": "StringAttributeType",
				},
				"assignedResourcePublicId": "Pricebook4PremierPackage",
				"minimumOccurrences":       1,
			},
		},
		"characteristicTypes": []any{},
	}}
	c, m := newClient(t, m)

	out, err := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name:       "Business Associate Agreement",
		AssetType:  btTypeName,
		Domain:     glossaryDomain,
		Attributes: []create_asset.InputAttribute{{Name: defAttrName, Value: "A legally binding contract."}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != create_asset.StatusSuccess {
		t.Fatalf("scoped assignment covering another domain must not block the create, got %q (%s)", out.Status, out.Message)
	}
	if len(m.createdAssets) != 1 {
		t.Fatalf("expected 1 POST /assets, got %d", len(m.createdAssets))
	}
}

// Core regression: the asset type's OWN located assignment does not list the
// target domain type (it lists a different one), while an ANCESTOR's assignment
// does. Creatability must be judged against the own located level — the create
// is refused and NOT resolved by climbing to the ancestor that would allow it.
// This is the central defect the single-assignment-selection PR fixes.
func TestCreateAsset_OwnLevelDoesNotGovernTarget_RefusedNotClimbed(t *testing.T) {
	const otherTypeID = "00000000-0000-0000-0000-0000000100aa"
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/assetTypes/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/rest/2.0/assetTypes/")
		switch path {
		case "publicId/" + acronymTypePublicID, acronymTypeID:
			writeJSON(w, http.StatusOK, map[string]any{
				"id": acronymTypeID, "publicId": acronymTypePublicID, "name": acronymTypeName,
				"parent": map[string]any{"id": btTypeID, "name": btTypeName},
			})
		case "publicId/" + btTypePublicID, btTypeID:
			writeJSON(w, http.StatusOK, map[string]any{"id": btTypeID, "publicId": btTypePublicID, "name": btTypeName})
		default:
			http.NotFound(w, r)
		}
	})
	mux.HandleFunc("GET /rest/2.0/assetTypes", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"results": []any{}, "total": 0})
	})
	mux.HandleFunc("GET /rest/2.0/domains/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/domains/")
		if id == glossaryDomainID {
			writeJSON(w, http.StatusOK, map[string]any{
				"id": glossaryDomainID, "name": glossaryDomain,
				"type": map[string]string{"id": glossaryTypeID, "name": glossaryTypeName},
			})
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("GET /rest/2.0/domains", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"results": []any{
			map[string]any{"id": glossaryDomainID, "name": glossaryDomain,
				"type": map[string]string{"id": glossaryTypeID, "name": glossaryTypeName}},
		}, "total": 1})
	})
	// Acronym's OWN assignment lists "Other Domain Type", not Glossary. Its
	// parent BusinessTerm lists Glossary — the target domain's type.
	mux.HandleFunc("GET /rest/2.0/assignments/assetType/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/assignments/assetType/")
		switch id {
		case acronymTypeID:
			writeJSON(w, http.StatusOK, []map[string]any{{
				"id":          "asgn-acronym-own",
				"domainTypes": []map[string]string{{"id": otherTypeID, "name": "Other Domain Type"}},
			}})
		case btTypeID:
			writeJSON(w, http.StatusOK, []map[string]any{{
				"id":          "asgn-bt",
				"domainTypes": []map[string]string{{"id": glossaryTypeID, "name": glossaryTypeName}},
			}})
		default:
			writeJSON(w, http.StatusOK, []any{})
		}
	})
	created := 0
	mux.HandleFunc("/rest/2.0/assets", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			created++
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": []any{}, "total": 0})
	})
	mux.HandleFunc("GET /rest/2.0/statuses", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"results": []any{}, "total": 0})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	c := testutil.NewClient(srv)

	out, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
		Name: "MRR", AssetType: acronymTypeName, Domain: glossaryDomain,
	})
	if out.Status != create_asset.StatusValidationError {
		t.Fatalf("create must be refused on the own level's coverage, got %q (%s)", out.Status, out.Message)
	}
	// The own level lists a domain type (Other), so the type IS creatable
	// somewhere — the not-here branch, not nowhere.
	if !strings.Contains(out.Message, "isn't allowed in domain") {
		t.Errorf("expected not-here-branch message, got %q", out.Message)
	}
	if created != 0 {
		t.Errorf("no create must be attempted; the ancestor's Glossary assignment must NOT be climbed to, got %d creates", created)
	}
}
