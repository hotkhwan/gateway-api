// utils/httputil/retry.go
package httputil

import (
	"context"
	"errors"
	"io"
	"math"
	"math/rand"
	"net"
	"net/http"
	"time"
)

type RetryCfg struct {
	MaxAttempts int
	BaseDelay   time.Duration // e.g. 200ms
	MaxDelay    time.Duration // e.g. 5s
	Jitter      bool
}

// Retry runs fn up to MaxAttempts with exponential backoff.
// fn should return (retry=true, err) when it wants another attempt.
func Retry(ctx context.Context, cfg RetryCfg, fn func(attempt int) (retry bool, err error)) error {
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 1
	}
	if cfg.BaseDelay <= 0 {
		cfg.BaseDelay = 200 * time.Millisecond
	}
	if cfg.MaxDelay <= 0 {
		cfg.MaxDelay = 5 * time.Second
	}

	var lastErr error
	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		shouldRetry, err := fn(attempt)
		if err == nil {
			return nil
		}
		lastErr = err

		if !shouldRetry || attempt == cfg.MaxAttempts {
			return err
		}

		sleep := backoff(cfg.BaseDelay, attempt, cfg.MaxDelay, cfg.Jitter)
		timer := time.NewTimer(sleep)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func backoff(base time.Duration, attempt int, max time.Duration, jitter bool) time.Duration {
	d := time.Duration(float64(base) * math.Pow(2, float64(attempt-1)))
	if d > max {
		d = max
	}
	if jitter {
		// 50–100% ของ d
		return d/2 + time.Duration(rand.Int63n(int64(d/2)))
	}
	return d
}

// RetryableStatus returns true for HTTP codes that are typically safe to retry.
func RetryableStatus(s int) bool {
	switch s {
	case http.StatusRequestTimeout, // 408
		http.StatusTooEarly,        // 425
		http.StatusTooManyRequests, // 429
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	default:
		// Some CDNs use 52x
		return s >= 520 && s <= 527
	}
}

// RetryableErr returns true for transient network/io errors that are worth retrying.
func RetryableErr(err error) bool {
	var ne net.Error
	if errors.As(err, &ne) && (ne.Timeout() || ne.Temporary()) {
		return true
	}
	// Common IO transient errors
	return errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.ErrClosedPipe)
}
