package eventmgmtrepo

import (
	"context"
	"github.com/hotkhwan/gateway-api/config"
)

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		return NewEventManagementRepo().EnsureIndexes(ctx)
	})
}
