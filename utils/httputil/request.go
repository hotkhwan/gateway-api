// utils/httputil/request.go
package httputil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func WithHeaders(req *http.Request, headers map[string]string) {
	for k, v := range headers {
		if v == "" {
			continue
		}
		if req.Header.Get(k) == "" {
			req.Header.Set(k, v)
		}
	}
}

func CreateJSONRequest(ctx context.Context, method, url string, body []byte, accessToken string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create JSON request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "klynx-api/iboc-client")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	return req, nil
}

func CreateJSONRequestFrom(ctx context.Context, method, url string, payload any, accessToken string) (*http.Request, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return CreateJSONRequest(ctx, method, url, b, accessToken)
}

func CreateFormRequest(ctx context.Context, method, url string, buf *bytes.Buffer, contentType, accessToken string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, buf)
	if err != nil {
		return nil, fmt.Errorf("failed to create form request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "klynx-api/iboc-client")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	return req, nil
}

func CreateFormRequestStream(ctx context.Context, method, url string, r io.Reader, contentType string, contentLength int64, accessToken string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, r)
	if err != nil {
		return nil, fmt.Errorf("failed to create streamed form request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "klynx-api/iboc-client")
	if contentLength >= 0 {
		req.ContentLength = contentLength
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	return req, nil
}
