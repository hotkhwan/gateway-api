// internal/services/usrsvc/user.go
package usrsvc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"klynx/models/gmod"
	"klynx/models/usrmod"
	"klynx/utils/authutil"
	"klynx/utils/traceutil"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ListUsersResult struct {
	Details    []usrmod.KeycloakUser
	Pagination gmod.Pagination
}

// keycloak realm role object (subset)
type kcRealmRole struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

func ListUsers(ctx context.Context, authHeader string, page, perPages int, search, sortField, sortOrder string) (ListUsersResult, error) {
	ctx, end, log := traceutil.StartLite(
		ctx, "klynx/usrsvc", "users.ListUsers", "usrsvc", "ListUsers",
	)
	defer end()

	// ให้เวลาเผื่อ LDAP/External User Federation
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	keycloakURL := os.Getenv("KEYCLOAK_URL")
	realm := os.Getenv("KEYCLOAK_REALM")
	if keycloakURL == "" || realm == "" {
		return ListUsersResult{}, errors.New("KEYCLOAK_URL or KEYCLOAK_REALM is not set")
	}

	// ใช้ admin token สำหรับเรียก admin API
	token, err := authutil.GetAdminAccessToken()
	if err != nil {
		log.Error().Err(err).Msg("❌ Cannot get admin token")
		return ListUsersResult{}, err
	}

	client := &http.Client{}

	// --- 1) COUNT ---
	countU, _ := url.Parse(strings.TrimRight(keycloakURL, "/") + "/admin/realms/" + url.PathEscape(realm) + "/users/count")
	qc := countU.Query()
	if s := strings.TrimSpace(search); s != "" {
		qc.Set("search", s)
	}
	countU.RawQuery = qc.Encode()

	reqCount, _ := http.NewRequestWithContext(ctx, "GET", countU.String(), nil)
	reqCount.Header.Set("Authorization", "Bearer "+token)

	respCount, err := client.Do(reqCount)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to call Keycloak /users/count")
		return ListUsersResult{}, err
	}
	defer respCount.Body.Close()

	if respCount.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(respCount.Body)
		log.Error().Int("status", respCount.StatusCode).Msgf("❌ Count error: %s", string(b))
		return ListUsersResult{}, fmt.Errorf("keycloak count error: %s", respCount.Status)
	}

	var totalRecords int
	if err := json.NewDecoder(respCount.Body).Decode(&totalRecords); err != nil {
		return ListUsersResult{}, err
	}

	// --- 2) LIST (pagination) ---
	first := (page - 1) * perPages
	usersU, _ := url.Parse(strings.TrimRight(keycloakURL, "/") + "/admin/realms/" + url.PathEscape(realm) + "/users")
	qu := usersU.Query()
	// ต้องการ createdTimestamp => ใช้ briefRepresentation=false
	qu.Set("briefRepresentation", "false")
	qu.Set("first", fmt.Sprintf("%d", first))
	qu.Set("max", fmt.Sprintf("%d", perPages))
	if s := strings.TrimSpace(search); s != "" {
		qu.Set("search", s)
	}
	usersU.RawQuery = qu.Encode()

	reqUsers, _ := http.NewRequestWithContext(ctx, "GET", usersU.String(), nil)
	reqUsers.Header.Set("Authorization", "Bearer "+token)

	respUsers, err := client.Do(reqUsers)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to call Keycloak /users")
		return ListUsersResult{}, err
	}
	defer respUsers.Body.Close()

	if respUsers.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(respUsers.Body)
		return ListUsersResult{}, fmt.Errorf("keycloak users error: %s - %s", respUsers.Status, string(b))
	}

	var kcUsersRaw []usrmod.KeycloakUserRaw
	if err := json.NewDecoder(respUsers.Body).Decode(&kcUsersRaw); err != nil {
		return ListUsersResult{}, err
	}

	avatarBase := os.Getenv("S3_BASE_URL")
	out := make([]usrmod.KeycloakUser, 0, len(kcUsersRaw))
	for _, u := range kcUsersRaw {
		user := usrmod.KeycloakUser{
			ID:        u.ID,
			Username:  u.Username,
			FirstName: u.FirstName,
			LastName:  u.LastName,
			Email:     u.Email,
			PerPages:  u.PerPages,
			Enabled:   u.Enabled,
		}

		// createdTimestamp -> CreatedAt
		if u.CreatedTimestamp > 0 {
			tm := time.UnixMilli(u.CreatedTimestamp).UTC()
			user.CreatedAt = &tm
		}

		// attributes
		// attributes
		if u.Attributes != nil {

			if latArr, ok := u.Attributes["map_lat"]; ok && len(latArr) > 0 {
				if user.MapLocation == nil {
					user.MapLocation = &usrmod.MapLocation{}
				}
				user.MapLocation.Lat = latArr[0]
			}

			if lngArr, ok := u.Attributes["map_lng"]; ok && len(lngArr) > 0 {
				if user.MapLocation == nil {
					user.MapLocation = &usrmod.MapLocation{}
				}
				user.MapLocation.Lng = lngArr[0]
			}

			if avatars, ok := u.Attributes["avatar"]; ok && len(avatars) > 0 {
				avatar := avatars[0]
				if strings.HasPrefix(avatar, "/") && avatarBase != "" {
					user.Avatar = strings.TrimRight(avatarBase, "/") + avatar
				} else {
					user.Avatar = avatar
				}
			}

			if perArr, ok := u.Attributes["perPages"]; ok && len(perArr) > 0 {
				if n, err := strconv.Atoi(perArr[0]); err == nil {
					user.PerPages = n
				}
			}

			if locales, ok := u.Attributes["locale"]; ok && len(locales) > 0 {
				user.Locale = locales[0]
			}

			if rolesAttr, ok := u.Attributes["role"]; ok && len(rolesAttr) > 0 && user.Role == "" {
				user.Role = rolesAttr[0]
			}
			if perArr, ok := u.Attributes["zoomLevel"]; ok && len(perArr) > 0 {
				if n, err := strconv.Atoi(perArr[0]); err == nil {
					user.ZoomLevel = n
				}
			}
		}

		out = append(out, user)
	}

	// --- 3) เติม Realm Roles จริงจาก Keycloak ---
	// จำกัด concurrent workers ไม่ให้ยิง Keycloak พร้อมกันเยอะเกิน
	maxWorkers := 6
	type job struct {
		idx int
		id  string
	}
	jobs := make(chan job)
	var wg sync.WaitGroup

	realmBase := strings.TrimRight(keycloakURL, "/") + "/admin/realms/" + url.PathEscape(realm)

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			rolesURL := fmt.Sprintf("%s/users/%s/role-mappings/realm", realmBase, url.PathEscape(out[j.idx].ID))
			req, _ := http.NewRequestWithContext(ctx, "GET", rolesURL, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			resp, err := client.Do(req)
			if err != nil {
				continue // เงียบ—ยังใช้งานได้แม้ไม่มี roles
			}
			func() {
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					io.Copy(io.Discard, resp.Body)
					return
				}
				var roles []kcRealmRole
				if err := json.NewDecoder(resp.Body).Decode(&roles); err != nil {
					return
				}
				// เก็บเฉพาะชื่อ role
				names := make([]string, 0, len(roles))
				for _, r := range roles {
					if r.Name != "" {
						names = append(names, r.Name)
					}
				}
				sort.Strings(names)
				// ใส่ใน RealmRoles และอัปเดต Role (หลัก) ถ้ายังว่าง
				out[j.idx].RealmRoles = names
				if out[j.idx].Role == "" && len(names) > 0 {
					out[j.idx].Role = names[0]
				}
			}()
		}
	}

	wg.Add(maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		go worker()
	}
	for i := range out {
		jobs <- job{idx: i, id: out[i].ID}
	}
	close(jobs)
	wg.Wait()

	// --- 4) In-memory sort (Keycloak API จำกัดการ sort) ---
	sort.Slice(out, func(i, j int) bool {
		var a, b string
		switch sortField {
		case "email":
			a, b = out[i].Email, out[j].Email
		case "firstName":
			a, b = out[i].FirstName, out[j].FirstName
		case "lastName":
			a, b = out[i].LastName, out[j].LastName
		case "createdAt":
			// sort ตามเวลา (ถ้าขาดค่า จะถือว่าเก่า)
			var ai, bj int64
			if out[i].CreatedAt != nil {
				ai = out[i].CreatedAt.Unix()
			}
			if out[j].CreatedAt != nil {
				bj = out[j].CreatedAt.Unix()
			}
			if sortOrder == "asc" {
				return ai < bj
			}
			return ai > bj
		default:
			a, b = out[i].Username, out[j].Username
		}
		if sortField == "createdAt" {
			// กรณีบน handled แล้ว
			return false
		}
		if sortOrder == "asc" {
			return strings.ToLower(a) < strings.ToLower(b)
		}
		return strings.ToLower(a) > strings.ToLower(b)
	})

	totalPages := (totalRecords + perPages - 1) / perPages

	return ListUsersResult{
		Details: out,
		Pagination: gmod.Pagination{
			Page:         page,
			PerPages:     perPages,
			TotalRecords: totalRecords,
			TotalPages:   totalPages,
			SortField:    sortField,
			SortOrder:    sortOrder,
		},
	}, nil
}

func GetUsersByIds(ctx context.Context, authHeader string, userIds []string) (ListUsersResult, error) {
	ctx, end, log := traceutil.StartLite(
		ctx, "klynx/usrsvc", "users.GetUsersByIds", "usrsvc", "GetUsersByIds",
	)
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	keycloakURL := os.Getenv("KEYCLOAK_URL")
	realm := os.Getenv("KEYCLOAK_REALM")
	if keycloakURL == "" || realm == "" {
		return ListUsersResult{}, errors.New("KEYCLOAK_URL or KEYCLOAK_REALM is not set")
	}

	token, err := authutil.GetAdminAccessToken()
	if err != nil {
		log.Error().Err(err).Msg("❌ Cannot get admin token")
		return ListUsersResult{}, err
	}

	client := &http.Client{}
	realmBase := strings.TrimRight(keycloakURL, "/") + "/admin/realms/" + url.PathEscape(realm)

	avatarBase := os.Getenv("S3_BASE_URL")

	type job struct {
		idx int
		id  string
	}

	out := make([]usrmod.KeycloakUser, len(userIds))
	errs := make([]error, len(userIds))

	maxWorkers := 8
	jobs := make(chan job)
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			u := fmt.Sprintf("%s/users/%s", realmBase, url.PathEscape(j.id))

			req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := client.Do(req)
			if err != nil {
				errs[j.idx] = err
				continue
			}

			func() {
				defer resp.Body.Close()

				if resp.StatusCode != http.StatusOK {
					b, _ := io.ReadAll(resp.Body)
					errs[j.idx] = fmt.Errorf("keycloak user %s error: %s - %s", j.id, resp.Status, string(b))
					return
				}

				var raw usrmod.KeycloakUserRaw
				if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
					errs[j.idx] = err
					return
				}

				user := usrmod.KeycloakUser{
					ID:        raw.ID,
					Username:  raw.Username,
					FirstName: raw.FirstName,
					LastName:  raw.LastName,
					Email:     raw.Email,
					PerPages:  raw.PerPages,
					Enabled:   raw.Enabled,
				}

				if raw.CreatedTimestamp > 0 {
					tm := time.UnixMilli(raw.CreatedTimestamp).UTC()
					user.CreatedAt = &tm
				}

				if raw.Attributes != nil {
					if latArr, ok := raw.Attributes["map_lat"]; ok && len(latArr) > 0 {
						if user.MapLocation == nil {
							user.MapLocation = &usrmod.MapLocation{}
						}
						user.MapLocation.Lat = latArr[0]
					}
					if lngArr, ok := raw.Attributes["map_lng"]; ok && len(lngArr) > 0 {
						if user.MapLocation == nil {
							user.MapLocation = &usrmod.MapLocation{}
						}
						user.MapLocation.Lng = lngArr[0]
					}

					if avatars, ok := raw.Attributes["avatar"]; ok && len(avatars) > 0 {
						avatar := avatars[0]
						if strings.HasPrefix(avatar, "/") && avatarBase != "" {
							user.Avatar = strings.TrimRight(avatarBase, "/") + avatar
						} else {
							user.Avatar = avatar
						}
					}

					if locales, ok := raw.Attributes["locale"]; ok && len(locales) > 0 {
						user.Locale = locales[0]
					}
					if rolesAttr, ok := raw.Attributes["role"]; ok && len(rolesAttr) > 0 && user.Role == "" {
						user.Role = rolesAttr[0]
					}
				}

				out[j.idx] = user
			}()
		}
	}

	wg.Add(maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		go worker()
	}

	for i, id := range userIds {
		jobs <- job{idx: i, id: id}
	}
	close(jobs)
	wg.Wait()

	// เก็บเฉพาะที่สำเร็จ + sort ตามลำดับ input (หรือจะ sort โดย username ก็ได้)
	final := make([]usrmod.KeycloakUser, 0, len(out))
	for i := range out {
		if out[i].ID == "" {
			continue
		}
		final = append(final, out[i])
	}

	// เติม realm roles เหมือน ListUsers (ถ้าต้องการ)
	// ถ้าคุณอยากให้ batch เร็วสุด: ตัดส่วน roles ออก หรือทำ optional flag
	// ที่นี่ทำแบบเดิมเพื่อให้ data shape consistent:
	final = fillRealmRoles(ctx, client, realmBase, token, final)

	// ให้ผลลัพธ์ deterministic
	sort.SliceStable(final, func(i, j int) bool {
		return strings.ToLower(final[i].Username) < strings.ToLower(final[j].Username)
	})

	return ListUsersResult{
		Details: final,
		Pagination: gmod.Pagination{
			Page:         1,
			PerPages:     len(final),
			TotalRecords: len(final),
			TotalPages:   1,
			SortField:    "userIds",
			SortOrder:    "asc",
		},
	}, nil
}

// แยก role-fill ออกมา reuse ได้ (ยกมาจากของเดิม ปรับให้รับ slice)
// หมายเหตุ: ต้องมี kcRealmRole type เหมือนเดิมในไฟล์นี้
func fillRealmRoles(ctx context.Context, client *http.Client, realmBase, token string, users []usrmod.KeycloakUser) []usrmod.KeycloakUser {
	if len(users) == 0 {
		return users
	}

	maxWorkers := 6
	type job struct {
		idx int
		id  string
	}
	jobs := make(chan job)
	var wg sync.WaitGroup

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			rolesURL := fmt.Sprintf("%s/users/%s/role-mappings/realm", realmBase, url.PathEscape(users[j.idx].ID))
			req, _ := http.NewRequestWithContext(ctx, "GET", rolesURL, nil)
			req.Header.Set("Authorization", "Bearer "+token)

			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			func() {
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					io.Copy(io.Discard, resp.Body)
					return
				}
				var roles []kcRealmRole
				if err := json.NewDecoder(resp.Body).Decode(&roles); err != nil {
					return
				}
				names := make([]string, 0, len(roles))
				for _, r := range roles {
					if r.Name != "" {
						names = append(names, r.Name)
					}
				}
				sort.Strings(names)
				users[j.idx].RealmRoles = names
				if users[j.idx].Role == "" && len(names) > 0 {
					users[j.idx].Role = names[0]
				}
			}()
		}
	}

	wg.Add(maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		go worker()
	}

	for i := range users {
		jobs <- job{idx: i, id: users[i].ID}
	}
	close(jobs)
	wg.Wait()

	return users
}
