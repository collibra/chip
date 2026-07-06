package upload_file_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/collibra/chip/pkg/tools/testutil"
	tools "github.com/collibra/chip/pkg/tools/upload_file"
)

func TestUploadFile_FromBase64Content(t *testing.T) {
	handler := http.NewServeMux()
	handler.HandleFunc("POST /edge/api/rest/v2/upload", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("cert://test-uuid/client-cert.pem"))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Filename:      "client-cert.pem",
		ContentBase64: base64.StdEncoding.EncodeToString([]byte("fake cert bytes")),
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
	if output.URI != "cert://test-uuid/client-cert.pem" {
		t.Fatalf("unexpected uri: %s", output.URI)
	}
}

func TestUploadFile_FromURL(t *testing.T) {
	edgeHandler := http.NewServeMux()
	edgeHandler.HandleFunc("POST /edge/api/rest/v2/upload", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("jar://test-uuid/driver.jar"))
	})
	edgeServer := httptest.NewServer(edgeHandler)
	defer edgeServer.Close()

	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("fake jar bytes"))
	}))
	defer sourceServer.Close()

	client := testutil.NewClient(edgeServer)
	output, err := tools.NewTool(client).Handler(t.Context(), tools.Input{
		Filename: "driver.jar",
		URL:      sourceServer.URL,
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
}

func TestUploadFile_RequiresExactlyOneSource(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))

	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Filename: "x"})
	if err == nil {
		t.Fatalf("expected an error when neither url nor contentBase64 is provided")
	}

	_, err = tools.NewTool(client).Handler(t.Context(), tools.Input{
		Filename:      "x",
		URL:           "http://example.com/x",
		ContentBase64: "Zm9v",
	})
	if err == nil {
		t.Fatalf("expected an error when both url and contentBase64 are provided")
	}
}
