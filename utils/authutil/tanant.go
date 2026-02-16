// utils/authutil/tanant.go
package authutil

import "strings"

func ExtractRealmFromIssuer(iss string) string {
	iss = strings.TrimSpace(iss)
	if iss == "" {
		return ""
	}
	// หา "/realms/<realm>"
	const key = "/realms/"
	i := strings.Index(iss, key)
	if i < 0 {
		return ""
	}
	realm := iss[i+len(key):]
	// ตัด path ต่อท้ายถ้ามี
	if j := strings.Index(realm, "/"); j >= 0 {
		realm = realm[:j]
	}
	return realm
}
