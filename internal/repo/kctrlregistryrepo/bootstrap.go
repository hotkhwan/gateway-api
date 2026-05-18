// internal/repo/kctrlregistryrepo/bootstrap.go
package kctrlregistryrepo

import (
	"context"

	"github.com/hotkhwan/gateway-api/config"
)

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		return NewKctrlRegistryRepo().EnsureIndexes(ctx)
	})
}
