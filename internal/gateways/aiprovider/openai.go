// internal/gateways/aiprovider/openai.go
package aiprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	openaiDefaultModel   = "gpt-4o-mini"
	openaiEndpoint       = "https://api.openai.com/v1/chat/completions"
	openaiDefaultMaxToks = 2048
)

// OpenAIProvider implements AIProvider for the OpenAI Chat Completions API.
type OpenAIProvider struct {
	apiKey string
	model  string
	http   *http.Client
}

// NewOpenAIProvider creates an OpenAIProvider with the given API key and model.
func NewOpenAIProvider(apiKey, model string) *OpenAIProvider {
	if model == "" {
		model = openaiDefaultModel
	}
	return &OpenAIProvider{
		apiKey: apiKey,
		model:  model,
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *OpenAIProvider) Name() string { return "openai" }

// Complete sends the prompt to OpenAI and returns the raw JSON string.
func (p *OpenAIProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	maxToks := req.MaxTokens
	if maxToks <= 0 {
		maxToks = openaiDefaultMaxToks
	}

	payload := map[string]any{
		"model": p.model,
		"messages": []map[string]any{
			{"role": "user", "content": req.Prompt},
		},
		"response_format": map[string]string{"type": "json_object"},
		"max_tokens":      maxToks,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, openaiEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	start := time.Now()
	resp, err := p.http.Do(httpReq)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("openai: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	rawJSON, err := extractOpenAIText(respBody)
	if err != nil {
		return nil, fmt.Errorf("openai: extract text: %w", err)
	}

	return &CompletionResponse{RawJSON: rawJSON, LatencyMs: latency}, nil
}

// openaiResponse is the minimal subset of OpenAI's response envelope we need.
type openaiResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func extractOpenAIText(body []byte) (string, error) {
	var or openaiResponse
	if err := json.Unmarshal(body, &or); err != nil {
		return "", fmt.Errorf("unmarshal openai response: %w", err)
	}
	if len(or.Choices) == 0 {
		return "", fmt.Errorf("openai response has no choices")
	}
	return or.Choices[0].Message.Content, nil
}
