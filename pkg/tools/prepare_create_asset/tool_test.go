package prepare_create_asset_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/clients"
	"github.com/collibra/chip/pkg/tools/prepare_create_asset"
	"github.com/collibra/chip/pkg/tools/testutil"
)

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
	noteAttrID       = "00000000-0000-0000-0000-0000000003116"
	noteAttrName     = "Note"
)

// Mock fixture for the consolidated /assignments shape. Kept local rather
// than shared with create_asset's mock because the two tools have distinct
// surfaces and a shared fixture would couple their wire-format assumptions.

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

type mockDGC struct {
	t               *testing.T
	excludeBT       bool // simulate license-gated asset type missing from /assetTypes
	domainTypeOther bool // domain returns a non-Glossary type
	noAssignments   bool // /assignments/assetType/{id} returns [] (subtype with inherited assignments)
}

func (m *mockDGC) server() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /rest/2.0/assetTypes/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/rest/2.0/assetTypes/")
		if strings.HasPrefix(path, "publicId/") {
			pid := strings.TrimPrefix(path, "publicId/")
			if !m.excludeBT && pid == btTypePublicID {
				writeJSON(w, http.StatusOK, assetTypeRow{ID: btTypeID, PublicID: btTypePublicID, Name: btTypeName})
				return
			}
			http.NotFound(w, r)
			return
		}
		if !m.excludeBT && path == btTypeID {
			writeJSON(w, http.StatusOK, assetTypeRow{ID: btTypeID, PublicID: btTypePublicID, Name: btTypeName})
			return
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc("GET /rest/2.0/assetTypes", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("name")
		var rows []assetTypeRow
		if name != "" {
			if !m.excludeBT && strings.EqualFold(name, btTypeName) {
				rows = []assetTypeRow{{ID: btTypeID, PublicID: btTypePublicID, Name: btTypeName}}
			}
		} else {
			rows = []assetTypeRow{
				{ID: "00000000-0000-0000-0000-000000011002", PublicID: "Code", Name: "Code"},
				{ID: "00000000-0000-0000-0000-000000011003", PublicID: "Column", Name: "Column"},
			}
			if !m.excludeBT {
				rows = append(rows, assetTypeRow{ID: btTypeID, PublicID: btTypePublicID, Name: btTypeName})
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": rows, "total": len(rows)})
	})

	mux.HandleFunc("GET /rest/2.0/domains/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/domains/")
		if id == glossaryDomainID {
			d := domainRow{ID: glossaryDomainID, Name: glossaryDomain, Type: &clients.PrepareCreateDomainType{ID: glossaryTypeID, Name: glossaryTypeName}}
			if m.domainTypeOther {
				d.Type = &clients.PrepareCreateDomainType{ID: "00000000-0000-0000-0000-000000010099", Name: "Other Domain Type"}
			}
			writeJSON(w, http.StatusOK, d)
			return
		}
		http.NotFound(w, r)
	})

	mux.HandleFunc("GET /rest/2.0/domains", func(w http.ResponseWriter, r *http.Request) {
		domainType := &clients.PrepareCreateDomainType{ID: glossaryTypeID, Name: glossaryTypeName}
		if m.domainTypeOther {
			domainType = &clients.PrepareCreateDomainType{ID: "00000000-0000-0000-0000-000000010099", Name: "Other Domain Type"}
		}
		name := r.URL.Query().Get("name")
		var rows []domainRow
		if name != "" {
			if strings.EqualFold(name, glossaryDomain) {
				rows = []domainRow{{ID: glossaryDomainID, Name: glossaryDomain, Type: domainType}}
			}
		} else {
			rows = []domainRow{{ID: glossaryDomainID, Name: glossaryDomain, Type: domainType}}
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": rows, "total": len(rows)})
	})

	mux.HandleFunc("GET /rest/2.0/assignments/domain/", func(w http.ResponseWriter, r *http.Request) {
		// /assignments/domain/{id}/assetTypes — used by enumerateAssetTypesForDomain
		writeJSON(w, http.StatusOK, []assetTypeRow{
			{ID: btTypeID, PublicID: btTypePublicID, Name: btTypeName},
			{ID: "00000000-0000-0000-0000-000000011099", PublicID: "Acronym", Name: "Acronym"},
		})
	})

	mux.HandleFunc("GET /rest/2.0/assignments/assetType/", func(w http.ResponseWriter, r *http.Request) {
		if m.noAssignments {
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		// When domain type doesn't match the assignment's domainTypes, the
		// reducer will surface "no scoped assignment" — we still serve the
		// canonical assignment so that branch is exercised in the reducer.
		writeJSON(w, http.StatusOK, []map[string]any{{
			"id":          "asgn-1",
			"domainTypes": []map[string]string{{"id": glossaryTypeID, "name": glossaryTypeName}},
			"assignedCharacteristicTypeReferences": []map[string]any{
				{
					"id": "ref-def",
					"assignedResourceReference": map[string]string{
						"id": defAttrID, "name": defAttrName, "resourceType": "StringAttributeType", "resourceDiscriminator": "StringAttributeType",
					},
					"assignedResourcePublicId": "Definition",
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
		}})
	})

	mux.HandleFunc("GET /rest/2.0/attributeTypes/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/rest/2.0/attributeTypes/")
		switch id {
		case defAttrID:
			writeJSON(w, http.StatusOK, map[string]any{
				"id":                         defAttrID,
				"name":                       defAttrName,
				"publicId":                   "Definition",
				"attributeTypeDiscriminator": "StringAttributeType",
				"stringType":                 "RICH_TEXT",
				"description":                "The definition.",
			})
		case noteAttrID:
			writeJSON(w, http.StatusOK, map[string]any{
				"id":                         noteAttrID,
				"name":                       noteAttrName,
				"publicId":                   "Note",
				"attributeTypeDiscriminator": "StringAttributeType",
				"stringType":                 "PLAIN_TEXT",
			})
		default:
			http.NotFound(w, r)
		}
	})

	mux.HandleFunc("GET /rest/2.0/statuses", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"results": []any{
				map[string]string{"id": "00000000-0000-0000-0000-000000005008", "name": "Candidate"},
				map[string]string{"id": "00000000-0000-0000-0000-000000005009", "name": "Accepted"},
			},
			"total": 2,
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

func client(t *testing.T, m *mockDGC) *http.Client {
	srv := m.server()
	return testutil.NewClient(srv)
}

// --- Tests ---

func TestPrepare_NoInputs_EnumeratesAssetTypes(t *testing.T) {
	c := client(t, &mockDGC{t: t})
	out, _ := prepare_create_asset.NewTool(c).Handler(t.Context(), prepare_create_asset.Input{})
	if out.Status != prepare_create_asset.StatusIncomplete {
		t.Fatalf("want incomplete, got %q", out.Status)
	}
	if len(out.AssetTypeOptions) == 0 {
		t.Errorf("expected asset type options, got none")
	}
}

func TestPrepare_DomainOnly_EnumeratesAssetTypesAllowedInDomain(t *testing.T) {
	c := client(t, &mockDGC{t: t})
	out, _ := prepare_create_asset.NewTool(c).Handler(t.Context(), prepare_create_asset.Input{
		Domain: glossaryDomain,
	})
	if out.Status != prepare_create_asset.StatusIncomplete {
		t.Fatalf("want incomplete, got %q (%s)", out.Status, out.Message)
	}
	if len(out.AssetTypeOptions) == 0 {
		t.Fatalf("expected asset type options scoped to the domain, got none")
	}
	if !strings.Contains(out.Message, glossaryDomain) {
		t.Errorf("expected domain name in message, got %q", out.Message)
	}
	if !strings.Contains(out.Message, glossaryTypeName) {
		t.Errorf("expected domain type name in message, got %q", out.Message)
	}
	// Verify the list came from /assignments/domain/{id}/assetTypes — not
	// the global enumeration. The mock returns BusinessTerm + Acronym there
	// (a deliberately small set unlike the global list).
	names := map[string]bool{}
	for _, o := range out.AssetTypeOptions {
		names[o.Name] = true
	}
	if !names[btTypeName] {
		t.Errorf("expected %q in domain-scoped options, got %v", btTypeName, names)
	}
}

func TestPrepare_AssetTypeOnly_EnumeratesDomains(t *testing.T) {
	c := client(t, &mockDGC{t: t})
	out, _ := prepare_create_asset.NewTool(c).Handler(t.Context(), prepare_create_asset.Input{
		AssetType: btTypeName,
	})
	if out.Status != prepare_create_asset.StatusIncomplete {
		t.Fatalf("want incomplete, got %q", out.Status)
	}
	if len(out.DomainOptions) == 0 {
		t.Errorf("expected domain options, got none")
	}
}

func TestPrepare_BothResolved_ReturnsReadyWithSchema(t *testing.T) {
	c := client(t, &mockDGC{t: t})
	out, _ := prepare_create_asset.NewTool(c).Handler(t.Context(), prepare_create_asset.Input{
		AssetType: btTypeName,
		Domain:    glossaryDomain,
	})
	if out.Status != prepare_create_asset.StatusReady {
		t.Fatalf("want ready, got %q (%s)", out.Status, out.Message)
	}
	if out.Resolved == nil {
		t.Fatalf("expected resolved")
	}
	if out.Resolved.AssetTypeID != btTypeID || out.Resolved.DomainID != glossaryDomainID {
		t.Errorf("resolved IDs: %#v", out.Resolved)
	}
	if len(out.AttributeSchema) != 2 {
		t.Errorf("expected 2 attribute slots in schema, got %d", len(out.AttributeSchema))
	}
	var def, note prepare_create_asset.AttributeSchemaEntry
	for _, e := range out.AttributeSchema {
		if e.AttributeTypeID == defAttrID {
			def = e
		}
		if e.AttributeTypeID == noteAttrID {
			note = e
		}
	}
	if !def.Required {
		t.Errorf("expected Definition to be required")
	}
	if note.Required {
		t.Errorf("expected Note not to be required")
	}
	if def.Kind != "StringAttributeType" {
		t.Errorf("expected Kind=StringAttributeType, got %q", def.Kind)
	}
	if def.StringType != "" {
		t.Errorf("StringType should be empty without includeStringType, got %q", def.StringType)
	}
}

func TestPrepare_IncludeStringType_HydratesDetails(t *testing.T) {
	c := client(t, &mockDGC{t: t})
	out, _ := prepare_create_asset.NewTool(c).Handler(t.Context(), prepare_create_asset.Input{
		AssetType:         btTypeName,
		Domain:            glossaryDomain,
		IncludeStringType: true,
	})
	if out.Status != prepare_create_asset.StatusReady {
		t.Fatalf("want ready, got %q (%s)", out.Status, out.Message)
	}
	var def prepare_create_asset.AttributeSchemaEntry
	for _, e := range out.AttributeSchema {
		if e.AttributeTypeID == defAttrID {
			def = e
		}
	}
	if def.StringType != "RICH_TEXT" {
		t.Errorf("expected stringType=RICH_TEXT after hydration, got %q", def.StringType)
	}
	if def.Description == "" {
		t.Errorf("expected description after hydration")
	}
}

func TestPrepare_AssetTypeNotResolved_IncludesLicenseHint(t *testing.T) {
	c := client(t, &mockDGC{t: t, excludeBT: true})
	out, _ := prepare_create_asset.NewTool(c).Handler(t.Context(), prepare_create_asset.Input{
		AssetType: btTypeName,
		Domain:    glossaryDomain,
	})
	if out.Status != prepare_create_asset.StatusNeedsClarification {
		t.Fatalf("want needs_clarification, got %q (%s)", out.Status, out.Message)
	}
	if !strings.Contains(out.Message, "module may not be enabled") {
		t.Errorf("expected DEV-177761 license hint, got %q", out.Message)
	}
	if len(out.AssetTypeOptions) == 0 {
		t.Errorf("expected asset type options to recover from")
	}
}

func TestPrepare_TypeNotAllowedInDomain(t *testing.T) {
	c := client(t, &mockDGC{t: t, domainTypeOther: true})
	out, _ := prepare_create_asset.NewTool(c).Handler(t.Context(), prepare_create_asset.Input{
		AssetType: btTypeName,
		Domain:    glossaryDomain,
	})
	if out.Status != prepare_create_asset.StatusNeedsClarification {
		t.Fatalf("want needs_clarification, got %q", out.Status)
	}
	if !strings.Contains(out.Message, "not allowed in domain") {
		t.Errorf("expected 'not allowed in domain' message, got %q", out.Message)
	}
}

func TestPrepare_AssetTypeWithNoAssignments_ReturnsNoCompatibleDomains(t *testing.T) {
	c := client(t, &mockDGC{t: t, noAssignments: true})
	out, _ := prepare_create_asset.NewTool(c).Handler(t.Context(), prepare_create_asset.Input{
		AssetType: btTypeName,
	})
	if out.Status != prepare_create_asset.StatusNeedsClarification {
		t.Fatalf("want needs_clarification, got %q (%s)", out.Status, out.Message)
	}
	if len(out.DomainOptions) != 0 {
		t.Errorf("expected empty DomainOptions, got %d", len(out.DomainOptions))
	}
	if !strings.Contains(out.Message, "No compatible domains") {
		t.Errorf("expected factual no-compatible-domains message, got %q", out.Message)
	}
}

func TestPrepare_AvailableStatusesAlwaysIncluded(t *testing.T) {
	c := client(t, &mockDGC{t: t})
	out, _ := prepare_create_asset.NewTool(c).Handler(t.Context(), prepare_create_asset.Input{
		AssetType: btTypeName,
		Domain:    glossaryDomain,
	})
	if out.Status != prepare_create_asset.StatusReady {
		t.Fatalf("want ready, got %q", out.Status)
	}
	if len(out.AvailableStatuses) == 0 {
		t.Errorf("expected availableStatuses to be populated")
	}
}
