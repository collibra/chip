package get_agent_rules_file_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/collibra/chip/pkg/tools/get_agent_rules_file"
	"github.com/collibra/chip/pkg/tools/testutil"
)

const (
	aiAgentID = "3f1c2b7a-8d4e-4a6b-9c1d-2e3f4a5b6c7d"
	bundleID  = "7f8a9b0c-1d2e-4f3a-8b4c-5d6e7f8a9b0c"
	signedURL = "https://storage.example.com/bundles/bundle.zip?X-Signature=example-secret-signature&X-Expires=300"
)

const bundle = `{"id":"` + bundleID + `","version":3,"agentVersionId":"5e6f7a8b-9c0d-4e1f-8a2b-3c4d5e6f7a8b","agentFingerprintId":"0b1c2d3e-4f5a-4b6c-8d7e-9f0a1b2c3d4e",` +
	`"sizeInBytes":1048576,"contentHash":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855","compiledAt":"2026-10-02T09:04:00Z"}`

type fakeGuardian struct {
	client       *http.Client
	bundleGets   atomic.Int32
	fileGets     atomic.Int32
	storageHits  atomic.Int32
	fileStatus   int
	fileLocation string
	fileBody     string
}

// newFakeGuardian answers the latest-bundle read with bundleStatus/bundleBody
// and the file read with a redirect to a storage host. Any request the
// storage host receives means the redirect was followed.
func newFakeGuardian(t *testing.T, bundleStatus int, bundleBody string) *fakeGuardian {
	t.Helper()
	f := &fakeGuardian{fileStatus: http.StatusFound, fileLocation: signedURL}
	storage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.storageHits.Add(1)
		_, _ = w.Write([]byte("PK"))
	}))
	t.Cleanup(storage.Close)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /rest/guardianEnforcement/v1/bundles/by/aiAgent/{id}", func(w http.ResponseWriter, r *http.Request) {
		f.bundleGets.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(bundleStatus)
		_, _ = w.Write([]byte(bundleBody))
	})
	mux.HandleFunc("GET /rest/guardianEnforcement/v1/bundles/{id}/file", func(w http.ResponseWriter, r *http.Request) {
		f.fileGets.Add(1)
		if r.PathValue("id") != bundleID {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"statusCode":404,"errorCode":"bundleNotFound"}`))
			return
		}
		if f.fileLocation != "" {
			w.Header().Set("Location", strings.Replace(f.fileLocation, "https://storage.example.com", storage.URL, 1))
		}
		w.WriteHeader(f.fileStatus)
		_, _ = w.Write([]byte(f.fileBody))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.client = testutil.NewClient(srv)
	return f
}

func run(t *testing.T, f *fakeGuardian, id string) get_agent_rules_file.Output {
	t.Helper()
	out, err := get_agent_rules_file.NewTool(f.client).Handler(t.Context(), get_agent_rules_file.Input{AIAgentID: id})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// captureLogs routes slog to a buffer for the rest of the test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buf
}

func TestGetAgentRulesFile_ReturnsTheLinkWithoutFollowingOrLoggingIt(t *testing.T) {
	logs := captureLogs(t)
	f := newFakeGuardian(t, http.StatusOK, bundle)
	out := run(t, f, aiAgentID)

	if out.Status != get_agent_rules_file.StatusSuccess {
		t.Fatalf("unexpected output: %+v", out)
	}
	if out.BundleID != bundleID || out.Version != 3 || out.AgentVersionID != "5e6f7a8b-9c0d-4e1f-8a2b-3c4d5e6f7a8b" || out.SizeInBytes != 1048576 ||
		out.ContentHash != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" || out.CompiledAt != "2026-10-02T09:04:00Z" {
		t.Errorf("metadata wrong: %+v", out)
	}
	if !strings.Contains(out.DownloadURL, "X-Signature=example-secret-signature") {
		t.Errorf("downloadUrl = %q, want the redirect's Location", out.DownloadURL)
	}
	if f.storageHits.Load() != 0 {
		t.Errorf("the redirect was followed: storage received %d request(s)", f.storageHits.Load())
	}
	if f.bundleGets.Load() != 1 || f.fileGets.Load() != 1 {
		t.Errorf("calls: %d bundle, %d file; want 1 each", f.bundleGets.Load(), f.fileGets.Load())
	}
	if strings.Contains(out.Message, "example-secret-signature") {
		t.Errorf("the message must not carry the link: %s", out.Message)
	}
	if strings.Contains(logs.String(), "example-secret-signature") || strings.Contains(logs.String(), "X-Expires") {
		t.Errorf("the link was logged:\n%s", logs.String())
	}
}

func TestGetAgentRulesFile_NoBundleAvailableIsAnAnswer(t *testing.T) {
	f := newFakeGuardian(t, http.StatusNotFound, `{"statusCode":404,"errorCode":"noBundleAvailable"}`)
	out := run(t, f, aiAgentID)
	if out.Status != get_agent_rules_file.StatusNotFound || out.ErrorCode != "noBundleAvailable" || !strings.Contains(out.Guidance, "get_agent_guardian_status") {
		t.Fatalf("unexpected output: %+v", out)
	}
	if f.fileGets.Load() != 0 {
		t.Errorf("asked for a file link although there is no rules file")
	}
}

func TestGetAgentRulesFile_DocumentedErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		code   string
		want   get_agent_rules_file.OutputStatus
		hint   string
	}{
		{http.StatusNotFound, "aiAgentNotFound", get_agent_rules_file.StatusNotFound, "list_scanned_agents"},
		{http.StatusForbidden, "accessDenied", get_agent_rules_file.StatusError, "Do not retry"},
		{http.StatusForbidden, "tenantEnvironmentDisabled", get_agent_rules_file.StatusError, "Do not retry"},
	} {
		f := newFakeGuardian(t, tc.status, `{"statusCode":0,"errorCode":"`+tc.code+`"}`)
		out := run(t, f, aiAgentID)
		if out.Status != tc.want || out.ErrorCode != tc.code || out.HTTPStatus != tc.status || !strings.Contains(out.Guidance, tc.hint) {
			t.Errorf("%s: output %+v", tc.code, out)
		}
	}
}

func TestGetAgentRulesFile_FileErrorsAreReported(t *testing.T) {
	f := newFakeGuardian(t, http.StatusOK, bundle)
	f.fileStatus = http.StatusForbidden
	f.fileLocation = ""
	f.fileBody = `{"statusCode":403,"errorCode":"accessDenied"}`
	out := run(t, f, aiAgentID)
	if out.Status != get_agent_rules_file.StatusError || out.ErrorCode != "accessDenied" || out.DownloadURL != "" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestGetAgentRulesFile_RedirectToASignInPageIsASignInProblem(t *testing.T) {
	for _, location := range []string{"/signin?redirect=%2Frest", "https://storage.example.com/login"} {
		f := newFakeGuardian(t, http.StatusOK, bundle)
		f.fileLocation = location
		out := run(t, f, aiAgentID)
		if out.Status != get_agent_rules_file.StatusError || out.HTTPStatus != http.StatusUnauthorized || out.DownloadURL != "" {
			t.Errorf("%s: unexpected output: %+v", location, out)
		}
	}
}

func TestGetAgentRulesFile_AnswerWithoutRedirectIsAnError(t *testing.T) {
	f := newFakeGuardian(t, http.StatusOK, bundle)
	f.fileStatus = http.StatusOK
	f.fileLocation = ""
	f.fileBody = "PK"
	out := run(t, f, aiAgentID)
	if out.Status != get_agent_rules_file.StatusError || out.DownloadURL != "" {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestGetAgentRulesFile_RejectsANonUUIDWithoutCalling(t *testing.T) {
	f := newFakeGuardian(t, http.StatusOK, bundle)
	out := run(t, f, "support_triage_agent")
	if out.Status != get_agent_rules_file.StatusValidationError || f.bundleGets.Load()+f.fileGets.Load() != 0 {
		t.Fatalf("status %q after %d call(s)", out.Status, f.bundleGets.Load()+f.fileGets.Load())
	}
}
