// internal/gateways/aiprovider/gemini.go
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
	geminiDefaultModel   = "gemini-2.5-flash"
	geminiBaseURL        = "https://generativelanguage.googleapis.com/v1beta/models"
	geminiDefaultMaxToks = 2048
)

// GeminiProvider implements AIProvider for Google Gemini REST API.
type GeminiProvider struct {
	apiKey string
	model  string
	http   *http.Client
}

// NewGeminiProvider creates a GeminiProvider.
// apiKey is required — Gemini API does not allow anonymous/keyless access.
func NewGeminiProvider(apiKey, model string) *GeminiProvider {
	if model == "" {
		model = geminiDefaultModel
	}
	return &GeminiProvider{
		apiKey: apiKey,
		model:  model,
		http:   &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *GeminiProvider) Name() string { return "gemini" }

// Complete sends the prompt to Gemini and returns the raw JSON string.
func (p *GeminiProvider) Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("gemini: API key is required — please configure an API key from https://aistudio.google.com/apikey")
	}
	maxToks := req.MaxTokens
	if maxToks <= 0 {
		maxToks = geminiDefaultMaxToks
	}

	payload := map[string]any{
		"contents": []map[string]any{
			{
				"role":  "user",
				"parts": []map[string]any{{"text": req.Prompt}},
			},
		},
		"generationConfig": map[string]any{
			"responseMimeType": "application/json",
			"maxOutputTokens":  maxToks,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/%s:generateContent?key=%s", geminiBaseURL, p.model, p.apiKey)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	resp, err := p.http.Do(httpReq)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		return nil, fmt.Errorf("gemini: http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("gemini: read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini: %s", extractGeminiError(resp.StatusCode, respBody))
	}

	rawJSON, err := extractGeminiText(respBody)
	if err != nil {
		return nil, fmt.Errorf("gemini: extract text: %w", err)
	}

	return &CompletionResponse{RawJSON: rawJSON, LatencyMs: latency}, nil
}

// geminiErrorBody is used to parse Gemini error responses.
type geminiErrorBody struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// extractGeminiError returns a concise error string from a non-200 Gemini response.
func extractGeminiError(statusCode int, body []byte) string {
	var eb geminiErrorBody
	if err := json.Unmarshal(body, &eb); err == nil && eb.Error.Message != "" {
		msg := eb.Error.Message
		// Trim verbose quota details after the first newline
		if idx := len(msg); idx > 200 {
			msg = msg[:200] + "..."
		}
		switch statusCode {
		case 429:
			return fmt.Sprintf("quota exceeded (429) — %s. Try switching to gemini-1.5-flash or set up billing at aistudio.google.com", eb.Error.Status)
		case 403:
			return fmt.Sprintf("permission denied (403) — API key invalid or not authorized for this model")
		case 401:
			return "unauthorized (401) — invalid API key"
		default:
			return fmt.Sprintf("status %d %s: %s", statusCode, eb.Error.Status, msg)
		}
	}
	// Fallback: truncate raw body
	raw := string(body)
	if len(raw) > 300 {
		raw = raw[:300] + "..."
	}
	return fmt.Sprintf("status %d: %s", statusCode, raw)
}

// geminiResponse is the minimal subset of Gemini's response envelope we need.
// Parts may include "thought" parts (Gemini 2.5+ thinking mode) — we skip those.
type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text    string `json:"text"`
				Thought bool   `json:"thought"` // true for internal thinking parts (Gemini 2.5+)
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	PromptFeedback *struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
}

func extractGeminiText(body []byte) (string, error) {
	var gr geminiResponse
	if err := json.Unmarshal(body, &gr); err != nil {
		return "", fmt.Errorf("unmarshal gemini response: %w", err)
	}
	if gr.PromptFeedback != nil && gr.PromptFeedback.BlockReason != "" {
		return "", fmt.Errorf("prompt blocked by safety filter: %s", gr.PromptFeedback.BlockReason)
	}
	if len(gr.Candidates) == 0 {
		return "", fmt.Errorf("gemini response has no candidates")
	}
	cand := gr.Candidates[0]
	// Skip thought parts — find first regular text part.
	for _, p := range cand.Content.Parts {
		if !p.Thought && p.Text != "" {
			return p.Text, nil
		}
	}
	// Fallback: any part with text (handles non-thinking models).
	for _, p := range cand.Content.Parts {
		if p.Text != "" {
			return p.Text, nil
		}
	}
	reason := cand.FinishReason
	if reason == "" {
		reason = "unknown"
	}
	return "", fmt.Errorf("gemini response has no text content (finishReason: %s)", reason)
}
