// internal/gateways/aiprovider/claude.go
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
	claudeDefaultModel   = "claude-haiku-4-5-20251001"
	claudeEndpoint       = "https://api.anthropic.com/v1/messages"
	claudeAPIVersion     = "2023-06-01"
	claudeDefaultMaxToks = 2048
	claudeToolName       = "ai_suggest_result"
)

// ClaudeProvider implements AIProvider for the Anthropic Claude Messages API.
// It uses Tool Use (function calling) to force structured JSON output matching AISuggestRawResult.
type ClaudeProvider struct {
	apiKey string
	model  string
	http   *http.Client
}

// NewClaudeProvider creates a ClaudeProvider with the given API key and model.
func NewClaudeProvider(apiKey, model string) *ClaudeProvider {
	if model == "" {
		model = claudeDefaultModel
	}
	return &ClaudeProvider{
		apiKey: apiKey,
		model:  model,
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *ClaudeProvider) Name() string { return "claude" }

// claudeToolSchema describes the JSON schema for AISuggestRawResult that Claude
// must populate via tool use.
var claudeToolSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"suggestedEventType": map[string]any{"type": "string"},
		"fieldMappings": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"sourceField": map[string]any{"type": "string"},
					"targetField": map[string]any{"type": "string"},
					"valueCodes": map[string]any{
						"type":                 "object",
						"additionalProperties": map[string]any{"type": "string"},
					},
				},
				"required": []string{"sourceField", "targetField"},
			},
		},
		"matchRules": map[string]any{
			"type": "array",
			"items": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"fieldPath": map[string]any{"type": "string"},
					"operator":  map[string]any{"type": "string", "enum": []string{"eq", "exists", "contains"}},
					"value":     map[string]any{},
					"required":  map[string]any{"type": "boolean"},
					"reason":    map[string]any{"type": "string"},
				},
				"required": []string{"fieldPath", "operator", "required"},
			},
		},
	},
	"required": []string{"suggestedEventType", "fieldMappings", "matchRules"},
}

// Complete sends the prompt to Claude using Tool Use to get structured JSON output.
func (p *ClaudeProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	maxToks := req.MaxTokens
	if maxToks <= 0 {
		maxToks = claudeDefaultMaxToks
	}

	payload := map[string]any{
		"model":      p.model,
		"max_tokens": maxToks,
		"tools": []map[string]any{
			{
				"name":        claudeToolName,
				"description": "Return the AI mapping suggestion result as structured JSON.",
				"input_schema": claudeToolSchema,
			},
		},
		"tool_choice": map[string]string{"type": "tool", "name": claudeToolName},
		"messages": []map[string]any{
			{"role": "user", "content": req.Prompt},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("claude: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, claudeEndpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("claude: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", claudeAPIVersion)

	start := time.Now()
	resp, err := p.http.Do(httpReq)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("claude: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("claude: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("claude: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	rawJSON, err := extractClaudeToolInput(respBody)
	if err != nil {
		return nil, fmt.Errorf("claude: extract tool input: %w", err)
	}

	return &CompletionResponse{RawJSON: rawJSON, LatencyMs: latency}, nil
}

// claudeMessagesResponse is the minimal subset of Claude's response envelope.
type claudeMessagesResponse struct {
	Content []struct {
		Type  string          `json:"type"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	} `json:"content"`
}

// extractClaudeToolInput finds the tool_use block for claudeToolName and
// returns its input as a JSON string.
func extractClaudeToolInput(body []byte) (string, error) {
	var cr claudeMessagesResponse
	if err := json.Unmarshal(body, &cr); err != nil {
		return "", fmt.Errorf("unmarshal claude response: %w", err)
	}
	for _, block := range cr.Content {
		if block.Type == "tool_use" && block.Name == claudeToolName {
			if len(block.Input) == 0 {
				return "", fmt.Errorf("claude tool_use block has empty input")
			}
			return string(block.Input), nil
		}
	}
	return "", fmt.Errorf("claude response contains no tool_use block named %q", claudeToolName)
}
