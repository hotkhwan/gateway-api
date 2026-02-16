// internal/services/authsvc/introspect.go
package authsvc

import (
	"context"
	"fmt"
	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/services/authzsvc"
	"github.com/hotkhwan/gateway-api/models/authmod"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/utils/authutil"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
)

var relationToAllow = map[string]authmod.Allow{
	"owner":  authmod.AllowCRUD,
	"editor": authmod.AllowCU,
	"viewer": authmod.AllowRead,
	"admin":  authmod.AllowCRUD,
}

func Introspect(ctx context.Context, token string) (*authmod.IntrospectResult, error) {
	tracer := otel.Tracer("github.com/hotkhwan/gateway-api/authsvc")
	ctx, span := tracer.Start(ctx, "Auth.Introspect")
	defer span.End()

	log := logger.FromCtx(ctx, "authsvc", "Introspect")

	claims, err := authutil.ValidateJWT(token)
	if err != nil {
		log.Warn().Err(err).Msg("token validation failed")
		return &authmod.IntrospectResult{Active: false}, nil
	}

	expFloat, _ := claims["exp"].(float64)
	exp := int64(expFloat)
	if exp < time.Now().Unix() {
		log.Warn().Int64("exp", exp).Msg("token expired")
		return &authmod.IntrospectResult{Active: false}, nil
	}

	// ✅ ตรวจ revoke ด้วย ctx เดิม (ไม่ใช้ Background)
	if sid := firstNonEmpty(
		getStringClaim(claims, "session_state"),
		getStringClaim(claims, "jti"),
	); sid != "" {
		if exists, _ := config.Redis.Exists(ctx, "revoked:"+sid).Result(); exists > 0 {
			log.Warn().Str("sid", sid).Msg("token revoked by signout")
			return &authmod.IntrospectResult{Active: false}, nil
		}
	}

	// ✅ จำกัดเวลาคุยกับ Permify ด้วย ctx ลูก (ไม่ shadow ctx เดิม)
	permCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	subjectID := getStringClaim(claims, "sub")

	relationshipsRequest := &authzmod.RelationshipReadRequest{
		Filter: authzmod.RelationshipReadFilter{
			Subject: &authzmod.RelationshipEntityFilter{
				Type: "user",
				IDs:  []string{subjectID},
			},
		},
		PageSize: 100,
		Metadata: &authzmod.RelationshipReadMetadata{},
	}

	relationships, err := authzsvc.GetRelationships(permCtx, relationshipsRequest)
	if err != nil {
		log.Error().Err(err).Msg("get relationships from Permify failed")
		relationships = &authzmod.RelationshipReadResponse{}
	}

	// Build permissions
	permissionsMap := make(map[string]authmod.Allow)
	for _, d := range relationships.Details {
		if d.Entity.Type != "resource" && d.Entity.Type != "menu" {
			continue
		}
		allow, ok := relationToAllow[d.Relation]
		if !ok {
			continue
		}
		if curr, ok := permissionsMap[d.Entity.ID]; !ok || isHigherPermission(allow, curr) {
			permissionsMap[d.Entity.ID] = allow
		}
	}
	// perms := make([]authmod.Permission, 0, len(permissionsMap))
	// for res, allow := range permissionsMap {
	// 	perms = append(perms, authmod.Permission{Resource: res, Allow: allow})
	// }

	// region  Get menu grants
	// ---- 1) โหลดเมนูจาก Mongo ----
	menuIDs, err := loadMenuIDs(ctx)
	coreMenuIDs := []string{"HOME", "DASHBOARD", "MAP", "VIDEO_WALL"}
	log.Info().Str("menuIDs", fmt.Sprintf("%v", menuIDs)).Msg("Loaded menu options")
	if err != nil || len(menuIDs) == 0 {
		log.Warn().Err(err).Msg("⚠️ Failed to load menu options, using hard-coded fallback")

		// fallback เฉพาะ core menus ที่อยากให้ระบบยังพอใช้ได้
		menuIDs = []string{
			"home",
			"dashboard",
			"map",
			"videowall",
		}
	} else {
		//Add defualt menu
		exists := make(map[string]bool)
		for i, id := range menuIDs {
			menuIDs[i] = strings.ToUpper(id)
			exists[menuIDs[i]] = true
		}
		for _, menu := range coreMenuIDs {
			if !exists[menu] {
				menuIDs = append(menuIDs, menu)
				exists[menu] = true
			}
		}
	}

	// ---- 2) default grants ตาม role ----
	role, _ := claims["role"].(string)
	if role == "" {
		role = "user"
	}
	preferredUsername := fmt.Sprintf("%v", claims["preferred_username"])
	//grants := buildDefaultMenuGrants(role, preferredUsername, menuIDs)

	// ---- 3) merge Permify เฉพาะ non-admin ----
	sub := fmt.Sprintf("%v", claims["sub"])
	var perms map[string]authzmod.PermissionSubjectCheckItem

	isAdmin := role == "administrator" || role == "admin" || preferredUsername == "admin"
	log.Info().Str("sub", sub).Str("role", role).Bool("isAdmin", isAdmin).Msg("Building menu grants")

	if sub != "" && !isAdmin {
		menu_perms, err := mergePermifyGrantsCRUD(ctx, "user|"+sub, menuIDs)
		if err != nil {
			log.Warn().Err(err).Str("userId", sub).Msg("⚠️ mergePermifyGrants failed, keep defaults")
		}
		perms = menu_perms
	}

	// ... ส่วนบนเหมือนเดิม (โหลด token/claims, menuIDs, buildDefaultMenuGrants, mergePermifyGrants)

	// ---- 4) meta ----
	meta := map[string]string{}
	if sv := config.CurrentSchemaVersion; sv != "" {
		meta["schema_version"] = sv
	}
	meta["source"] = "permify"
	meta["permissions_format"] = "menus_array_v1"

	log.Info().
		Str("sub", sub).
		Str("preferred_username", preferredUsername).
		Str("role", role).
		Msg("Menu grants built")

	// ---- 5) แปลง grants (map) → array (คงลำดับตาม options.menu)
	menusArr := make([]authmod.MenuGrant, 0, len(menuIDs))
	isCoreMap := make(map[string]bool)
	for _, v := range coreMenuIDs {
		isCoreMap[strings.ToUpper(v)] = true
	}

	for _, id := range menuIDs {
		upperID := strings.ToUpper(id)
		isCore := isCoreMap[upperID]
		p := perms[upperID]

		readVal := p.View
		if isCore || isAdmin {
			readVal = true
		}
		createVal := p.Create
		if isCore || isAdmin {
			createVal = true
		}

		menusArr = append(menusArr, authmod.MenuGrant{
			ID:     upperID,
			Read:   readVal,
			Create: createVal,
		})
	}
	// endregion
	log.Info().Interface("MenusArr", menusArr).Msg("Final menu grants array")

	res := &authmod.IntrospectResult{
		Active:            true,
		Sub:               subjectID,
		Username:          getStringClaim(claims, "preferred_username"),
		PreferredUsername: getStringClaim(claims, "preferred_username"),
		GivenName:         getStringClaim(claims, "given_name"),
		FamilyName:        getStringClaim(claims, "family_name"),
		Avatar:            getStringClaim(claims, "avatar"),
		Email:             getStringClaim(claims, "email"),
		Locale:            getStringClaim(claims, "locale"),
		Exp:               exp,
		Scope:             getStringClaim(claims, "scope"),
		Role:              getStringClaim(claims, "role"),
		Permissions:       authmod.PermissionsBlock{Menus: menusArr},
		ZoomLevel:         getIntClaim(claims, "zoomLevel"),
		MapLocation:       authmod.MapLocation{Lat: getFloatClaim(claims, "lat"), Lng: getFloatClaim(claims, "lng")},
	}

	log.Debug().
		Str("sub", res.Sub).
		Str("username", res.Username).
		Int64("exp", res.Exp).
		Int("permissions_count", len(res.Permissions.Menus)).
		Msg("token validated successfully")

	return res, nil
}

func isHigherPermission(newP, oldP authmod.Allow) bool {
	order := map[authmod.Allow]int{authmod.AllowRead: 1, authmod.AllowCU: 2, authmod.AllowCRUD: 3}
	return order[newP] > order[oldP]
}

func getIntClaim(claims map[string]interface{}, key string) int {
	if v, ok := claims[key].(int); ok {
		return v
	}
	return 0
}

func getFloatClaim(claims map[string]interface{}, key string) float64 {
	if v, ok := claims[key].(float64); ok {
		return v
	}
	return 0
}

func getStringClaim(claims map[string]interface{}, key string) string {
	if v, ok := claims[key].(string); ok {
		return v
	}
	return ""
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
