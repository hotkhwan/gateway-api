// internal/services/authzsvc/auditlog.go
package authzsvc

import (
	"context"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"sync"
	"time"
)

var (
	auditMu    sync.Mutex
	auditStore []authzmod.AuditLog
)

// LogAudit stores an audit log entry (in-memory stub)
func LogAudit(ctx context.Context, entry authzmod.AuditLog) error {
	auditMu.Lock()
	defer auditMu.Unlock()
	entry.CreatedAt = time.Now()
	auditStore = append(auditStore, entry)
	return nil
}
