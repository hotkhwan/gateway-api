// internal/services/usrsvc/manage.go
package usrsvc

import (
	"context"
	"fmt"
	"github.com/hotkhwan/gateway-api/internal/gateways/authgw"
)

func UpdateUser(ctx context.Context, userID string, attrs map[string]any) error {
	token, err := authgw.GetAdminAccessToken(ctx)
	if err != nil {
		return err
	}

	// 1) โหลด user เดิม
	current, err := authgw.GetUser(ctx, token, userID)
	if err != nil {
		return err
	}

	// 2) attributes เดิม
	existingAttrs := map[string][]string{}
	if raw, ok := current["attributes"].(map[string]any); ok {
		for k, v := range raw {
			if arr, ok := v.([]any); ok {
				out := make([]string, 0, len(arr))
				for _, x := range arr {
					out = append(out, fmt.Sprintf("%v", x))
				}
				existingAttrs[k] = out
			}
		}
	}

	// 3) merge attributes ใหม่
	setAttr := func(k string, v any) {
		existingAttrs[k] = []string{fmt.Sprintf("%v", v)}
	}

	if v, ok := attrs["role"]; ok {
		setAttr("role", v)
	}
	if v, ok := attrs["locale"]; ok {
		setAttr("locale", v)
	}
	if v, ok := attrs["avatar"]; ok {
		setAttr("avatar", v)
	}
	if v, ok := attrs["plant"]; ok {
		setAttr("plant", v)
	}
	if v, ok := attrs["perPages"]; ok {
		setAttr("perPages", v)
	}

	if ml, ok := attrs["mapLocation"].(map[string]any); ok {
		if lat, ok := ml["lat"]; ok {
			setAttr("map_lat", lat)
		}
		if lng, ok := ml["lng"]; ok {
			setAttr("map_lng", lng)
		}
	}
	if v, ok := attrs["zoomLevel"]; ok {
		setAttr("zoomLevel", v)
	}
	// 4) payload (สำคัญมาก)
	payload := map[string]any{
		"attributes": existingAttrs,
	}

	// top-level fields (ห้ามอยู่ใน attributes)
	for _, f := range []string{"firstName", "lastName", "email"} {
		if v, ok := attrs[f]; ok {
			payload[f] = v
		}
	}

	// 5) PUT ตรง ๆ (❗ ไม่ใช้ UpdateUserAttributes)
	return authgw.PutUser(ctx, token, userID, payload)
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return fmt.Sprintf("%v", t)
	case int:
		return fmt.Sprintf("%d", t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func SetUserEnabled(ctx context.Context, userID string, enabled bool) error {
	token, err := authgw.GetAdminAccessToken(ctx)
	if err != nil {
		return err
	}
	return authgw.SetEnabled(ctx, token, userID, enabled)
}

func ChangePassword(
	ctx context.Context,
	userID string,
	password string,
	temporary bool,
) error {

	token, err := authgw.GetAdminAccessToken(ctx)
	if err != nil {
		return err
	}

	return authgw.ChangePassword(ctx, token, userID, password, temporary)
}

func DeleteUser(ctx context.Context, userID string) error {
	token, err := authgw.GetAdminAccessToken(ctx)
	if err != nil {
		return err
	}
	return authgw.DeleteUser(ctx, token, userID)
}
