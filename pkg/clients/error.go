package clients

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// collibraStandardError is the error envelope returned by Collibra APIs on
// non-2xx responses. Both the Semantic Blueprint and Context Engine APIs use
// this shape.
type collibraStandardError struct {
	StatusCode   int    `json:"statusCode"`
	ErrorCode    string `json:"errorCode"`
	TitleMessage string `json:"titleMessage,omitempty"`
	UserMessage  string `json:"userMessage,omitempty"`
	HelpMessage  string `json:"helpMessage,omitempty"`
}

// executeCollibraRequest is identical to executeRequest but parses the
// Collibra StandardErrorResponse envelope on non-2xx responses, surfacing the
// machine-readable errorCode and user-facing userMessage so the calling model
// can understand why the call failed.
func executeCollibraRequest(client *http.Client, req *http.Request) ([]byte, error) {
	body, _, err := executeCollibraRequestWithStatus(client, req)
	return body, err
}

// executeCollibraRequestWithStatus is executeCollibraRequest plus the response's HTTP status
// code, for callers that need to branch on it (e.g. 403 vs. 404 vs. everything else) rather than
// just surfacing the error message. See executeRequestWithStatus for the non-envelope-aware
// equivalent used by callers that don't go through the Collibra standard error envelope.
func executeCollibraRequestWithStatus(client *http.Client, req *http.Request) ([]byte, int, error) {
	response, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to make request: %w", err)
	}
	defer func(body io.ReadCloser) {
		_ = body.Close()
	}(response.Body)

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, response.StatusCode, fmt.Errorf("failed to read response body: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var errResp collibraStandardError
		if jsonErr := json.Unmarshal(responseBody, &errResp); jsonErr == nil && errResp.ErrorCode != "" {
			msg := fmt.Sprintf("HTTP %d [%s]", response.StatusCode, errResp.ErrorCode)
			if errResp.UserMessage != "" {
				msg += ": " + errResp.UserMessage
			}
			if errResp.HelpMessage != "" {
				msg += ". Hint: " + errResp.HelpMessage
			}
			return responseBody, response.StatusCode, errors.New(msg)
		}
		return responseBody, response.StatusCode, fmt.Errorf("HTTP %d: %s", response.StatusCode, string(responseBody))
	}

	return responseBody, response.StatusCode, nil
}
