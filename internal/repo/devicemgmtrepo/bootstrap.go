// internal/repo/devicemgmtrepo/bootstrap.go
package devicemgmtrepo

import (
	"context"

	"github.com/hotkhwan/gateway-api/config"
)

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		return NewDeviceManagementRepo().EnsureIndexes(ctx)
	})
}
