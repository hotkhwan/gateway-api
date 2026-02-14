// internal/services/thirdsvc/random.go
package thirdsvc

import (
	"context"
	"fmt"
	"time"

	"klynx/models/thirdmod"

	"github.com/google/uuid"
)

func GenerateRandomClient(ctx context.Context, desc string) (string, string, error) {
	id := fmt.Sprintf("svc-%s", uuid.New().String())
	// จะไปเรียก Keycloak Admin API เหมือนเดิมได้เลย
	out, err := CreateServiceAccountClient(ctx, thirdmod.CreateServiceAccountInput{
		ClientID:    id,
		Description: desc,
	})
	if err != nil {
		return "", "", err
	}
	_ = time.Now() // เพื่อ chain ctx ให้ครบ trace
	return out.ClientID, out.ClientSecret, nil
}
