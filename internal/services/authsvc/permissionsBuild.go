// internal/services/authsvc/permissionsBuild.go
package authsvc

import (
	"context"
	"fmt"
	"strings"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/models/authmod"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"

	"github.com/rs/zerolog/log"
)

func buildMenusAndPermissions(
	ctx context.Context,
	claims map[string]any,
	role string,
	preferredUsername string,
) (menuIDs []string, coreMenuIDs []string, perms map[string]authzmod.PermissionSubjectCheckItem, meta map[string]string, err error) {
	ctx, end, _ := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/authsvc", "permissions.build", "authsvc", "buildMenusAndPermissions")
	defer end()

	// 1) menu options
	menuIDs, err = loadMenuIDs(ctx)
	coreMenuIDs = []string{"HOME", "DASHBOARD", "MAP", "VIDEO_WALL"}

	if err != nil || len(menuIDs) == 0 {
		log.Warn().Err(err).Msg("⚠️ Failed to load menu options, using hard-coded fallback")
		menuIDs = []string{"home", "dashboard", "map", "videowall"}
	} else {
		exists := make(map[string]bool)
		for i, id := range menuIDs {
			menuIDs[i] = strings.ToUpper(id)
			exists[menuIDs[i]] = true
		}
		for _, m := range coreMenuIDs {
			if !exists[m] {
				menuIDs = append(menuIDs, m)
				exists[m] = true
			}
		}
	}

	// 2) permify merge (non-admin)
	sub := fmt.Sprintf("%v", claims["sub"])
	perms = map[string]authzmod.PermissionSubjectCheckItem{}

	if sub != "" && role != "administrator" && role != "admin" && preferredUsername != "admin" {
		menuPerms, e := mergePermifyGrantsCRUD(ctx, "user|"+sub, menuIDs)
		if e != nil {
			log.Warn().Err(e).Str("userId", sub).Msg("⚠️ mergePermifyGrants failed, keep defaults")
		} else {
			perms = menuPerms
		}
	}

	meta = map[string]string{
		"source":             "permify",
		"permissions_format": "menus_array_v1",
	}
	if sv := config.CurrentSchemaVersion; sv != "" {
		meta["schema_version"] = sv
	}

	return
}

func buildMenusArray(
	menuIDs []string,
	coreMenuIDs []string,
	perms map[string]authzmod.PermissionSubjectCheckItem,
) []authmod.MenuGrant {
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
		if isCore {
			readVal = true
		}

		createVal := p.Create
		menusArr = append(menusArr, authmod.MenuGrant{
			ID:     upperID,
			Read:   readVal,
			Create: createVal,
		})
	}

	return menusArr
}
