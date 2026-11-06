package get_asset_operating_model_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tools "github.com/collibra/chip/pkg/tools/get_asset_operating_model"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	assetTypeID = "00000000-0000-0000-0000-000000000011"
	domainID    = "00000000-0000-0000-0000-000000000022"
)

const (
	assetTypeJSON = `{"id":"` + assetTypeID + `","publicId":"BusinessAsset","name":"Business Asset"}`
	domainJSON    = `{"id":"` + domainID + `","name":"AI Use Cases","type":{"id":"dt-1","name":"Business Asset Domain"}}`
)

const assignmentJSON = `{
	"id": "assignment-1",
	"assignedCharacteristicTypeReferences": [
		{"assignedResourceReference": {"id": "attr-1", "name": "Definition", "resourceDiscriminator": "StringAttributeType"},
		 "assignedResourcePublicId": "Definition", "minimumOccurrences": 1, "maximumOccurrences": 1},
		{"assignedResourceReference": {"id": "rel-1", "resourceDiscriminator": "RelationType"},
		 "relationTypeDirection": "TO_SOURCE"},
		{"assignedResourceReference": {"id": "rel-2", "resourceDiscriminator": "ComplexRelationType"},
		 "assignedResourcePublicId": "ComplexOne"}
	]
}`

// serverConfig describes which collaborators the fake Collibra serves.
type serverConfig struct {
	assignmentStatus int
	withDetails      bool
	// defaultAssignments is the body served for the no-domain assignments
	// lookup; empty means the endpoint returns an empty list.
	defaultAssignments string
}

func newServer(t *testing.T, cfg serverConfig) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/2.0/assetTypes/"+assetTypeID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(assetTypeJSON))
	})
	mux.HandleFunc("/rest/2.0/assetTypes/publicId/BusinessAsset", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(assetTypeJSON))
	})
	mux.HandleFunc("/rest/2.0/assetTypes", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("name") {
		case "business asset", "Business Asset":
			_, _ = w.Write([]byte(`{"results":[` + assetTypeJSON + `,{"id":"other","name":"Business Asset Group"}],"total":2}`))
		case "Twin":
			_, _ = w.Write([]byte(`{"results":[{"id":"t1","name":"Twin"},{"id":"t2","name":"twin"}],"total":2}`))
		default:
			_, _ = w.Write([]byte(`{"results":[],"total":0}`))
		}
	})
	mux.HandleFunc("/rest/2.0/domains", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("name") {
		case "ai use cases", "AI Use Cases":
			_, _ = w.Write([]byte(`{"results":[` + domainJSON + `,{"id":"d2","name":"AI Use Cases Archive"}],"total":2}`))
		case "Twin":
			_, _ = w.Write([]byte(`{"results":[{"id":"d3","name":"Twin"},{"id":"d4","name":"twin"}],"total":2}`))
		default:
			_, _ = w.Write([]byte(`{"results":[],"total":0}`))
		}
	})
	mux.HandleFunc("/rest/2.0/domains/"+domainID, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(domainJSON))
	})
	mux.HandleFunc("/rest/2.0/assignments/assetType/"+assetTypeID+"/domain/"+domainID, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(cfg.assignmentStatus)
		if cfg.assignmentStatus == http.StatusOK {
			_, _ = w.Write([]byte(assignmentJSON))
		}
	})
	mux.HandleFunc("/rest/2.0/assignments/assetType/"+assetTypeID, func(w http.ResponseWriter, _ *http.Request) {
		if cfg.defaultAssignments == "" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(cfg.defaultAssignments))
	})
	if cfg.withDetails {
		mux.HandleFunc("/rest/2.0/attributeTypes/attr-1", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"id":"attr-1","stringType":"RICH_TEXT","description":"Shortest clear definition","allowedValues":["a","b"]}`))
		})
		mux.HandleFunc("/rest/2.0/relationTypes/rel-1", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"id":"rel-1","publicId":"TermIsPartOf","role":"is part of","coRole":"contains",
				"sourceType":{"id":"src-1","name":"Source"},"targetType":{"id":"tgt-1","name":"Target"}}`))
		})
		mux.HandleFunc("/rest/2.0/complexRelationTypes/rel-2", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"id":"rel-2","publicId":"ComplexOne","legTypes":[
				{"role":"leg a","coRole":"leg b","relationTypePublicId":"LegRel","minimumOccurrences":1,"assetType":{"id":"at-1","name":"Type A"}}]}`))
		})
		mux.HandleFunc("/rest/2.0/statuses", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"results":[{"id":"st-1","name":"Candidate"},{"id":"st-2","name":"Approved"}],"total":2}`))
		})
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func call(t *testing.T, server *httptest.Server, input tools.Input) tools.Output {
	t.Helper()
	output, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), input)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	return output
}

func TestGetAssetOperatingModel_ShouldReturnResolvedSchemaStatusesAndHydratedDetails(t *testing.T) {
	server := newServer(t, serverConfig{assignmentStatus: http.StatusOK, withDetails: true})

	output := call(t, server, tools.Input{AssetType: assetTypeID, Domain: domainID})

	if output.Status != tools.StatusReady {
		t.Fatalf("expected status ready, got %q: %s", output.Status, output.Message)
	}
	if output.Resolved == nil || output.Resolved.AssetTypeName != "Business Asset" || output.Resolved.AssetTypePublicID != "BusinessAsset" ||
		output.Resolved.DomainName != "AI Use Cases" || output.Resolved.DomainTypeName != "Business Asset Domain" {
		t.Fatalf("unexpected resolved: %+v", output.Resolved)
	}
	if len(output.AttributeSchema) != 1 {
		t.Fatalf("expected 1 attribute, got %+v", output.AttributeSchema)
	}
	definition := output.AttributeSchema[0]
	if definition.Name != "Definition" || !definition.Required || definition.StringType != "RICH_TEXT" ||
		definition.Description != "Shortest clear definition" || len(definition.AllowedValues) != 2 {
		t.Errorf("attribute not hydrated: %+v", definition)
	}
	if len(output.RelationTypes) != 2 {
		t.Fatalf("expected 2 relations, got %+v", output.RelationTypes)
	}
	simple := output.RelationTypes[0]
	if simple.Role != "is part of" || simple.CoRole != "contains" || simple.PublicID != "TermIsPartOf" {
		t.Errorf("simple relation not hydrated: %+v", simple)
	}
	if simple.TargetTypeID != "src-1" {
		t.Errorf("expected TO_SOURCE relation to point at the source type, got %q", simple.TargetTypeID)
	}
	complexRel := output.RelationTypes[1]
	if len(complexRel.Legs) != 1 || complexRel.Legs[0].Role != "leg a" || complexRel.Legs[0].AssetTypeName != "Type A" ||
		complexRel.Legs[0].RelationTypePublicID != "LegRel" {
		t.Errorf("complex relation legs not hydrated: %+v", complexRel)
	}
	if len(output.AvailableStatuses) != 2 || output.AvailableStatuses[0].Name != "Candidate" {
		t.Errorf("unexpected statuses: %+v", output.AvailableStatuses)
	}
}

func TestGetAssetOperatingModel_ShouldKeepSchemaWhenHydrationFails(t *testing.T) {
	server := newServer(t, serverConfig{assignmentStatus: http.StatusOK})

	output := call(t, server, tools.Input{AssetType: assetTypeID, Domain: domainID})

	if output.Status != tools.StatusReady {
		t.Fatalf("expected status ready, got %q: %s", output.Status, output.Message)
	}
	if len(output.AttributeSchema) != 1 || len(output.RelationTypes) != 2 {
		t.Fatalf("expected schema to survive failed hydration, got %+v", output)
	}
	if output.AttributeSchema[0].StringType != "" || output.RelationTypes[0].Role != "" {
		t.Errorf("expected no hydrated details, got %+v", output)
	}
	if !strings.Contains(output.Message, "could not be fully hydrated") {
		t.Errorf("expected message to mention failed hydration, got %q", output.Message)
	}
	if len(output.AvailableStatuses) != 0 {
		t.Errorf("expected no statuses, got %+v", output.AvailableStatuses)
	}
}

func TestGetAssetOperatingModel_ShouldNeedClarificationWhenAssetTypeNotAllowedInDomain(t *testing.T) {
	// The asset type is allowed in another domain type, just not in this domain.
	server := newServer(t, serverConfig{
		assignmentStatus:   http.StatusNotFound,
		defaultAssignments: `[{"id": "a", "domainTypes": [{"id": "other-dt", "name": "Other Domain Type"}]}]`,
	})

	output := call(t, server, tools.Input{AssetType: assetTypeID, Domain: domainID})

	if output.Status != tools.StatusNeedsClarification {
		t.Fatalf("expected needs_clarification, got %q", output.Status)
	}
	if !strings.Contains(output.Message, "isn't allowed in domain") || !strings.Contains(output.Message, "AI Use Cases") {
		t.Errorf("expected message to name the asset type and domain, got %q", output.Message)
	}
	if output.Resolved != nil || len(output.AttributeSchema) != 0 {
		t.Errorf("expected no schema, got %+v", output)
	}
}

func TestGetAssetOperatingModel_ShouldReportLookupFailureOnServerError(t *testing.T) {
	server := newServer(t, serverConfig{assignmentStatus: http.StatusInternalServerError})

	output := call(t, server, tools.Input{AssetType: assetTypeID, Domain: domainID})

	if output.Status != tools.StatusNeedsClarification || !strings.Contains(output.Message, "Failed to retrieve operating model") {
		t.Fatalf("expected a lookup failure message, got %q: %s", output.Status, output.Message)
	}
}

func TestGetAssetOperatingModel_ShouldResolveAssetTypeAndDomainByAnyIdentifier(t *testing.T) {
	server := newServer(t, serverConfig{assignmentStatus: http.StatusOK})

	tests := map[string]tools.Input{
		"uuids":                    {AssetType: assetTypeID, Domain: domainID},
		"publicId and domain name": {AssetType: "BusinessAsset", Domain: "AI Use Cases"},
		"names ignoring case":      {AssetType: "business asset", Domain: "ai use cases"},
		"surrounding whitespace":   {AssetType: "  BusinessAsset ", Domain: " " + domainID + " "},
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			output := call(t, server, input)
			if output.Status != tools.StatusReady {
				t.Fatalf("expected status ready, got %q: %s", output.Status, output.Message)
			}
			if output.Resolved == nil || output.Resolved.AssetTypeID != assetTypeID || output.Resolved.DomainID != domainID {
				t.Errorf("expected the asset type and domain to resolve to the known IDs, got %+v", output.Resolved)
			}
		})
	}
}

func TestGetAssetOperatingModel_ShouldNeedClarificationWhenAssetTypeOrDomainCannotBeResolved(t *testing.T) {
	server := newServer(t, serverConfig{assignmentStatus: http.StatusOK})
	otherID := "00000000-0000-0000-0000-000000000099"

	tests := map[string]struct {
		input   tools.Input
		message string
	}{
		"unknown asset type uuid": {tools.Input{AssetType: otherID, Domain: domainID}, "Asset type"},
		"unknown asset type name": {tools.Input{AssetType: "Nope", Domain: domainID}, "no asset type matches"},
		"ambiguous asset type":    {tools.Input{AssetType: "Twin", Domain: domainID}, "ambiguous"},
		"unknown domain uuid":     {tools.Input{AssetType: assetTypeID, Domain: otherID}, "Domain"},
		"unknown domain name":     {tools.Input{AssetType: assetTypeID, Domain: "Nope"}, "no domain matches"},
		"ambiguous domain":        {tools.Input{AssetType: assetTypeID, Domain: "Twin"}, "ambiguous"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			output := call(t, server, tt.input)
			if output.Status != tools.StatusNeedsClarification || !strings.Contains(output.Message, tt.message) {
				t.Fatalf("expected needs_clarification containing %q, got %q: %s", tt.message, output.Status, output.Message)
			}
			if output.Resolved != nil {
				t.Errorf("expected nothing resolved, got %+v", output.Resolved)
			}
		})
	}
}

func TestGetAssetOperatingModel_ShouldRejectMissingAssetType(t *testing.T) {
	server := newServer(t, serverConfig{assignmentStatus: http.StatusOK})

	tests := map[string]tools.Input{
		"missing asset type": {Domain: domainID},
		"blank asset type":   {AssetType: "  ", Domain: domainID},
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := tools.NewTool(testutil.NewClient(server)).Handler(t.Context(), input); err == nil {
				t.Fatal("expected a validation error")
			}
		})
	}
}

const defaultAssignmentsJSON = `[
	{"id": "scoped", "scope": {"id": "scope-1", "name": "Scope"},
	 "assignedCharacteristicTypeReferences": [
		{"assignedResourceReference": {"id": "attr-s", "name": "Scoped Only", "resourceDiscriminator": "StringAttributeType"}}]},
	{"id": "default",
	 "assignedCharacteristicTypeReferences": [
		{"assignedResourceReference": {"id": "attr-1", "name": "Definition", "resourceDiscriminator": "StringAttributeType"}, "minimumOccurrences": 1}]}
]`

func TestGetAssetOperatingModel_ShouldReturnDefaultModelWhenDomainIsOmitted(t *testing.T) {
	server := newServer(t, serverConfig{assignmentStatus: http.StatusNotFound, withDetails: true, defaultAssignments: defaultAssignmentsJSON})

	for name, domain := range map[string]string{"omitted": "", "blank": "   "} {
		t.Run(name, func(t *testing.T) {
			output := call(t, server, tools.Input{AssetType: "BusinessAsset", Domain: domain})

			if output.Status != tools.StatusReady {
				t.Fatalf("expected status ready, got %q: %s", output.Status, output.Message)
			}
			if output.Resolved == nil || output.Resolved.AssetTypeID != assetTypeID || output.Resolved.DomainID != "" || output.Resolved.DomainName != "" {
				t.Errorf("expected the asset type only, got %+v", output.Resolved)
			}
			if len(output.AttributeSchema) != 1 || output.AttributeSchema[0].Name != "Definition" || output.AttributeSchema[0].StringType != "RICH_TEXT" {
				t.Errorf("expected the default Definition attribute, hydrated, got %+v", output.AttributeSchema)
			}
			if !strings.Contains(output.Message, "without a domain") {
				t.Errorf("expected message to say no domain was given, got %q", output.Message)
			}
			if len(output.AvailableStatuses) != 2 {
				t.Errorf("expected statuses, got %+v", output.AvailableStatuses)
			}
		})
	}
}

func TestGetAssetOperatingModel_ShouldAskForDomainWhenAssetTypeHasOnlyScopedAssignments(t *testing.T) {
	server := newServer(t, serverConfig{defaultAssignments: `[{"id": "scoped", "scope": {"id": "scope-1", "name": "Scope"}}]`})

	output := call(t, server, tools.Input{AssetType: assetTypeID})

	if output.Status != tools.StatusNeedsClarification || !strings.Contains(output.Message, "a domain is required") {
		t.Fatalf("expected a request for a domain, got %q: %s", output.Status, output.Message)
	}
}

func TestGetAssetOperatingModel_ShouldReportNoAssignmentWhenDomainIsOmittedAndNoneExist(t *testing.T) {
	server := newServer(t, serverConfig{})

	output := call(t, server, tools.Input{AssetType: assetTypeID})

	if output.Status != tools.StatusNeedsClarification || !strings.Contains(output.Message, "no attributes or relations assigned") {
		t.Fatalf("expected a no-assignment message, got %q: %s", output.Status, output.Message)
	}
}

func TestGetAssetOperatingModel_ShouldSayAssetTypeIsCreatableNowhereWhenItHasNoAssignments(t *testing.T) {
	server := newServer(t, serverConfig{assignmentStatus: http.StatusNotFound})

	output := call(t, server, tools.Input{AssetType: assetTypeID, Domain: domainID})

	if output.Status != tools.StatusNeedsClarification || !strings.Contains(output.Message, "can't be created in any domain") {
		t.Fatalf("expected the creatable-nowhere message, got %q: %s", output.Status, output.Message)
	}
}
