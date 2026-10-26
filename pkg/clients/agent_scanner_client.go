package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
)

const agentScanPath = "/rest/guardianFingerprint/v1/scans"

type AgentScanMetadata struct {
	DomainID string `json:"domainId,omitempty"`
}

type CreateAgentScanRequest struct {
	Document   json.RawMessage    `json:"document"`
	ExternalID string             `json:"externalId"`
	Metadata   *AgentScanMetadata `json:"metadata,omitempty"`
}

type AgentScan struct {
	ID            string `json:"id"`
	FingerprintID string `json:"fingerprintId"`
	CreatedOn     string `json:"createdOn"`
	GeneratedOn   string `json:"generatedOn"`
}

type AgentScanAPIError struct {
	StatusCode  int
	ErrorCode   string
	UserMessage string
}

func (e *AgentScanAPIError) Error() string {
	msg := fmt.Sprintf("HTTP %d", e.StatusCode)
	if e.ErrorCode != "" {
		msg += " [" + e.ErrorCode + "]"
	}
	if e.UserMessage != "" {
		msg += ": " + e.UserMessage
	}
	return msg
}

func CreateAgentScan(ctx context.Context, collibraHttpClient *http.Client, scanRequest CreateAgentScanRequest) (*AgentScan, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Creating agent scan for externalId: %s", scanRequest.ExternalID))

	payload, err := marshalWithoutHTMLEscaping(scanRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal scan request: %w", err)
	}

	// Sent once on purpose: every submission records a scan, so a retry could duplicate it.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, agentScanPath, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	body, status, err := executeRequestWithStatus(collibraHttpClient, req)
	if err != nil {
		if status == 0 {
			return nil, err
		}
		if status >= 300 && status < 400 {
			return nil, signInRequiredError()
		}
		return nil, parseAgentScanAPIError(status, body)
	}

	var scan AgentScan
	if err := json.Unmarshal(body, &scan); err != nil || scan.ID == "" {
		if looksLikeHTML(body) {
			return nil, signInRequiredError()
		}
		return nil, errors.New("Collibra answered success but the response did not contain a scan id")
	}
	return &scan, nil
}

// Not an API error code: reported when the call ended on a sign-in page.
const AgentScanSignInRequiredCode = "signInRequired"

func signInRequiredError() *AgentScanAPIError {
	return &AgentScanAPIError{StatusCode: http.StatusUnauthorized, ErrorCode: AgentScanSignInRequiredCode}
}

func looksLikeHTML(body []byte) bool {
	return bytes.HasPrefix(bytes.TrimSpace(body), []byte("<"))
}

func parseAgentScanAPIError(status int, body []byte) *AgentScanAPIError {
	apiErr := &AgentScanAPIError{StatusCode: status}
	var envelope struct {
		ErrorCode   string `json:"errorCode"`
		UserMessage string `json:"userMessage"`
	}
	if json.Unmarshal(body, &envelope) == nil {
		apiErr.ErrorCode = envelope.ErrorCode
		apiErr.UserMessage = envelope.UserMessage
	}
	return apiErr
}

func marshalWithoutHTMLEscaping(v any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
