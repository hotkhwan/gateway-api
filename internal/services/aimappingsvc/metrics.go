// internal/services/aimappingsvc/metrics.go
package aimappingsvc

import "sync/atomic"

// Counters incremented atomically. Exposed via GetMetrics() for health endpoints.
var (
	suggestRequestsTotal   atomic.Int64
	suggestAIFailuresTotal atomic.Int64
	suggestFallbackTotal   atomic.Int64
	suggestParseFailures   atomic.Int64
)

func incRequests()     { suggestRequestsTotal.Add(1) }
func incAIFailures()   { suggestAIFailuresTotal.Add(1) }
func incFallback()     { suggestFallbackTotal.Add(1) }
func incParseFailures() { suggestParseFailures.Add(1) }

// AIMappingMetrics is a snapshot of all counters.
type AIMappingMetrics struct {
	RequestsTotal   int64 `json:"requestsTotal"`
	AIFailuresTotal int64 `json:"aiFailuresTotal"`
	FallbackTotal   int64 `json:"fallbackTotal"`
	ParseFailures   int64 `json:"parseFailures"`
}

// GetMetrics returns a snapshot of current counters.
func GetMetrics() AIMappingMetrics {
	return AIMappingMetrics{
		RequestsTotal:   suggestRequestsTotal.Load(),
		AIFailuresTotal: suggestAIFailuresTotal.Load(),
		FallbackTotal:   suggestFallbackTotal.Load(),
		ParseFailures:   suggestParseFailures.Load(),
	}
}
