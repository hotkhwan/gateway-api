// internal/repo/sourceprofilerepo/bootstrap.go
package sourceprofilerepo

import (
	"context"

	"github.com/hotkhwan/gateway-api/config"
)

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		return NewSourceProfileRepo().EnsureIndexes(ctx)
	})
}
