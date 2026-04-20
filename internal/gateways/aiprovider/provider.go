// internal/gateways/aiprovider/provider.go
package aiprovider

import "context"

// AIProvider is the interface that all AI backend adapters must implement.
type AIProvider interface {
	// Complete sends a prompt to the AI backend and returns a structured JSON response.
	Complete(ctx context.Context, req CompletionRequest) (*CompletionResponse, error)
	// Name returns the provider identifier (e.g. "gemini", "openai", "claude").
	Name() string
}

// CompletionRequest carries the full prompt (system + user merged) and a token budget.
type CompletionRequest struct {
	// Prompt is the full merged prompt text sent to the model.
	Prompt string
	// MaxTokens is the maximum number of output tokens the model may produce.
	MaxTokens int
}

// CompletionResponse carries the raw JSON string returned by the model and the
// measured round-trip latency.
type CompletionResponse struct {
	// RawJSON is the JSON string produced by the AI model.
	RawJSON string
	// LatencyMs is the wall-clock duration of the HTTP round trip in milliseconds.
	LatencyMs int64
}
