// internal/repo/ingestrepo/mongoBootstrap.go
package ingestrepo

import (
	"context"

	"github.com/hotkhwan/gateway-api/config"
)

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		return NewMappingTemplateRepo().EnsureIndexes(ctx)
	})
}
