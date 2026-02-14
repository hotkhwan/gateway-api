// internal/services/authsvc/signin.go
package authsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"klynx/config"
	"klynx/internal/services/authzsvc"
	"klynx/models/authmod"
	"klynx/models/authzmod"
	"klynx/utils/authutil"
	"klynx/utils/traceutil"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func Authenticate(ctx context.Context, req authmod.SigninRequest) (authmod.SigninResponse, error) {
	ctx, end, log := traceutil.StartLite(ctx, "klynx/authsvc", "authenticate.singin", "authsvc", "Authenticate")
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	log.Debug().Str("username", req.Username).Str("hwId", req.HwID).Msg("🔐 Starting authentication")

	// ---- Keycloak ----
	form := url.Values{}
	form.Add("client_id", os.Getenv("KEYCLOAK_CLIENT_ID"))
	form.Add("client_secret", os.Getenv("KEYCLOAK_CLIENT_SECRET"))
	form.Add("grant_type", "password")
	form.Add("scope", "openid")
	form.Add("username", req.Username)
	form.Add("password", req.Password)

	keycloakURL := fmt.Sprintf("%s/realms/%s/protocol/openid-connect/token",
		os.Getenv("KEYCLOAK_URL"), os.Getenv("KEYCLOAK_REALM"))

	resp, err := http.PostForm(keycloakURL, form)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to post to Keycloak")
		return authmod.SigninResponse{}, errors.New("unauthorized")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		log.Warn().Int("status", resp.StatusCode).Msg("⚠️ Keycloak returned non-200")
		return authmod.SigninResponse{}, errors.New("unauthorized")
	}

	var tokenData map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&tokenData); err != nil {
		log.Error().Err(err).Msg("❌ Failed to decode token response")
		return authmod.SigninResponse{}, err
	}

	claims, err := authutil.DecodeJWT(tokenData["access_token"].(string))
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to decode access token")
		return authmod.SigninResponse{}, err
	}

	// ---- 1) โหลดเมนูจาก Mongo ----
	menuIDs, err := loadMenuIDs(ctx)
	coreMenuIDs := []string{"HOME", "DASHBOARD", "MAP", "VIDEO_WALL"}
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
	grants := buildDefaultMenuGrants(role, preferredUsername, menuIDs)
	log.Info().Interface("grants", grants).Msg("grants")
	// ---- 3) merge Permify เฉพาะ non-admin ----

	sub := fmt.Sprintf("%v", claims["sub"])
	var perms map[string]authzmod.PermissionSubjectCheckItem
	if sub != "" && role != "administrator" && role != "admin" && preferredUsername != "admin" {
		menu_perms, err := mergePermifyGrantsCRUD(ctx, "user|"+sub, menuIDs)
		if err != nil {
			log.Warn().Err(err).Str("userId", sub).Msg("⚠️ mergePermifyGrants failed, keep defaults")
		}
		perms = menu_perms
	}
	log.Info().Interface("perms", perms).Msg("perms")

	// ---- 4) meta ----
	meta := map[string]string{}
	if sv := config.CurrentSchemaVersion; sv != "" {
		meta["schema_version"] = sv
	}
	meta["source"] = "permify"
	meta["permissions_format"] = "menus_array_v1"

	// ---- 5) แปลง grants (map) → array (คงลำดับตาม options.menu)

	menusArr := make([]authmod.MenuGrant, 0, len(menuIDs))
	// 1. เตรียม Map สำหรับเช็ค Core Menu
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
	response := authmod.SigninResponse{
		Sub:      sub,
		Username: fmt.Sprintf("%v", claims["preferred_username"]),
		Name:     fmt.Sprintf("%v", claims["name"]),
		Email:    fmt.Sprintf("%v", claims["email"]),
		Role:     role,
		Permissions: authmod.PermissionsBlock{
			Menus: menusArr,
			// MenusByID: grants, // ← ถ้าต้องการช่องช่วย migration ชั่วคราว
		},
		Meta:         meta,
		Locale:       fmt.Sprintf("%v", claims["locale"]),
		Plant:        claims["plant"],
		AccessToken:  tokenData["access_token"].(string),
		RefreshToken: tokenData["refresh_token"].(string),
	}

	// ---- 6) HwID / Station ----
	if req.HwID != "" {
		if err := fillStationInfo(ctx, req.HwID, &response); err != nil {
			return authmod.SigninResponse{}, err
		}
	}

	log.Debug().Str("username", req.Username).Msg("✅ Authentication successful")
	return response, nil
}

func loadMenuIDs(ctx context.Context) ([]string, error) {
	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	coll := db.Collection("options")

	var doc authmod.MenuListDoc
	if err := coll.FindOne(ctx, bson.M{"_id": "list.klynx"}).Decode(&doc); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(doc.Menu))
	for _, m := range doc.Menu {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	return ids, nil
}

// admin: CRUD true ทั้งหมด; user/default: read เฉพาะ home/dashboard/map/videowall
func buildDefaultMenuGrants(role, username string, menuIDs []string) map[string]authmod.CRUDGrant {
	grants := make(map[string]authmod.CRUDGrant, len(menuIDs))

	// เงื่อนไข full CRUD:
	// - role เป็น administrator/admin
	// - หรือ username == "admin" (user bootstrap แรกของระบบ)
	if role == "administrator" || role == "admin" || username == "admin" {
		for _, id := range menuIDs {
			grants[id] = authmod.CRUDGrant{Read: true, Create: true, Update: true, Delete: true}
		}
		return grants
	}

	// default read-only: home, dashboard, map, videowall
	baseRead := map[string]struct{}{
		"home":      {},
		"dashboard": {},
		"map":       {},
		"videowall": {},
		// ถ้าจะให้ "biDash" โผล่มาด้วย ก็แค่เพิ่ม "biDash": {} ตรงนี้
	}

	for _, id := range menuIDs {
		_, ok := baseRead[id]
		grants[id] = authmod.CRUDGrant{Read: ok}
	}
	return grants
}

// mergePermifyGrants ดึง permission จาก Permify โดยใช้ subject = user:<sub>
func mergePermifyGrants(
	ctx context.Context,
	userID string,
	menuIDs []string,
	grants map[string]authmod.CRUDGrant,
) error {
	resourceMap := map[string]struct {
		Type string
		Acts []string
	}{}

	for _, id := range menuIDs {
		resourceMap[id] = struct {
			Type string
			Acts []string
		}{
			Type: "resource",
			Acts: []string{"r"}, // r → map เป็น "view" ใน GetUserGlobalPermissions
		}
	}

	perms, err := authzsvc.GetUserGlobalPermissions(ctx, userID, resourceMap)
	log.Info().Interface("perms", perms).Msg("✅ GetUserGlobalPermissions")

	if err != nil {
		log.Info().Err(err).Str("userID", userID).Msg("❌ GetUserGlobalPermissions failed")
		return err
	}

	// perms: key = resourceId ("kcontrol", "kwatch", ...)
	for resID, acts := range perms {
		menuID := stripMenuPrefix(resID) // ถ้าไม่มี prefix "menu:" จะคืนค่าเดิม
		g := grants[menuID]
		for _, a := range acts {
			if a == "r" || a == "view" {
				g.Read = true
			}
		}
		grants[menuID] = g
	}
	return nil
}

func mergePermifyGrantsCRUD(ctx context.Context, subject string, menuIDs []string) (map[string]authzmod.PermissionSubjectCheckItem, error) {

	perms, err := GetPermissionsParallel(ctx, menuIDs, subject)

	if err != nil {
		log.Info().Err(err).Str("userID", strings.Split(subject, "|")[1]).Msg("❌ mergePermifyGrantsCRUD failed")
		return nil, err
	}

	return perms, nil
}

func fillStationInfo(_ context.Context, hwID string, resp *authmod.SigninResponse) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db := config.MongoClient.Database(os.Getenv("MONGO_DB"))
	stations := db.Collection("stations")

	var result map[string]interface{}
	err := stations.FindOne(ctx, bson.M{
		"hwId":   hwID,
		"state":  "create",
		"verify": true,
	}).Decode(&result)

	if err == nil {
		resp.StationID = fmt.Sprintf("%v", result["_id"])
		resp.StationName = fmt.Sprintf("%v", result["name"])
		resp.StationHwId = fmt.Sprintf("%v", result["hwId"])
		resp.StationType = fmt.Sprintf("%v", result["stationType"])
		resp.StationAddress = fmt.Sprintf("%v", result["ipAddress"])
		resp.StationCamera = fmt.Sprintf("%v", result["cameraURL"])
		return nil
	}
	if errors.Is(err, mongo.ErrNoDocuments) {
		return errors.New("invalid username or password")
	}
	return err
}

// menuIDToResourceID แปลง internal menuID → Permify resourceId (with prefix)
func menuIDToResourceID(menuID string) string {
	// ถ้าอยาก normalize เป็น lowercase ใน Permify ให้ใช้ strings.ToLower(menuID)
	// return "menu:" + strings.ToLower(menuID)

	// ถ้าจะรักษา camelCase ตามที่ team ใช้ ก็ใช้แบบนี้:
	return "menu:" + menuID
}

// stripMenuPrefix ตัด "menu:" ออก เหลือ menuID ปกติ
func stripMenuPrefix(resourceID string) string {
	const prefix = "menu:"
	if strings.HasPrefix(resourceID, prefix) {
		return resourceID[len(prefix):]
	}
	return resourceID
}

//string subject=> subjectType, subjectID

func GetPermissionsParallel(ctx context.Context, resourceIDs []string, subject string) (map[string]authzmod.PermissionSubjectCheckItem, error) {

	resultsMap := make(map[string]authzmod.PermissionSubjectCheckItem)
	var mu sync.Mutex
	var wg sync.WaitGroup

	semaphore := make(chan struct{}, 10)

	for _, id := range resourceIDs {
		wg.Add(1)
		go func(resourceID string) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			permReq := authzmod.PermissionSubjectCheckRequest{
				Entity: authzmod.PermissionEntity{
					Type: "resource",
					ID:   resourceID,
				},
				Subject: authzmod.PermissionSubject{
					Type: "user",
					ID:   "user:" + strings.Split(subject, "|")[1],
				},
			}

			log.Info().Interface("permReq", permReq).Msg("🔄 Starting permission check")
			res, err := authzsvc.PermissionSubjectCheck(ctx, permReq)
			if err != nil {
				log.Error().Err(err).Interface("permReq", permReq).Msg("❌ Permission check failed")
				return
			}

			if len(res) > 0 {
				mu.Lock()
				resultsMap[resourceID] = res[0]
				mu.Unlock()
			}
		}(id)
	}

	wg.Wait()

	return resultsMap, nil
}

// "sub": "user",
//         "username": "user",
//         "name": "User klynx",
//         "email": "user@klynx.co",
//         "role": "user",
//         "permissions": {

// 			permReq={"entity":{"id":"KWATCH","type":"resource"},"metadata":{"depth":0},"permission":null,"subject":{"id":"useruser","type":"user"}}

// admin,administrator => ไม่ต้อง check permission
// นอกนั้น check permission ทั้งหมด

// //Defaulr menu
// //user, management => view : true, create: false
// "home":      {}, // ตัวเดียวกับที่มี ในปัจจุบันใน signin
// "dashboard": {}, // ตัวเดียวกับที่มี ในปัจจุบันใน signin
// "map":       {},
// "videowall": {},

// return menu permission ทั้งหมด แล้วกำหนดสิทธิ์เอา

// menus": [
//                 {
//                     "id": "HOME",
//                     "read": false,
//                     "create": false
//                 },
//                 {
//                     "id": "DASHBOARD",
//                     "read": false,
//                     "create": false
//                 },

// 				{
//     "detail": {
//         "sub": "",
//         "username": "admin",
//         "name": "Admin klynx",
//         "email": "administrator@klynx.c
