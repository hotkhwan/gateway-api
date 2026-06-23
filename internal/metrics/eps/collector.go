// internal/metrics/eps/collector.go
package eps

import (
	"context"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Collector exports the per-workspace EPS soft metric as Prometheus gauges.
// It reads the recorder's ring buffers out-of-band on each scrape and joins
// the licensed limit + customer label from the injected resolvers.
//
// Fail-open semantics:
//   - limit <= 0 or limit lookup error  → emit current(+peak) only; omit
//     limit + percent (never a false "exceeded").
//   - customer lookup error             → emit with an empty customer label.
type Collector struct {
	rec       *Recorder
	limits    LimitResolver
	customers CustomerResolver
	timeout   time.Duration

	currentDesc *prometheus.Desc
	peakDesc    *prometheus.Desc
	limitDesc   *prometheus.Desc
	percentDesc *prometheus.Desc
}

// NewCollector builds the collector. limits and/or customers may be nil
// (limit/percent or customer label are then simply omitted/blank).
func NewCollector(rec *Recorder, limits LimitResolver, customers CustomerResolver) *Collector {
	labels := []string{"workspace", "customer"}
	return &Collector{
		rec:       rec,
		limits:    limits,
		customers: customers,
		timeout:   2 * time.Second,
		currentDesc: prometheus.NewDesc(
			"license_eps_current",
			"Current per-workspace events/sec (most recent completed 1s) on gw.events.normalized.v1.",
			labels, nil,
		),
		peakDesc: prometheus.NewDesc(
			"license_eps_peak",
			"Smoothed peak events/sec (max 10s-average) observed for the workspace.",
			labels, nil,
		),
		limitDesc: prometheus.NewDesc(
			"license_eps_limit",
			"Licensed maxEventsPerSecond for the workspace (emitted only when > 0).",
			labels, nil,
		),
		percentDesc: prometheus.NewDesc(
			"license_eps_percent",
			"Current EPS as a percentage of the licensed limit (emitted only when limit > 0).",
			labels, nil,
		),
	}
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.currentDesc
	ch <- c.peakDesc
	ch <- c.limitDesc
	ch <- c.percentDesc
}

// Collect implements prometheus.Collector. Called on every scrape.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	if c.rec == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	for _, ws := range c.rec.SnapshotAll() {
		customer := c.resolveCustomer(ctx, ws.WorkspaceID)

		ch <- prometheus.MustNewConstMetric(c.currentDesc, prometheus.GaugeValue, ws.Current, ws.WorkspaceID, customer)
		ch <- prometheus.MustNewConstMetric(c.peakDesc, prometheus.GaugeValue, ws.Peak, ws.WorkspaceID, customer)

		limit := c.resolveLimit(ctx, ws.WorkspaceID)
		if limit > 0 {
			ch <- prometheus.MustNewConstMetric(c.limitDesc, prometheus.GaugeValue, float64(limit), ws.WorkspaceID, customer)
			ch <- prometheus.MustNewConstMetric(c.percentDesc, prometheus.GaugeValue, ws.Current/float64(limit)*100.0, ws.WorkspaceID, customer)
		}
	}
}

// resolveCustomer returns the customer label, blank on missing resolver/error.
func (c *Collector) resolveCustomer(ctx context.Context, workspaceID string) string {
	if c.customers == nil {
		return ""
	}
	cust, err := c.customers.CustomerForWorkspace(ctx, workspaceID)
	if err != nil {
		return ""
	}
	return cust
}

// resolveLimit returns the licensed limit, 0 (=unknown) on missing resolver/error.
func (c *Collector) resolveLimit(ctx context.Context, workspaceID string) int {
	if c.limits == nil {
		return 0
	}
	limit, err := c.limits.MaxEventsPerSecond(ctx, workspaceID)
	if err != nil {
		return 0 // fail-open
	}
	return limit
}
