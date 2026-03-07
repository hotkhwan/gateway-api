// internal/repo/dlqrepo/mongoBootstrap.go
package dlqrepo

import (
	"context"

	"github.com/hotkhwan/gateway-api/config"
)

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		return NewDLQRepo().EnsureIndexes(ctx)
	})
}
