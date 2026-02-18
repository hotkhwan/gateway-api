// internal/repo/authzrepo/mongoBootstrap.go
package authzrepo

import (
	"context"

	"github.com/hotkhwan/gateway-api/config"
)

func init() {
	config.RegisterMongoBootstrap(func(ctx context.Context) error {
		// เรียก EnsureIndexes ของทุก repo ที่ต้องการ
		if err := NewOrgRepo(config.DB).EnsureIndexes(ctx); err != nil {
			return err
		}
		if err := NewOrgUnitRepo().EnsureIndexes(ctx); err != nil {
			return err
		}
		// ถ้ามี repo อื่นก็เพิ่มต่อได้
		// _ = NewOrgRepo(config.DB).EnsureIndexes(ctx)  // ตัวอย่าง ถ้ามี
		return nil
	})
}
