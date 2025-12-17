package clients

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

func AskGlossary(ctx context.Context, collibraHttpClient *http.Client, question string) (string, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Data glossary agent query: '%s'", question))
	return callTool(ctx, collibraHttpClient, "/rest/aiCopilot/v1/tools/askGlossary", question)
}

func AskDad(ctx context.Context, collibraHttpClient *http.Client, question string) (string, error) {
	slog.InfoContext(ctx, fmt.Sprintf("Data asset agent query: '%s'", question))
	return callTool(ctx, collibraHttpClient, "/rest/aiCopilot/v1/tools/askDad", question)
}

func callTool(ctx context.Context, collibraHttpClient *http.Client, endpoint string, question string) (string, error) {
	toolRequest := createToolRequest(question)

	jsonData, err := json.Marshal(toolRequest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	response, err := collibraHttpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(response.Body)

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if response.StatusCode < 200 || response.StatusCode >= 300 { // TODO: Handle 400 and 500 errors.
		return "", fmt.Errorf("HTTP %d: %s", response.StatusCode, string(body))
	}

	toolResponse, err := unmarshalToolResponse(body)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if len(toolResponse.Content) == 0 {
		return "", fmt.Errorf("empty response content")
	}

	return toolResponse.Content[0].Text, nil
}

func createToolRequest(question string) ToolRequest {
	return ToolRequest{
		Message: ToolMessage{
			MessagerRole: "user",
			Content:      ToolContent{Type: "text", Text: question},
		},
		History: []ToolMessage{},
	}
}

func unmarshalToolResponse(body []byte) (*ToolResponse, error) {
	var toolResponse ToolResponse
	if err := json.Unmarshal(body, &toolResponse); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return &toolResponse, nil
}

type ToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type ChatContext struct {
	OriginUrl string `json:"originUrl"`
}

type ToolMessage struct {
	MessagerRole string      `json:"messagerRole"`
	Content      ToolContent `json:"content"`
	Context      ChatContext `json:"context"`
}

type ToolRequest struct {
	Message ToolMessage   `json:"message"`
	History []ToolMessage `json:"history"`
}

type ToolResponse struct {
	Content []ToolContent `json:"content"`
}
