// internal/services/usrsvc/create.go
package usrsvc

import (
	"context"
	"fmt"
	"github.com/hotkhwan/gateway-api/internal/gateways/authgw"
	"github.com/hotkhwan/gateway-api/models/usrmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"strings"
)

func CreateUser(ctx context.Context, req usrmod.CreateUserRequest) error {
	ctx, end, _ := traceutil.StartLite(
		ctx, "github.com/hotkhwan/gateway-api/usrsvc", "users.CreateUser",
		"usrsvc", "CreateUser",
	)
	defer end()

	token, err := authgw.GetAdminAccessToken(ctx)
	if err != nil {
		return err
	}

	attrs := map[string]any{
		"locale":     req.Locale,
		"role":       req.Role,
		"avatar":     req.Avatar,
		"department": req.Department,
		"perPages":   fmt.Sprintf("%d", req.PerPage),
		"refId":      req.RefId,
		"map_lat":    req.MapLocation.Lat,
		"map_lng":    req.MapLocation.Lng,
		"zoomLevel":  req.ZoomLevel,
	}

	// permission เป็น array (ถูกต้อง)
	if len(req.Permission) > 0 {
		attrs["permission"] = req.Permission
	}

	payload := map[string]any{
		"username":   req.Username,
		"firstName":  req.FirstName,
		"lastName":   req.LastName,
		"enabled":    req.Enabled,
		"attributes": attrs,
	}

	// ส่ง email เฉพาะถ้ามีค่า
	if strings.TrimSpace(req.Email) != "" {
		payload["email"] = req.Email
	}

	// 1) create
	if err := authgw.CreateUser(ctx, token, payload); err != nil {
		return err
	}

	// 2) find userId
	userID, err := authgw.FindUserIDByUsername(ctx, token, req.Username)
	if err != nil {
		return err
	}

	// 3) set password
	if err := authgw.SetPassword(ctx, token, userID, req.Password); err != nil {
		return err
	}

	// 4) assign role
	if req.Role != "" {
		_ = authgw.AssignRealmRole(ctx, token, userID, req.Role)
	}

	return nil
}
