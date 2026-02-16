// internal/gateways/gwcom/http.go
package gwcom

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hotkhwan/gateway-api/utils/httputil"
)

func PostJSON(ctx context.Context, url string, headers map[string]string, body io.Reader) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, body)
	if err != nil {
		return nil, 0, err
	}
	httputil.WithHeaders(req, map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	})
	httputil.WithHeaders(req, headers)
	return doWithRetry(ctx, req)
}

func PutJSON(ctx context.Context, url string, headers map[string]string, body io.Reader) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, body)
	if err != nil {
		return nil, 0, err
	}
	httputil.WithHeaders(req, map[string]string{
		"Content-Type": "application/json",
		"Accept":       "application/json",
	})
	httputil.WithHeaders(req, headers)
	return doWithRetry(ctx, req)
}

func Delete(ctx context.Context, url string, headers map[string]string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return nil, 0, err
	}
	httputil.WithHeaders(req, headers)
	return doWithRetry(ctx, req)
}

// เหมาะเวลาอยาก re-send body ใน retry
func doWithRetry(ctx context.Context, req *http.Request) ([]byte, int, error) {
	var (
		outBody []byte
		status  int
	)
	client := &http.Client{Timeout: 15 * time.Second}

	// สำรอง body (ถ้ามี) เพื่อ retry
	var backup []byte
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		backup = b
		req.Body.Close()
		req.Body = io.NopCloser(bytes.NewReader(b))
	}

	err := httputil.Retry(ctx, httputil.RetryCfg{MaxAttempts: 3, BaseDelay: 200 * time.Millisecond, MaxDelay: 2 * time.Second, Jitter: true},
		func(_ int) (bool, error) {
			if backup != nil {
				req.Body = io.NopCloser(bytes.NewReader(backup))
			}
			resp, e := client.Do(req)
			if e != nil {
				return httputil.RetryableErr(e), e
			}
			defer resp.Body.Close()
			b, e := io.ReadAll(resp.Body)
			if e != nil {
				return httputil.RetryableErr(e), e
			}
			outBody = b
			status = resp.StatusCode
			if httputil.RetryableStatus(status) {
				return true, fmt.Errorf("retryable status %d", status)
			}
			return false, nil
		})
	if err != nil {
		return nil, status, err
	}
	return outBody, status, nil
}
