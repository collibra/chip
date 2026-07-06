// Package upload_file implements the upload_file MCP tool: uploads an arbitrary file
// to an edge site and returns an artifact URI usable as the value of any FILE-type
// connection or capability parameter (JDBC drivers, TLS certs, private keys, keytabs,
// etc.) — not just the JDBC-driver convenience built into create_connection.
package upload_file

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

	"github.com/collibra/chip/pkg/chip"
	"github.com/collibra/chip/pkg/clients"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Input struct {
	Filename      string `json:"filename" jsonschema:"The filename to upload as (e.g. 'client-cert.pem'). Determines the file extension DGC uses to infer the artifact type."`
	URL           string `json:"url,omitempty" jsonschema:"A URL to download the file's content from (e.g. a Maven Central artifact URL or an internal file server). Exactly one of url or contentBase64 must be provided."`
	ContentBase64 string `json:"contentBase64,omitempty" jsonschema:"The file's content, base64-encoded. Exactly one of url or contentBase64 must be provided."`
}

type Output struct {
	URI     string `json:"uri,omitempty" jsonschema:"The uploaded artifact URI (e.g. 'jar://<uuid>/<filename>'). Use this as the value of a FILE-type connection or capability parameter."`
	Success bool   `json:"success" jsonschema:"Whether the file was uploaded successfully."`
	Error   string `json:"error,omitempty" jsonschema:"Error message if the upload failed."`
}

func NewTool(collibraClient *http.Client) *chip.Tool[Input, Output] {
	return &chip.Tool[Input, Output]{
		Name:        "upload_file",
		Title:       "Upload File to Edge Site",
		Description: "Uploads a file (JDBC driver, TLS certificate, private key, keytab, etc.) to an edge site and returns an artifact URI usable as the value of any FILE-type connection or capability parameter.",
		Handler:     handler(collibraClient),
		Permissions: []string{},
		Annotations: &mcp.ToolAnnotations{DestructiveHint: chip.Ptr(true)},
	}
}

func handler(collibraClient *http.Client) chip.ToolHandlerFunc[Input, Output] {
	return func(ctx context.Context, input Input) (Output, error) {
		if input.Filename == "" {
			return Output{}, fmt.Errorf("filename is required")
		}
		if (input.URL == "") == (input.ContentBase64 == "") {
			return Output{}, fmt.Errorf("exactly one of url or contentBase64 must be provided")
		}

		content, err := resolveContent(ctx, input)
		if err != nil {
			return Output{Success: false, Error: err.Error()}, nil
		}

		uri, err := clients.UploadFile(ctx, collibraClient, input.Filename, content)
		if err != nil {
			return Output{Success: false, Error: fmt.Sprintf("failed to upload file: %s", err.Error())}, nil
		}

		return Output{URI: uri, Success: true}, nil
	}
}

func resolveContent(ctx context.Context, input Input) ([]byte, error) {
	if input.ContentBase64 != "" {
		content, err := base64.StdEncoding.DecodeString(input.ContentBase64)
		if err != nil {
			return nil, fmt.Errorf("decoding contentBase64: %w", err)
		}
		return content, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, input.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("building download request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading file: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("downloading file: unexpected status %d", resp.StatusCode)
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading downloaded content: %w", err)
	}

	return content, nil
}
