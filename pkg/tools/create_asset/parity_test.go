package create_asset_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/collibra/chip/pkg/tools/create_asset"
	"github.com/collibra/chip/pkg/tools/prepare_create_asset"
	"github.com/collibra/chip/pkg/tools/testutil"
)

// allowedTypeSentinel is a distinctive allowed-domain-type name used to prove
// the scope-conditioned allowed-type list never leaks into the "not allowed"
// message. It is a substring of no other fixture field.
const allowedTypeSentinel = "AllowedTypeSentinel-9f3a"

// parityServer serves the minimum both create_asset and prepare_create_asset
// touch on the creatability-gate path. notHere selects the not-here scenario
// (domain type mismatch, assignment governs somewhere); otherwise the nowhere
// scenario (no assignment anywhere).
func parityServer(t *testing.T, notHere bool) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/2.0/assetTypes/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/rest/2.0/assetTypes/")
		if path == "publicId/"+btTypePublicID || path == btTypeID {
			writeJSON(w, http.StatusOK, assetTypeRow{ID: btTypeID, PublicID: btTypePublicID, Name: btTypeName})
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("GET /rest/2.0/assetTypes", func(w http.ResponseWriter, r *http.Request) {
		var rows []assetTypeRow
		if strings.EqualFold(r.URL.Query().Get("name"), btTypeName) {
			rows = []assetTypeRow{{ID: btTypeID, PublicID: btTypePublicID, Name: btTypeName}}
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": rows, "total": len(rows)})
	})
	domainType := map[string]string{"id": glossaryTypeID, "name": glossaryTypeName}
	if notHere {
		domainType = map[string]string{"id": "00000000-0000-0000-0000-000000010099", "name": "Other Domain Type"}
	}
	mux.HandleFunc("GET /rest/2.0/domains/", func(w http.ResponseWriter, r *http.Request) {
		if strings.TrimPrefix(r.URL.Path, "/rest/2.0/domains/") == glossaryDomainID {
			writeJSON(w, http.StatusOK, map[string]any{"id": glossaryDomainID, "name": glossaryDomain, "type": domainType})
			return
		}
		http.NotFound(w, r)
	})
	mux.HandleFunc("GET /rest/2.0/domains", func(w http.ResponseWriter, r *http.Request) {
		var rows []map[string]any
		if strings.EqualFold(r.URL.Query().Get("name"), glossaryDomain) {
			rows = []map[string]any{{"id": glossaryDomainID, "name": glossaryDomain, "type": domainType}}
		}
		writeJSON(w, http.StatusOK, map[string]any{"results": rows, "total": len(rows)})
	})
	mux.HandleFunc("GET /rest/2.0/assignments/assetType/", func(w http.ResponseWriter, r *http.Request) {
		if !notHere { // nowhere scenario: no assignment anywhere
			writeJSON(w, http.StatusOK, []any{})
			return
		}
		// The allowed domain type carries a distinctive name so the no-leak
		// assertion below can prove it never reaches the user-facing message.
		writeJSON(w, http.StatusOK, []map[string]any{{
			"id":          "asgn-bt",
			"domainTypes": []map[string]string{{"id": glossaryTypeID, "name": allowedTypeSentinel}},
		}})
	})
	mux.HandleFunc("GET /rest/2.0/statuses", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"results": []any{}, "total": 0})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// Both callers must emit the SAME two-branch "not allowed" message for the same
// (assetType, domain) situation — the parity the PRD requires.
func TestNotAllowedMessage_ParityAcrossCallers(t *testing.T) {
	cases := []struct {
		name    string
		notHere bool
		want    string // substring both messages must contain
	}{
		{"not-here", true, "isn't allowed in domain"},
		{"nowhere", false, "can't be created in any domain"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := testutil.NewClient(parityServer(t, tc.notHere))

			createOut, _ := create_asset.NewTool(c).Handler(t.Context(), create_asset.Input{
				Name: "X", AssetType: btTypeName, Domain: glossaryDomain,
			})
			prepareOut, _ := prepare_create_asset.NewTool(c).Handler(t.Context(), prepare_create_asset.Input{
				AssetType: btTypeName, Domain: glossaryDomain,
			})

			if createOut.Message != prepareOut.Message {
				t.Errorf("callers diverge:\n  create_asset:         %q\n  prepare_create_asset: %q", createOut.Message, prepareOut.Message)
			}
			if !strings.Contains(createOut.Message, tc.want) {
				t.Errorf("expected %q in message, got %q", tc.want, createOut.Message)
			}
			// Neither branch may leak the scope-conditioned allowed-type list.
			if strings.Contains(createOut.Message, allowedTypeSentinel) {
				t.Errorf("allowed domain type list must not leak into the message, got %q", createOut.Message)
			}
		})
	}
}
