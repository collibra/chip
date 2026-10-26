package publish_agent_scan_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/collibra/chip/pkg/tools/publish_agent_scan"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const scansPath = "/rest/guardianFingerprint/v1/scans"

// Key order and spacing are deliberate: the document must reach the API as the
// producer wrote it, so the test compares it with the bytes below.
const document = `{"fingerprintVersion":"1.0","contentHash":"f766","agents":[{"name":"support_triage_agent","role":"ROOT"}],"note":"a<b & c"}`

const domainID = "4d250cc5-e583-4640-9874-b93d82c7a6cb"

type fakeGuardian struct {
	client   *http.Client
	calls    atomic.Int32
	lastBody []byte
}

// newFakeGuardian answers every POST /scans with status and body.
func newFakeGuardian(t *testing.T, status int, body string) *fakeGuardian {
	t.Helper()
	f := &fakeGuardian{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+scansPath, func(w http.ResponseWriter, r *http.Request) {
		f.calls.Add(1)
		f.lastBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.client = testutil.NewClient(srv)
	return f
}

func validInput(confirm bool) publish_agent_scan.Input {
	return publish_agent_scan.Input{
		Document:   json.RawMessage(document),
		ExternalID: "catalog.schema.support_triage_agent",
		DomainID:   domainID,
		Confirm:    confirm,
	}
}

func TestPublishAgentScan_PreviewEchoesEveryFieldAndSendsNothing(t *testing.T) {
	f := newFakeGuardian(t, http.StatusCreated, `{}`)

	out, err := publish_agent_scan.NewTool(f.client).Handler(t.Context(), validInput(false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != publish_agent_scan.StatusPreview {
		t.Fatalf("status = %q, want preview (%s)", out.Status, out.Message)
	}
	if f.calls.Load() != 0 {
		t.Fatalf("preview sent %d request(s), want 0", f.calls.Load())
	}
	if out.Preview == nil {
		t.Fatal("preview is nil")
	}
	if string(out.Preview.Document) != document {
		t.Errorf("preview document = %s, want %s", out.Preview.Document, document)
	}
	if out.Preview.DocumentBytes != len(document) {
		t.Errorf("preview documentBytes = %d, want %d", out.Preview.DocumentBytes, len(document))
	}
	if out.Preview.ExternalID != "catalog.schema.support_triage_agent" {
		t.Errorf("preview externalId = %q", out.Preview.ExternalID)
	}
	if out.Preview.DomainID != domainID {
		t.Errorf("preview domainId = %q, want %q", out.Preview.DomainID, domainID)
	}
}

func TestPublishAgentScan_PreviewIsTheDefault(t *testing.T) {
	f := newFakeGuardian(t, http.StatusCreated, `{}`)
	in := validInput(false)
	in.DomainID = ""

	out, _ := publish_agent_scan.NewTool(f.client).Handler(t.Context(), in)
	if out.Status != publish_agent_scan.StatusPreview || f.calls.Load() != 0 {
		t.Fatalf("status = %q, calls = %d; want preview and no calls", out.Status, f.calls.Load())
	}
	if out.Preview.DomainID != "" {
		t.Errorf("preview domainId = %q, want empty when none was given", out.Preview.DomainID)
	}
}

func TestPublishAgentScan_SuccessSendsDocumentUnchangedAndReturnsScan(t *testing.T) {
	f := newFakeGuardian(t, http.StatusCreated, `{"id":"scan-1","fingerprintId":"fp-1","createdOn":"2026-02-11T09:31:05Z","generatedOn":"2026-02-11T09:31:04Z"}`)

	out, err := publish_agent_scan.NewTool(f.client).Handler(t.Context(), validInput(true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != publish_agent_scan.StatusSuccess {
		t.Fatalf("status = %q, want success (%s)", out.Status, out.Message)
	}
	if out.ScanID != "scan-1" || out.FingerprintID != "fp-1" ||
		out.CreatedOn != "2026-02-11T09:31:05Z" || out.GeneratedOn != "2026-02-11T09:31:04Z" {
		t.Errorf("unexpected scan fields: %+v", out)
	}
	if f.calls.Load() != 1 {
		t.Fatalf("sent %d request(s), want 1", f.calls.Load())
	}

	// The envelope is what the spec's CreateScanRequest names, and the document
	// inside it is byte-for-byte what was supplied (no key reordering, no
	// HTML escaping).
	want := `{"document":` + document + `,"externalId":"catalog.schema.support_triage_agent","metadata":{"domainId":"` + domainID + `"}}`
	if string(f.lastBody) != want {
		t.Errorf("request body =\n%s\nwant\n%s", f.lastBody, want)
	}
}

func TestPublishAgentScan_SuccessWithoutDomainOmitsMetadata(t *testing.T) {
	f := newFakeGuardian(t, http.StatusCreated, `{"id":"scan-1","fingerprintId":"fp-1","createdOn":"c","generatedOn":"g"}`)
	in := validInput(true)
	in.DomainID = ""

	out, _ := publish_agent_scan.NewTool(f.client).Handler(t.Context(), in)
	if out.Status != publish_agent_scan.StatusSuccess {
		t.Fatalf("status = %q (%s)", out.Status, out.Message)
	}
	var sent map[string]json.RawMessage
	if err := json.Unmarshal(f.lastBody, &sent); err != nil {
		t.Fatalf("request body is not JSON: %v", err)
	}
	if _, ok := sent["metadata"]; ok {
		t.Errorf("metadata should be omitted when no domainId is given, body = %s", f.lastBody)
	}
}

// The create-scan response carries no import state, so the tool cannot report
// a stored-but-not-imported outcome and must not invent one. A success says
// only what the API says: Guardian has the evidence, and the import is not
// confirmed.
func TestPublishAgentScan_SuccessDoesNotClaimImportState(t *testing.T) {
	f := newFakeGuardian(t, http.StatusCreated, `{"id":"scan-1","fingerprintId":"fp-1","createdOn":"c","generatedOn":"g"}`)

	out, _ := publish_agent_scan.NewTool(f.client).Handler(t.Context(), validInput(true))
	if !strings.Contains(out.Message, "does not mean the catalog update") {
		t.Errorf("success message should say the import is not confirmed, got: %s", out.Message)
	}
	for _, invented := range []string{"imported successfully", "import failed", "update failed", "did not complete", "pending"} {
		if strings.Contains(strings.ToLower(out.Message), invented) {
			t.Errorf("success message invents import state %q: %s", invented, out.Message)
		}
	}
}

func TestPublishAgentScan_RejectionsReturnErrorCodeAndPlainMessage(t *testing.T) {
	codes := map[int][]string{
		http.StatusBadRequest: {
			"requestInvalid", "fingerprintVersionUnsupported", "fingerprintRootAgentInvalid",
			"fingerprintDuplicateName", "fingerprintNameTooShort", "fingerprintUnresolvedReference",
			"fingerprintDefaultModelInvalid", "fingerprintGuardrailConfidenceMissing",
			"fingerprintLlmFallbackWithoutLlm", "fingerprintRootOnlyField",
		},
		http.StatusUnauthorized: {"bearerTokenRequired", "tenantEnvironmentRequired"},
		http.StatusForbidden:    {"tenantEnvironmentForbidden", "tenantEnvironmentDisabled"},
	}
	for status, errorCodes := range codes {
		for _, code := range errorCodes {
			t.Run(code, func(t *testing.T) {
				f := newFakeGuardian(t, status, `{"errorCode":"`+code+`","statusCode":`+itoa(status)+`,"userMessage":"detail from guardian"}`)

				out, err := publish_agent_scan.NewTool(f.client).Handler(t.Context(), validInput(true))
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if out.Status != publish_agent_scan.StatusError {
					t.Fatalf("status = %q, want error", out.Status)
				}
				if out.ErrorCode != code || out.HTTPStatus != status {
					t.Errorf("errorCode/httpStatus = %q/%d, want %q/%d", out.ErrorCode, out.HTTPStatus, code, status)
				}
				if strings.Contains(out.Message, code) {
					t.Errorf("message should be plain language, not the raw code: %s", out.Message)
				}
				if !strings.Contains(out.Message, "detail from guardian") {
					t.Errorf("message should carry the API's userMessage: %s", out.Message)
				}
				if out.Guidance == "" {
					t.Error("guidance is empty")
				}
				if f.calls.Load() != 1 {
					t.Errorf("sent %d request(s), want exactly 1", f.calls.Load())
				}
			})
		}
	}
}

func TestPublishAgentScan_AuthErrorsTellTheSkillNotToRetry(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		f := newFakeGuardian(t, status, `{"errorCode":"x","statusCode":`+itoa(status)+`}`)
		out, _ := publish_agent_scan.NewTool(f.client).Handler(t.Context(), validInput(true))
		if !strings.Contains(out.Guidance, "Do not retry") {
			t.Errorf("HTTP %d guidance should say not to retry: %s", status, out.Guidance)
		}
	}
}

func TestPublishAgentScan_NoRetryOn403(t *testing.T) {
	f := newFakeGuardian(t, http.StatusForbidden, `{"errorCode":"tenantEnvironmentForbidden","statusCode":403}`)

	_, _ = publish_agent_scan.NewTool(f.client).Handler(t.Context(), validInput(true))
	if got := f.calls.Load(); got != 1 {
		t.Fatalf("403 produced %d request(s), want 1 (no retry)", got)
	}
}

func TestPublishAgentScan_NoRetryOn401(t *testing.T) {
	f := newFakeGuardian(t, http.StatusUnauthorized, `{"errorCode":"bearerTokenRequired","statusCode":401}`)

	_, _ = publish_agent_scan.NewTool(f.client).Handler(t.Context(), validInput(true))
	if got := f.calls.Load(); got != 1 {
		t.Fatalf("401 produced %d request(s), want 1 (no retry)", got)
	}
}

func TestPublishAgentScan_UnstructuredErrorBodyStillReportsStatus(t *testing.T) {
	f := newFakeGuardian(t, http.StatusBadGateway, `<html>bad gateway</html>`)

	out, _ := publish_agent_scan.NewTool(f.client).Handler(t.Context(), validInput(true))
	if out.Status != publish_agent_scan.StatusError || out.HTTPStatus != http.StatusBadGateway || out.ErrorCode != "" {
		t.Fatalf("got status=%q httpStatus=%d errorCode=%q", out.Status, out.HTTPStatus, out.ErrorCode)
	}
	if strings.Contains(out.Message, "bad gateway") {
		t.Errorf("raw response body should not be echoed: %s", out.Message)
	}
	if f.calls.Load() != 1 {
		t.Errorf("sent %d request(s), want 1", f.calls.Load())
	}
}

func TestPublishAgentScan_TransportFailureIsReportedWithoutStatus(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	client := testutil.NewClient(srv)
	srv.Close() // connection refused

	out, err := publish_agent_scan.NewTool(client).Handler(t.Context(), validInput(true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != publish_agent_scan.StatusError || out.HTTPStatus != 0 || out.ErrorCode != "" {
		t.Fatalf("got status=%q httpStatus=%d errorCode=%q", out.Status, out.HTTPStatus, out.ErrorCode)
	}
}

func TestPublishAgentScan_ValidationErrorsSendNothing(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*publish_agent_scan.Input)
		mention string
	}{
		{"missing document", func(in *publish_agent_scan.Input) { in.Document = nil }, "document is required"},
		{"null document", func(in *publish_agent_scan.Input) { in.Document = json.RawMessage(`null`) }, "document is required"},
		{"string document", func(in *publish_agent_scan.Input) { in.Document = json.RawMessage(`"{}"`) }, "must be a JSON object"},
		{"array document", func(in *publish_agent_scan.Input) { in.Document = json.RawMessage(`[]`) }, "must be a JSON object"},
		{"missing externalId", func(in *publish_agent_scan.Input) { in.ExternalID = "  " }, "externalId is required"},
		{"bad domainId", func(in *publish_agent_scan.Input) { in.DomainID = "not-a-uuid" }, "not a valid UUID"},
	}
	for _, tt := range tests {
		for _, confirm := range []bool{false, true} {
			t.Run(tt.name, func(t *testing.T) {
				f := newFakeGuardian(t, http.StatusCreated, `{}`)
				in := validInput(confirm)
				tt.mutate(&in)

				out, err := publish_agent_scan.NewTool(f.client).Handler(t.Context(), in)
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if out.Status != publish_agent_scan.StatusValidationError {
					t.Fatalf("status = %q, want validation_error", out.Status)
				}
				if !strings.Contains(out.Message, tt.mention) {
					t.Errorf("message %q should mention %q", out.Message, tt.mention)
				}
				if f.calls.Load() != 0 {
					t.Errorf("validation error sent %d request(s)", f.calls.Load())
				}
			})
		}
	}
}

func TestPublishAgentScan_Metadata(t *testing.T) {
	tool := publish_agent_scan.NewTool(&http.Client{})
	if tool.Name != "publish_agent_scan" {
		t.Errorf("name = %q", tool.Name)
	}
	a := tool.Annotations
	if a == nil || a.DestructiveHint == nil || a.OpenWorldHint == nil || a.ReadOnlyHint {
		t.Errorf("annotations must set DestructiveHint and OpenWorldHint on a writing tool: %+v", a)
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
