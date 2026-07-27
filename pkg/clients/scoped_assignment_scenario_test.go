package clients

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/collibra/chip/pkg/tools/testutil"
)

type scenario struct {
	Description  string               `json:"description,omitempty"`
	AssetTypes   []scenarioAssetType  `json:"assetTypes"`
	DomainTypes  []scenarioDomainType `json:"domainTypes,omitempty"`
	Organization []scenarioOrgNode    `json:"organization,omitempty"`
	Scopes       []scenarioScope      `json:"scopes,omitempty"`
	Assignments  []scenarioAssignment `json:"assignments,omitempty"`
}

type scenarioAssetType struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Parent string `json:"parent,omitempty"`
}

type scenarioDomainType struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type scenarioOrgNode struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Type       string `json:"type"`
	Parent     string `json:"parent,omitempty"`
	DomainType string `json:"domainType,omitempty"`
}

type scenarioScope struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Domains     []string `json:"domains,omitempty"`
	Communities []string `json:"communities,omitempty"`
}

type scenarioAssignment struct {
	ID             string              `json:"id"`
	AssetType      string              `json:"assetType"`
	Scope          string              `json:"scope,omitempty"`
	DomainTypes    []string            `json:"domainTypes"`
	Attributes     []scenarioAttribute `json:"attributes,omitempty"`
	Relations      []scenarioRelation  `json:"relations,omitempty"`
	Traits         []scenarioTrait     `json:"traits,omitempty"`
	AncestorTraits []scenarioTrait     `json:"ancestorTraits,omitempty"`
}

type scenarioTrait struct {
	Attributes []scenarioAttribute `json:"attributes,omitempty"`
	Relations  []scenarioRelation  `json:"relations,omitempty"`
}

type scenarioAttribute struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	PublicID string `json:"publicId,omitempty"`
	Kind     string `json:"kind,omitempty"`
	Min      int    `json:"min,omitempty"`
	Max      *int   `json:"max,omitempty"`
}

type scenarioRelation struct {
	ID            string  `json:"id"`
	PublicID      string  `json:"publicId,omitempty"`
	Direction     string  `json:"direction,omitempty"`
	Restriction   string  `json:"restriction,omitempty"`
	Discriminator *string `json:"discriminator,omitempty"`
}

type scenarioHarness struct {
	t      *testing.T
	sc     scenario
	client *http.Client

	assetTypesByName  map[string]scenarioAssetType
	domainTypesByName map[string]scenarioDomainType
	domainsByName     map[string]scenarioOrgNode
	communitiesByName map[string]scenarioOrgNode
	scopesByName      map[string]scenarioScope
}

func newScenarioHarness(t *testing.T, name string) *scenarioHarness {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "scoped_assignment", name+".json"))
	if err != nil {
		t.Fatalf("reading scenario %q: %v", name, err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var sc scenario
	if err := dec.Decode(&sc); err != nil {
		t.Fatalf("decoding scenario %q: %v", name, err)
	}

	h := &scenarioHarness{
		t:                 t,
		sc:                sc,
		assetTypesByName:  make(map[string]scenarioAssetType),
		domainTypesByName: make(map[string]scenarioDomainType),
		domainsByName:     make(map[string]scenarioOrgNode),
		communitiesByName: make(map[string]scenarioOrgNode),
		scopesByName:      make(map[string]scenarioScope),
	}
	h.index()
	h.validate()
	h.startServer()
	return h
}

func (h *scenarioHarness) resolveScopedAssignment(assetType, domain string) (*PrepareCreateScopedAssignment, error) {
	h.t.Helper()
	d := h.domain(domain)
	return GetScopedAssignment(h.t.Context(), h.client, h.assetType(assetType).ID, h.domainType(d.DomainType).ID, d.ID)
}

func (h *scenarioHarness) listAllowedDomainTypes(assetType string) ([]PrepareCreateAllowedDomainType, error) {
	h.t.Helper()
	return ListAllowedDomainTypesForAssetType(h.t.Context(), h.client, h.assetType(assetType).ID)
}

func (h *scenarioHarness) index() {
	for _, at := range h.sc.AssetTypes {
		h.assetTypesByName[at.Name] = at
	}
	for _, dt := range h.sc.DomainTypes {
		h.domainTypesByName[dt.Name] = dt
	}
	for _, n := range h.sc.Organization {
		switch n.Type {
		case "Domain":
			h.domainsByName[n.Name] = n
		case "Community":
			h.communitiesByName[n.Name] = n
		default:
			h.t.Fatalf("organization node %q has unknown type %q (want Domain or Community)", n.Name, n.Type)
		}
	}
	for _, s := range h.sc.Scopes {
		h.scopesByName[s.Name] = s
	}
}

func (h *scenarioHarness) validate() {
	for _, at := range h.sc.AssetTypes {
		if at.Parent != "" {
			h.assetType(at.Parent)
		}
	}
	for _, n := range h.sc.Organization {
		if n.Parent != "" {
			h.community(n.Parent)
		}
		if n.Type == "Domain" && n.DomainType != "" {
			h.domainType(n.DomainType)
		}
	}
	for _, s := range h.sc.Scopes {
		for _, d := range s.Domains {
			h.domain(d)
		}
		for _, c := range s.Communities {
			h.community(c)
		}
	}
	for _, a := range h.sc.Assignments {
		h.assetType(a.AssetType)
		if a.Scope != "" {
			h.scope(a.Scope)
		}
		for _, dt := range a.DomainTypes {
			h.domainType(dt)
		}
		for _, r := range a.Relations {
			if r.Restriction != "" {
				h.assetType(r.Restriction)
			}
		}
	}
}

func (h *scenarioHarness) assetType(name string) scenarioAssetType {
	h.t.Helper()
	at, ok := h.assetTypesByName[name]
	if !ok {
		h.t.Fatalf("scenario has no asset type named %q", name)
	}
	return at
}

func (h *scenarioHarness) domainType(name string) scenarioDomainType {
	h.t.Helper()
	dt, ok := h.domainTypesByName[name]
	if !ok {
		h.t.Fatalf("scenario has no domain type named %q", name)
	}
	return dt
}

func (h *scenarioHarness) domain(name string) scenarioOrgNode {
	h.t.Helper()
	d, ok := h.domainsByName[name]
	if !ok {
		h.t.Fatalf("scenario has no domain named %q", name)
	}
	return d
}

func (h *scenarioHarness) community(name string) scenarioOrgNode {
	h.t.Helper()
	c, ok := h.communitiesByName[name]
	if !ok {
		h.t.Fatalf("scenario has no community named %q", name)
	}
	return c
}

func (h *scenarioHarness) scope(name string) scenarioScope {
	h.t.Helper()
	s, ok := h.scopesByName[name]
	if !ok {
		h.t.Fatalf("scenario has no scope named %q", name)
	}
	return s
}

func (h *scenarioHarness) startServer() {
	mux := http.NewServeMux()
	for _, at := range h.sc.AssetTypes {
		resp := PrepareCreateAssetType{ID: at.ID, Name: at.Name}
		if at.Parent != "" {
			p := h.assetType(at.Parent)
			resp.Parent = &PrepareCreateAssetType{ID: p.ID, Name: p.Name}
		}
		mux.Handle("GET /rest/2.0/assetTypes/"+at.ID, h.jsonResponse(resp))
		mux.Handle("GET /rest/2.0/assignments/assetType/"+at.ID, h.jsonResponse(h.rawAssignmentsFor(at.Name)))
	}
	for _, n := range h.sc.Organization {
		body := map[string]any{"id": n.ID, "name": n.Name}
		switch n.Type {
		case "Domain":
			if n.DomainType != "" {
				dt := h.domainType(n.DomainType)
				body["type"] = map[string]string{"id": dt.ID, "name": dt.Name}
			}
			if n.Parent != "" {
				body["community"] = map[string]string{"id": h.community(n.Parent).ID}
			}
			mux.Handle("GET /rest/2.0/domains/"+n.ID, h.jsonResponse(body))
		case "Community":
			if n.Parent != "" {
				body["parent"] = map[string]string{"id": h.community(n.Parent).ID}
			}
			mux.Handle("GET /rest/2.0/communities/"+n.ID, h.jsonResponse(body))
		}
	}
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		h.t.Errorf("scenario has no mock for %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	})

	srv := httptest.NewServer(mux)
	h.t.Cleanup(srv.Close)
	h.client = testutil.NewClient(srv)
}

func (h *scenarioHarness) jsonResponse(v any) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(v); err != nil {
			h.t.Errorf("encoding scenario response: %v", err)
		}
	})
}

func (h *scenarioHarness) rawAssignmentsFor(assetTypeName string) []rawScopedAssignment {
	var raws []rawScopedAssignment
	for _, a := range h.sc.Assignments {
		if a.AssetType != assetTypeName {
			continue
		}
		raws = append(raws, h.buildRawAssignment(a))
	}
	return raws
}

func (h *scenarioHarness) buildRawAssignment(a scenarioAssignment) rawScopedAssignment {
	raw := rawScopedAssignment{ID: a.ID}
	for _, dt := range a.DomainTypes {
		d := h.domainType(dt)
		raw.DomainTypes = append(raw.DomainTypes, rawAssignmentResourceRef{ID: d.ID, Name: d.Name})
	}
	if a.Scope != "" {
		s := h.scope(a.Scope)
		scope := &rawAssignmentScope{ID: s.ID, Name: s.Name}
		for _, dn := range s.Domains {
			d := h.domain(dn)
			scope.Domains = append(scope.Domains, rawAssignmentResourceRef{ID: d.ID, Name: d.Name})
		}
		for _, cn := range s.Communities {
			c := h.community(cn)
			scope.Communities = append(scope.Communities, rawAssignmentResourceRef{ID: c.ID, Name: c.Name})
		}
		raw.Scope = scope
	}
	raw.AssignedCharacteristicTypeReferences = h.buildCharacteristics(a.ID+"-own", a.Attributes, a.Relations)
	for i, tr := range a.Traits {
		refs := h.buildCharacteristics(fmt.Sprintf("%s-trait-%d", a.ID, i), tr.Attributes, tr.Relations)
		raw.TraitAssignmentInheritances = append(raw.TraitAssignmentInheritances,
			rawTraitAssignmentInheritance{AssignedCharacteristicTypeReferences: refs})
	}
	for i, tr := range a.AncestorTraits {
		refs := h.buildCharacteristics(fmt.Sprintf("%s-ancestor-%d", a.ID, i), tr.Attributes, tr.Relations)
		raw.AssignmentInheritances = append(raw.AssignmentInheritances, rawAssignmentInheritance{
			TraitAssignmentInheritances: []rawTraitAssignmentInheritance{
				{AssignedCharacteristicTypeReferences: refs},
			},
		})
	}
	return raw
}

func (h *scenarioHarness) buildCharacteristics(prefix string, attrs []scenarioAttribute, rels []scenarioRelation) []rawAssignedCharacteristicTypeReference {
	var refs []rawAssignedCharacteristicTypeReference
	line := 0
	nextLineID := func() string {
		line++
		return fmt.Sprintf("%s-line-%d", prefix, line)
	}
	for _, at := range attrs {
		kind := at.Kind
		if kind == "" {
			kind = "StringAttributeType"
		}
		refs = append(refs, rawAssignedCharacteristicTypeReference{
			ID: nextLineID(),
			AssignedResourceReference: rawAssignmentResourceRef{
				ID: at.ID, Name: at.Name, ResourceDiscriminator: kind,
			},
			AssignedResourcePublicID: at.PublicID,
			MinimumOccurrences:       at.Min,
			MaximumOccurrences:       at.Max,
		})
	}
	for _, r := range rels {
		direction := r.Direction
		if direction == "" {
			direction = "TO_TARGET"
		}
		disc := "RelationType"
		if r.Discriminator != nil {
			disc = *r.Discriminator
		}
		ref := rawAssignedCharacteristicTypeReference{
			ID: nextLineID(),
			AssignedResourceReference: rawAssignmentResourceRef{
				ID: r.ID, ResourceDiscriminator: disc,
			},
			AssignedResourcePublicID: r.PublicID,
			RelationTypeDirection:    direction,
		}
		if r.Restriction != "" {
			restriction := h.assetType(r.Restriction)
			ref.RelationTypeRestriction = &rawAssignmentResourceRef{ID: restriction.ID, Name: restriction.Name}
		}
		refs = append(refs, ref)
	}
	return refs
}
