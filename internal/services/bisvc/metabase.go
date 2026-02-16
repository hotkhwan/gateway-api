// internal/services/bisvc/metabase.go
package bisvc

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type MetabaseResponse struct {
	BaseURL string `json:"baseUrl"`
}

var ErrConfigNotSet = fmt.Errorf("metabase env not configured")

// GenerateSignedURL: เซ็น JWT และคืน base URL เท่านั้น
func GenerateSignedURL(ctx context.Context, dashboardIdStr string) (*MetabaseResponse, error) {
	tr := otel.Tracer("github.com/hotkhwan/gateway-api/bisvc")
	ctx, span := tr.Start(ctx, "BI.GenerateSignedURL")
	defer span.End()

	siteURL := os.Getenv("METABASE_SITE_URL") // ใส่ /bi ถ้า Metabase รันใต้ subpath
	secret := os.Getenv("METABASE_SECRET_KEY")
	if siteURL == "" || secret == "" {
		return nil, ErrConfigNotSet
	}

	did, _ := strconv.Atoi(dashboardIdStr)
	if did <= 0 {
		did = 4
	}

	claims := jwt.MapClaims{
		"resource": map[string]any{"dashboard": did},
		"params":   map[string]any{},
		"exp":      time.Now().Add(10 * time.Minute).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString([]byte(secret))
	if err != nil {
		return nil, err
	}

	base := siteURL + "/embed/dashboard/" + signed

	span.SetAttributes(
		attribute.Int("metabase.dashboard_id", did),
		attribute.String("metabase.site_url", siteURL),
	)

	return &MetabaseResponse{BaseURL: base}, nil
}
