// internal/repo/targetrepo/mongoBootstrap.go
package targetrepo

import (
	"context"

	"github.com/hotkhwan/gateway-api/config"
)

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		return NewTargetRepo().EnsureIndexes(ctx)
	})
}
