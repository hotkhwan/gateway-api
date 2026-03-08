// internal/repo/unknownpayloadreviewrepo/bootstrap.go
package unknownpayloadreviewrepo

import (
	"context"

	"github.com/hotkhwan/gateway-api/config"
)

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		return NewUnknownPayloadReviewRepo().EnsureIndexes(ctx)
	})
}
