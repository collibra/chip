package publish_agent_scan_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/collibra/chip/pkg/tools/publish_agent_scan"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const databricksID = "databricks:1234567890123456:example_catalog.agents.support_triage_agent"

func databricksDocument(workspaceID, model string) json.RawMessage {
	return json.RawMessage(`{"fingerprintVersion":"1.0","platform":{"name":"DATABRICKS","metadata":{"workspaceId":` + workspaceID + `,"ucModelName":"` + model + `"}}}`)
}

// Each rule is checked with confirm=false and confirm=true: the preview must
// fail early too, and neither state may reach the API.
func TestPublishAgentScan_ExternalIDRules(t *testing.T) {
	tests := []struct {
		name       string
		externalID string
		mention    string
	}{
		{"blank", "   ", "must not be blank"},
		{"empty", "", "must not be blank"},
		{"leading whitespace", " " + databricksID, "leading or trailing whitespace"},
		{"trailing whitespace", databricksID + "\n", "leading or trailing whitespace"},
		{"greater-than", "my>agent", "contains '>'"},
		{"greater-than in a databricks id", "databricks:1:cat.schema.mo>del", "contains '>'"},
		{"databricks non-numeric workspace", "databricks:abc:cat.schema.model", "databricks:<workspace id"},
		{"databricks two-part name", "databricks:123:schema.model", "databricks:<workspace id"},
		{"databricks four-part name", "databricks:123:a.b.c.d", "databricks:<workspace id"},
		{"databricks empty segment", "databricks:123:cat..model", "databricks:<workspace id"},
		{"databricks missing workspace", "databricks::cat.schema.model", "databricks:<workspace id"},
	}
	for _, tt := range tests {
		for _, confirm := range []bool{false, true} {
			t.Run(tt.name, func(t *testing.T) {
				f := newFakeGuardian(t, http.StatusCreated, `{}`)
				in := validInput(confirm)
				in.ExternalID = tt.externalID

				out, err := publish_agent_scan.NewTool(f.client).Handler(t.Context(), in)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if out.Status != publish_agent_scan.StatusValidationError {
					t.Fatalf("confirm=%v: status = %q, want validation_error (%s)", confirm, out.Status, out.Message)
				}
				if !strings.Contains(out.Message, tt.mention) {
					t.Errorf("message %q should mention %q", out.Message, tt.mention)
				}
				if f.calls.Load() != 0 {
					t.Errorf("confirm=%v: sent %d request(s), want none", confirm, f.calls.Load())
				}
			})
		}
	}
}

func TestPublishAgentScan_ValidExternalIDsAreAccepted(t *testing.T) {
	for _, id := range []string{
		databricksID,
		"https://github.com/acme/agents#support_triage_agent",
		"snowflake:acct1:db.schema.agent",
		"a b c",
		"databricks", // no ':' after the prefix, so not a databricks-shaped claim
	} {
		f := newFakeGuardian(t, http.StatusCreated, `{"id":"s","fingerprintId":"f","createdOn":"c","generatedOn":"g"}`)
		in := validInput(false)
		in.ExternalID = id
		out, _ := publish_agent_scan.NewTool(f.client).Handler(t.Context(), in)
		if out.Status != publish_agent_scan.StatusPreview {
			t.Errorf("externalId %q: status = %q (%s), want preview", id, out.Status, out.Message)
		}
	}
}

// The structure after "otel:" is undecided (see the TODO in
// telemetry_routes.go), so no unknown prefix may be validated beyond the
// universal rules, and none may be compared with the document.
func TestPublishAgentScan_UnknownTelemetryRoutePrefixesPassThrough(t *testing.T) {
	for _, id := range []string{"otel:cluster-1:ns/agent", "otel:x", "otel::", "otel:anything:at.all:really", "newroute:1:2:3"} {
		for _, confirm := range []bool{false, true} {
			f := newFakeGuardian(t, http.StatusCreated, `{"id":"s","fingerprintId":"f","createdOn":"c","generatedOn":"g"}`)
			in := validInput(confirm)
			in.ExternalID = id
			in.Document = databricksDocument("1", "a.b.c") // must not be compared with an otel: id

			out, _ := publish_agent_scan.NewTool(f.client).Handler(t.Context(), in)
			if out.Status == publish_agent_scan.StatusValidationError || len(out.Warnings) != 0 {
				t.Errorf("externalId %q confirm=%v: status=%q warnings=%v (%s)", id, confirm, out.Status, out.Warnings, out.Message)
			}
		}
	}
}

func TestPublishAgentScan_UniversalRulesStillApplyToUnknownPrefixes(t *testing.T) {
	for _, id := range []string{"otel:a>b", " otel:a", "otel:a ", "  "} {
		for _, confirm := range []bool{false, true} {
			f := newFakeGuardian(t, http.StatusCreated, `{}`)
			in := validInput(confirm)
			in.ExternalID = id
			out, _ := publish_agent_scan.NewTool(f.client).Handler(t.Context(), in)
			if out.Status != publish_agent_scan.StatusValidationError || f.calls.Load() != 0 {
				t.Errorf("externalId %q confirm=%v: status=%q calls=%d", id, confirm, out.Status, f.calls.Load())
			}
		}
	}
}

func TestPublishAgentScan_ExternalIDIsSentWithoutModification(t *testing.T) {
	f := newFakeGuardian(t, http.StatusCreated, `{"id":"s","fingerprintId":"f","createdOn":"c","generatedOn":"g"}`)
	in := validInput(true)
	in.ExternalID = databricksID
	_, _ = publish_agent_scan.NewTool(f.client).Handler(t.Context(), in)
	if !strings.Contains(string(f.lastBody), `"externalId":"`+databricksID+`"`) {
		t.Errorf("request body = %s", f.lastBody)
	}
}

func TestPublishAgentScan_PlatformMismatchWarnsButDoesNotBlock(t *testing.T) {
	const created = `{"id":"s","fingerprintId":"f","createdOn":"c","generatedOn":"g"}`
	for _, confirm := range []bool{false, true} {
		f := newFakeGuardian(t, http.StatusCreated, created)
		in := validInput(confirm)
		in.ExternalID = "databricks:1111:cat.schema.other_model"
		in.Document = databricksDocument("1234567890123456", "example_catalog.agents.support_triage_agent")

		out, _ := publish_agent_scan.NewTool(f.client).Handler(t.Context(), in)
		want := publish_agent_scan.StatusPreview
		wantCalls := int32(0)
		if confirm {
			want, wantCalls = publish_agent_scan.StatusSuccess, 1
		}
		if out.Status != want || f.calls.Load() != wantCalls {
			t.Fatalf("confirm=%v: status=%q calls=%d, want %q and %d call(s)", confirm, out.Status, f.calls.Load(), want, wantCalls)
		}
		if len(out.Warnings) != 1 || !strings.Contains(out.Warnings[0], databricksID) {
			t.Errorf("confirm=%v: warnings = %v, want one naming the expected id %q", confirm, out.Warnings, databricksID)
		}
		if !strings.Contains(out.Message, "Warning:") {
			t.Errorf("confirm=%v: message should repeat the warning: %s", confirm, out.Message)
		}
	}
}

func TestPublishAgentScan_PlatformMatchHasNoWarning(t *testing.T) {
	for _, confirm := range []bool{false, true} {
		f := newFakeGuardian(t, http.StatusCreated, `{"id":"s","fingerprintId":"f","createdOn":"c","generatedOn":"g"}`)
		in := validInput(confirm)
		in.ExternalID = databricksID
		in.Document = databricksDocument("1234567890123456", "example_catalog.agents.support_triage_agent")
		out, _ := publish_agent_scan.NewTool(f.client).Handler(t.Context(), in)
		if len(out.Warnings) != 0 {
			t.Errorf("confirm=%v: unexpected warnings %v", confirm, out.Warnings)
		}
	}
}

func TestPublishAgentScan_WorkspaceIDAsStringAlsoMatches(t *testing.T) {
	f := newFakeGuardian(t, http.StatusCreated, `{}`)
	in := validInput(false)
	in.ExternalID = databricksID
	in.Document = databricksDocument(`"1234567890123456"`, "example_catalog.agents.support_triage_agent")
	out, _ := publish_agent_scan.NewTool(f.client).Handler(t.Context(), in)
	if len(out.Warnings) != 0 {
		t.Errorf("unexpected warnings %v", out.Warnings)
	}
}

func TestPublishAgentScan_NoWarningWhenPlatformInfoIsAbsent(t *testing.T) {
	documents := map[string]string{
		"no platform":            `{"fingerprintVersion":"1.0"}`,
		"other platform":         `{"platform":{"name":"SNOWFLAKE","metadata":{"workspaceId":1,"ucModelName":"a.b.c"}}}`,
		"no metadata":            `{"platform":{"name":"DATABRICKS"}}`,
		"missing ucModelName":    `{"platform":{"name":"DATABRICKS","metadata":{"workspaceId":1}}}`,
		"missing workspaceId":    `{"platform":{"name":"DATABRICKS","metadata":{"ucModelName":"a.b.c"}}}`,
		"unusable metadata type": `{"platform":{"name":"DATABRICKS","metadata":{"workspaceId":{"x":1},"ucModelName":["a"]}}}`,
	}
	for name, doc := range documents {
		for _, confirm := range []bool{false, true} {
			t.Run(name, func(t *testing.T) {
				f := newFakeGuardian(t, http.StatusCreated, `{"id":"s","fingerprintId":"f","createdOn":"c","generatedOn":"g"}`)
				in := validInput(confirm)
				in.ExternalID = "https://github.com/acme/agents#root"
				in.Document = json.RawMessage(doc)
				out, _ := publish_agent_scan.NewTool(f.client).Handler(t.Context(), in)
				if len(out.Warnings) != 0 {
					t.Errorf("confirm=%v: unexpected warnings %v", confirm, out.Warnings)
				}
				if out.Status == publish_agent_scan.StatusValidationError {
					t.Errorf("confirm=%v: should pass through, got %s", confirm, out.Message)
				}
			})
		}
	}
}

func TestPublishAgentScan_MismatchWarningAlsoAccompaniesRejection(t *testing.T) {
	f := newFakeGuardian(t, http.StatusBadRequest, `{"errorCode":"requestInvalid","statusCode":400}`)
	in := validInput(true)
	in.ExternalID = "databricks:1:a.b.c"
	in.Document = databricksDocument("2", "x.y.z")
	out, _ := publish_agent_scan.NewTool(f.client).Handler(t.Context(), in)
	if out.Status != publish_agent_scan.StatusError || len(out.Warnings) != 1 {
		t.Errorf("status=%q warnings=%v", out.Status, out.Warnings)
	}
}

// A base URL that points at a sign-in page, or an expired session, ends in a
// redirect to /signin. That is the not-signed-in case, not a network error.
func TestPublishAgentScan_RedirectToSignInIsReportedAsNotSignedIn(t *testing.T) {
	var scanCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/rest/guardianFingerprint/v1/scans", func(w http.ResponseWriter, r *http.Request) {
		scanCalls.Add(1)
		http.Redirect(w, r, "/signin", http.StatusFound)
	})
	mux.HandleFunc("/signin", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>Sign in</body></html>"))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	out, _ := publish_agent_scan.NewTool(testutil.NewClient(srv)).Handler(t.Context(), validInput(true))
	assertNotSignedIn(t, out)
	if scanCalls.Load() != 1 {
		t.Errorf("sent %d request(s), want 1", scanCalls.Load())
	}
}

func TestPublishAgentScan_UnfollowedRedirectIsReportedAsNotSignedIn(t *testing.T) {
	f := newFakeGuardian(t, http.StatusFound, `<a href="/signin">Found</a>`)
	out, _ := publish_agent_scan.NewTool(noRedirects(f.client)).Handler(t.Context(), validInput(true))
	assertNotSignedIn(t, out)
}

func assertNotSignedIn(t *testing.T, out publish_agent_scan.Output) {
	t.Helper()
	if out.Status != publish_agent_scan.StatusError || out.HTTPStatus != http.StatusUnauthorized || out.ErrorCode != "signInRequired" {
		t.Fatalf("got status=%q httpStatus=%d errorCode=%q (%s)", out.Status, out.HTTPStatus, out.ErrorCode, out.Message)
	}
	if !strings.Contains(out.Guidance, "Do not retry") {
		t.Errorf("guidance should say not to retry: %s", out.Guidance)
	}
	if strings.Contains(strings.ToLower(out.Guidance), "safe to offer to try again") {
		t.Errorf("guidance must not call this retryable: %s", out.Guidance)
	}
}

func noRedirects(c *http.Client) *http.Client {
	clone := *c
	clone.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &clone
}
