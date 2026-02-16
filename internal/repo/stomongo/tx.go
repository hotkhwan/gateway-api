// internal/adapters/repo/stomongo/tx.go
package stomongo

import (
	"context"

	"github.com/hotkhwan/gateway-api/config"

	"go.mongodb.org/mongo-driver/mongo"
)

func WithSession(ctx context.Context, fn func(mongo.SessionContext) error) error {
	client := config.DB.Client()
	session, err := client.StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)
	return mongo.WithSession(ctx, session, fn)
}

func WithTransaction(ctx context.Context, fn func(mongo.SessionContext) error) error {
	return WithSession(ctx, func(sc mongo.SessionContext) error {
		return sc.StartTransaction() // ให้ fn เรียก sc.CommitTransaction()/AbortTransaction() เอง
	})
}
