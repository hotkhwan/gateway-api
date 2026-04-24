// internal/grpc/targetgrpc/redact.go
package targetgrpc

import (
	"time"

	"github.com/hotkhwan/gateway-api/models/authzmod"
)

// toView converts a canonical DeliveryTarget into the wire view with secrets
// redacted. The rule: never transmit a secret value across process boundaries.
// Boolean "*Set" markers let the FE render "configured ✓" without exposing
// the credential. Non-secret fields (URL, chatId, recipients) pass through.
//
// Callers must build all gRPC outbound responses through toView — the raw
// authzmod.DeliveryTarget must never hit the wire.
func toView(t *authzmod.DeliveryTarget) *DeliveryTargetView {
	if t == nil {
		return nil
	}
	return &DeliveryTargetView{
		TargetID:    t.TargetId,
		WorkspaceID: t.WorkspaceId,
		TenantID:    t.TenantId,
		Name:        t.Name,
		Type:        t.Type,
		Mode:        t.Mode,
		Enabled:     t.Enabled,
		Config:      toConfigView(t.Config),
		CreatedBy:   t.CreatedBy,
		CreatedAt:   formatRFC3339(t.CreatedAt),
		UpdatedAt:   formatRFC3339(t.UpdatedAt),
	}
}

func toConfigView(c authzmod.TargetConfig) TargetConfigView {
	return TargetConfigView{
		URL:                      c.URL,
		Headers:                  c.Headers,
		SigningEnabled:           c.SigningEnabled,
		SigningSecretSet:         c.SigningSecret != "",
		TimeoutMs:                c.TimeoutMs,
		ChannelAccessTokenSet:    c.ChannelAccessToken != "",
		ChannelAccessTokenRefSet: c.ChannelAccessTokenRef != "",
		To:                       []string(c.To),
		BotTokenSet:              c.BotToken != "",
		ChatID:                   c.ChatId,
	}
}

func formatRFC3339(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
