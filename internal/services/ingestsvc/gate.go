// internal/services/ingestsvc/gate.go
package ingestsvc

import "github.com/hotkhwan/gateway-api/models/ingestmod"

// runApprovalGate validates that a pending event satisfies all prerequisites
// before the approval workflow begins.
//
// Returns:
//   - nil                 — event may proceed to approval
//   - ErrInvalidEventType — eventType is empty; field mapping is required before approval
func runApprovalGate(pending *ingestmod.EventManagement) error {
	if pending.EventType == "" {
		return ErrInvalidEventType
	}
	return nil
}
