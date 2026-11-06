package clients

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	operatingModelAssetTypeID = "00000000-0000-0000-0000-000000000011"
	operatingModelDomainID    = "00000000-0000-0000-0000-000000000022"
	operatingModelPath        = "/rest/2.0/assignments/assetType/" + operatingModelAssetTypeID + "/domain/" + operatingModelDomainID
)

func TestGetAssignmentForAssetTypeAndDomain_ShouldMapAttributesAndRelations(t *testing.T) {
	var gotPath string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id": "assignment-1",
			"assignedCharacteristicTypeReferences": [
				{"assignedResourceReference": {"id": "attr-1", "name": "Definition", "resourceDiscriminator": "StringAttributeType"},
				 "assignedResourcePublicId": "Definition", "minimumOccurrences": 1, "maximumOccurrences": 1},
				{"assignedResourceReference": {"id": "attr-2", "name": "Score", "resourceDiscriminator": "NumericAttributeType"},
				 "minimumOccurrences": 0, "readOnly": true},
				{"assignedResourceReference": {"id": "rel-1", "resourceDiscriminator": "RelationType"},
				 "assignedResourcePublicId": "TermIsPartOf", "relationTypeDirection": "TO_TARGET",
				 "relationTypeRestriction": {"id": "type-9", "name": "Business Term"}}
			],
			"traitAssignmentInheritances": [
				{"assignedCharacteristicTypeReferences": [
					{"assignedResourceReference": {"id": "attr-3", "name": "Note", "resourceDiscriminator": "StringAttributeType"}}
				]}
			]
		}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	got, err := GetAssignmentForAssetTypeAndDomain(t.Context(), testutil.NewClient(server), operatingModelAssetTypeID, operatingModelDomainID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != operatingModelPath {
		t.Errorf("expected path %q, got %q", operatingModelPath, gotPath)
	}
	if got.AssignmentID != "assignment-1" {
		t.Errorf("expected assignment id assignment-1, got %q", got.AssignmentID)
	}
	definition, ok := attrByName(got.Attributes, "Definition")
	if !ok || !definition.Required || definition.Kind != "StringAttributeType" || definition.AttributeTypePublicID != "Definition" {
		t.Errorf("unexpected Definition attribute: %+v", definition)
	}
	if definition.Max == nil || *definition.Max != 1 {
		t.Errorf("expected Definition max 1, got %v", definition.Max)
	}
	score, ok := attrByName(got.Attributes, "Score")
	if !ok || !score.ReadOnly || score.Required {
		t.Errorf("expected Score to be optional and read-only, got %+v", score)
	}
	if _, ok := attrByName(got.Attributes, "Note"); !ok {
		t.Errorf("expected trait-inherited attribute Note, got %+v", got.Attributes)
	}
	rels := relsByID(got.Relations, "rel-1")
	if len(rels) != 1 || rels[0].Direction != "TO_TARGET" || rels[0].TargetType == nil || rels[0].TargetType.ID != "type-9" {
		t.Errorf("unexpected relations: %+v", got.Relations)
	}
}

func TestGetAssignmentForAssetTypeAndDomain_ShouldReturnErrorWhenNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := GetAssignmentForAssetTypeAndDomain(t.Context(), testutil.NewClient(server), operatingModelAssetTypeID, operatingModelDomainID)
	if !errors.Is(err, ErrAssignmentNotFound) {
		t.Fatalf("expected ErrAssignmentNotFound, got %v", err)
	}
}

func TestGetAssignmentForAssetTypeAndDomain_ShouldReturnErrorOnServerFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer server.Close()

	_, err := GetAssignmentForAssetTypeAndDomain(t.Context(), testutil.NewClient(server), operatingModelAssetTypeID, operatingModelDomainID)
	if err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("expected status 500 error, got %v", err)
	}
}

func TestGetAssignmentForAssetTypeAndDomain_ShouldReturnErrorOnMalformedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()

	_, err := GetAssignmentForAssetTypeAndDomain(t.Context(), testutil.NewClient(server), operatingModelAssetTypeID, operatingModelDomainID)
	if err == nil || !strings.Contains(err.Error(), "decoding") {
		t.Fatalf("expected decoding error, got %v", err)
	}
}

const defaultAssignmentPath = "/rest/2.0/assignments/assetType/" + operatingModelAssetTypeID

func serveDefaultAssignments(t *testing.T, body string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(defaultAssignmentPath, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func TestGetDefaultAssignmentForAssetType_ShouldPickTheUnscopedAssignment(t *testing.T) {
	server := serveDefaultAssignments(t, `[
		{"id": "scoped", "scope": {"id": "scope-1", "name": "Scope"},
		 "assignedCharacteristicTypeReferences": [
			{"assignedResourceReference": {"id": "attr-s", "name": "Scoped Only", "resourceDiscriminator": "StringAttributeType"}}]},
		{"id": "default",
		 "assignedCharacteristicTypeReferences": [
			{"assignedResourceReference": {"id": "attr-d", "name": "Definition", "resourceDiscriminator": "StringAttributeType"}, "minimumOccurrences": 1}]}
	]`)

	got, err := GetDefaultAssignmentForAssetType(t.Context(), testutil.NewClient(server), operatingModelAssetTypeID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.AssignmentID != "default" {
		t.Errorf("expected the unscoped assignment, got %q", got.AssignmentID)
	}
	if _, ok := attrByName(got.Attributes, "Definition"); !ok {
		t.Errorf("expected Definition, got %+v", got.Attributes)
	}
	if _, ok := attrByName(got.Attributes, "Scoped Only"); ok {
		t.Errorf("scoped attribute must not bleed in, got %+v", got.Attributes)
	}
}

func TestGetDefaultAssignmentForAssetType_ShouldFailWhenOnlyScopedAssignmentsExist(t *testing.T) {
	server := serveDefaultAssignments(t, `[{"id": "scoped", "scope": {"id": "scope-1", "name": "Scope"}}]`)

	_, err := GetDefaultAssignmentForAssetType(t.Context(), testutil.NewClient(server), operatingModelAssetTypeID)

	if !errors.Is(err, ErrNoDefaultAssignment) {
		t.Fatalf("expected ErrNoDefaultAssignment, got %v", err)
	}
}

func TestGetDefaultAssignmentForAssetType_ShouldFailWhenThereAreNoAssignments(t *testing.T) {
	server := serveDefaultAssignments(t, `[]`)

	_, err := GetDefaultAssignmentForAssetType(t.Context(), testutil.NewClient(server), operatingModelAssetTypeID)

	if !errors.Is(err, ErrAssignmentNotFound) {
		t.Fatalf("expected ErrAssignmentNotFound, got %v", err)
	}
}

func TestGetDefaultAssignmentForAssetType_ShouldReturnErrorOnServerFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := GetDefaultAssignmentForAssetType(t.Context(), testutil.NewClient(server), operatingModelAssetTypeID)

	if err == nil || errors.Is(err, ErrAssignmentNotFound) || errors.Is(err, ErrNoDefaultAssignment) {
		t.Fatalf("expected a plain failure, got %v", err)
	}
}
