// internal/services/authzsvc/membership.go
package authzsvc

import "context"

// MembershipService stub for Kafka consumer wiring
type MembershipService struct{}

func (m *MembershipService) HandleMembershipEvent(ctx context.Context, topic string, event interface{}) error {
	// TODO: implement tuple write/revoke logic
	return nil
}
