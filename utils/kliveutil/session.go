// utils/kliveutil/session.go
package kliveutil

import (
	"net/url"
	"strings"
)

// ✅ สำหรับ hooks เท่านั้น: ไม่ fallback เด็ดขาด
func ParseSessionFromParamsOnly(params string) string {
	params = strings.TrimSpace(params)
	if params == "" {
		return ""
	}
	uq, err := url.ParseQuery(params)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(uq.Get("session"))
}

// (ของเดิม) ใช้ที่อื่นได้ แต่ห้ามใช้ใน hook
func ParseSessionFromParams(params string, fallback string) string {
	if params == "" {
		return strings.TrimSpace(fallback)
	}
	if uq, err := url.ParseQuery(params); err == nil {
		if s := strings.TrimSpace(uq.Get("session")); s != "" {
			return s
		}
	}
	return strings.TrimSpace(fallback)
}
