// internal/metrics/eps/server.go
package eps

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Server is a standalone HTTP listener that serves /metrics on a dedicated
// internal port. It is intentionally NOT mounted on the public Fiber app or
// the istio HTTPRoute, so the per-workspace/customer labeled license_eps_*
// series stay on the in-cluster scrape path only (review-gate F3): one tenant
// must never be able to read every tenant's EPS via labels.
//
// A ServiceMonitor (infra-side, with the gateway-api deploy) targets this port.
type Server struct {
	srv *http.Server
}

// NewServer registers the given collectors on a private registry (no default
// Go/process collectors — keep the internal surface minimal) and returns a
// listener bound to addr (e.g. ":9091").
func NewServer(addr string, collectors ...prometheus.Collector) (*Server, error) {
	reg := prometheus.NewRegistry()
	for _, col := range collectors {
		if err := reg.Register(col); err != nil {
			return nil, err
		}
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))

	return &Server{
		srv: &http.Server{
			Addr:              addr,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}, nil
}

// Start serves in the background. Returns immediately; a normal Shutdown closes
// the listener without surfacing the expected http.ErrServerClosed.
func (s *Server) Start() {
	go func() {
		_ = s.srv.ListenAndServe()
	}()
}

// Shutdown gracefully stops the listener.
func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil || s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}
