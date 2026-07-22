package upload_file_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestUploadFile_RequiresContentBase64(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))

	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{Filename: "x"})
	if err == nil {
		t.Fatalf("expected an error when contentBase64 is not provided")
	}
}

func TestUploadFile_RequiresFilename(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))

	_, err := tools.NewTool(client).Handler(t.Context(), tools.Input{ContentBase64: "Zm9v"})
	if err == nil {
		t.Fatalf("expected an error when filename is not provided")
	}
}

func TestUploadFile_WithFilePath_FromDisk(t *testing.T) {
	tmpFile, err := os.CreateTemp(t.TempDir(), "driver-*.jar")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	if _, err := tmpFile.Write([]byte("fake jar bytes")); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}
	if err := tmpFile.Close(); err != nil {
		t.Fatalf("failed to close temp file: %v", err)
	}

	handler := http.NewServeMux()
	handler.HandleFunc("POST /edge/api/rest/v2/upload", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("jar://test-uuid/driver.jar"))
	})
	server := httptest.NewServer(handler)
	defer server.Close()

	client := testutil.NewClient(server)
	output, err := tools.NewToolWithFilePath(client).Handler(t.Context(), tools.InputWithFilePath{
		Filename: "driver.jar",
		FilePath: tmpFile.Name(),
	})
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !output.Success {
		t.Fatalf("expected success, got error: %s", output.Error)
	}
	if output.URI != "jar://test-uuid/driver.jar" {
		t.Fatalf("unexpected uri: %s", output.URI)
	}
}

func TestUploadFile_WithFilePath_RequiresExactlyOneSource(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))

	_, err := tools.NewToolWithFilePath(client).Handler(t.Context(), tools.InputWithFilePath{Filename: "x"})
	if err == nil {
		t.Fatalf("expected an error when neither contentBase64 nor filePath is provided")
	}

	_, err = tools.NewToolWithFilePath(client).Handler(t.Context(), tools.InputWithFilePath{
		Filename:      "x",
		ContentBase64: "Zm9v",
		FilePath:      "/tmp/x",
	})
	if err == nil {
		t.Fatalf("expected an error when both contentBase64 and filePath are provided")
	}
}

func TestUploadFile_WithFilePath_ReadError(t *testing.T) {
	client := testutil.NewClient(httptest.NewServer(http.NewServeMux()))

	output, err := tools.NewToolWithFilePath(client).Handler(t.Context(), tools.InputWithFilePath{
		Filename: "driver.jar",
		FilePath: "/nonexistent/path/driver.jar",
	})
	if err != nil {
		t.Fatalf("expected no error (failure reported via Output), got: %v", err)
	}
	if output.Success {
		t.Fatalf("expected failure for a nonexistent file")
	}
}
