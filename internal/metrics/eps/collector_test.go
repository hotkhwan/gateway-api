// internal/metrics/eps/collector_test.go
package eps

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

type fakeLimit struct {
	val int
	err error
}

func (f fakeLimit) MaxEventsPerSecond(_ context.Context, _ string) (int, error) {
	return f.val, f.err
}

type fakeCustomer struct {
	val string
	err error
}

func (f fakeCustomer) CustomerForWorkspace(_ context.Context, _ string) (string, error) {
	return f.val, f.err
}

// seedCurrent makes ws's most-recent completed second hold n events.
func seedCurrent(r *Recorder, clk *int64, ws string, n int) {
	for i := 0; i < n; i++ {
		r.Observe(ws)
	}
	*clk++ // advance so the seeded second becomes "completed"
}

// gather registers c on a fresh registry and returns name -> first gauge value.
func gather(t *testing.T, c prometheus.Collector) map[string]float64 {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("register: %v", err)
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	out := map[string]float64{}
	for _, mf := range mfs {
		for _, m := range mf.GetMetric() {
			out[mf.GetName()] = m.GetGauge().GetValue()
		}
	}
	return out
}

func TestCollectorLicensed(t *testing.T) {
	var clk int64 = 1000
	r := newTestRecorder(&clk)
	seedCurrent(r, &clk, "ws1", 7)

	c := NewCollector(r, fakeLimit{val: 100}, fakeCustomer{val: "tenantA"})
	got := gather(t, c)

	if got["license_eps_current"] != 7 {
		t.Fatalf("current: got %v want 7", got["license_eps_current"])
	}
	if got["license_eps_limit"] != 100 {
		t.Fatalf("limit: got %v want 100", got["license_eps_limit"])
	}
	if p := got["license_eps_percent"]; p < 6.999 || p > 7.001 { // 7/100*100
		t.Fatalf("percent: got %v want ~7", p)
	}
}

func TestCollectorUnlimitedOmitsLimitAndPercent(t *testing.T) {
	var clk int64 = 1000
	r := newTestRecorder(&clk)
	seedCurrent(r, &clk, "ws1", 5)

	c := NewCollector(r, fakeLimit{val: 0}, fakeCustomer{val: "tenantA"})

	if n := countSeries(t, c, "license_eps_limit"); n != 0 {
		t.Fatalf("limit series under unlimited: got %d want 0", n)
	}
	if n := countSeries(t, c, "license_eps_percent"); n != 0 {
		t.Fatalf("percent series under unlimited: got %d want 0", n)
	}
	if n := countSeries(t, c, "license_eps_current"); n != 1 {
		t.Fatalf("current series: got %d want 1", n)
	}
}

func TestCollectorFailOpenOnLimitError(t *testing.T) {
	var clk int64 = 1000
	r := newTestRecorder(&clk)
	seedCurrent(r, &clk, "ws1", 5)

	c := NewCollector(r, fakeLimit{err: errors.New("redis down")}, fakeCustomer{val: "tenantA"})

	if n := countSeries(t, c, "license_eps_limit"); n != 0 {
		t.Fatalf("limit must be omitted on resolver error: got %d want 0", n)
	}
	if n := countSeries(t, c, "license_eps_percent"); n != 0 {
		t.Fatalf("percent must be omitted on resolver error: got %d want 0", n)
	}
	if n := countSeries(t, c, "license_eps_current"); n != 1 {
		t.Fatalf("current must still publish on limit error: got %d want 1", n)
	}
}

func TestCollectorCustomerLabel(t *testing.T) {
	var clk int64 = 1000
	r := newTestRecorder(&clk)
	seedCurrent(r, &clk, "ws1", 1)

	// customer resolver error → blank label, but the series still publishes.
	c := NewCollector(r, fakeLimit{val: 10}, fakeCustomer{err: errors.New("not found")})

	lbls := labelsFor(t, c, "license_eps_current")
	if lbls["workspace"] != "ws1" {
		t.Fatalf("workspace label: got %q want ws1", lbls["workspace"])
	}
	if lbls["customer"] != "" {
		t.Fatalf("customer label on resolver error: got %q want empty", lbls["customer"])
	}
}

// countSeries gathers c and counts series for the named metric.
func countSeries(t *testing.T, c prometheus.Collector, name string) int {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("register: %v", err)
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() == name {
			return len(mf.GetMetric())
		}
	}
	return 0
}

// labelsFor returns the label set of the first series of the named metric.
func labelsFor(t *testing.T, c prometheus.Collector, name string) map[string]string {
	t.Helper()
	reg := prometheus.NewRegistry()
	if err := reg.Register(c); err != nil {
		t.Fatalf("register: %v", err)
	}
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	out := map[string]string{}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, lp := range firstMetric(mf).GetLabel() {
			out[lp.GetName()] = lp.GetValue()
		}
	}
	return out
}

func firstMetric(mf *dto.MetricFamily) *dto.Metric {
	if len(mf.GetMetric()) == 0 {
		return &dto.Metric{}
	}
	return mf.GetMetric()[0]
}
