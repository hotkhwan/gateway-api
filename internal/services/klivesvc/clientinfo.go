// internal/services/klivesvc/clientinfo.go
package klivesvc

import (
	"strings"

	"github.com/hotkhwan/gateway-api/models/klivemod"

	"github.com/gofiber/fiber/v2"
)

// ExtractClientInfo เก็บข้อมูลฝั่ง user "ตอนยิง API" เช่น CreateStream
func ExtractClientInfo(c *fiber.Ctx) klivemod.ClientInfo {
	ip := strings.TrimSpace(c.IP())
	ua := strings.TrimSpace(c.Get("User-Agent"))
	lang := strings.TrimSpace(c.Get("Accept-Language"))

	xff := strings.TrimSpace(c.Get("X-Forwarded-For"))
	xrip := firstIPFromXFF(xff)

	chUA := strings.TrimSpace(c.Get("Sec-CH-UA"))
	chPlatform := strings.TrimSpace(c.Get("Sec-CH-UA-Platform"))
	chMobile := strings.TrimSpace(c.Get("Sec-CH-UA-Mobile"))

	browser, osName, device := parseUAQuick(ua, chUA, chPlatform, chMobile)

	raw := map[string]string{
		"user-agent":         ua,
		"accept-language":    lang,
		"x-forwarded-for":    xff,
		"xff-ip":             xrip,
		"sec-ch-ua":          chUA,
		"sec-ch-ua-platform": chPlatform,
		"sec-ch-ua-mobile":   chMobile,
		"x-real-ip":          strings.TrimSpace(c.Get("X-Real-IP")),
		"cf-connecting-ip":   strings.TrimSpace(c.Get("CF-Connecting-IP")),
		"true-client-ip":     strings.TrimSpace(c.Get("True-Client-IP")),
		"forwarded":          strings.TrimSpace(c.Get("Forwarded")),
		"origin":             strings.TrimSpace(c.Get("Origin")),
		"referer":            strings.TrimSpace(c.Get("Referer")),
	}

	ci := klivemod.ClientInfo{
		IP:         ip,
		UA:         ua,
		Lang:       lang,
		Browser:    browser,
		OS:         osName,
		Device:     device,
		RawHeaders: raw,
	}

	// ipChain (ช่วย debug)
	if ip != "" {
		ci.IPChain = append(ci.IPChain, ip)
	}
	if xrip != "" && xrip != ip {
		ci.IPChain = append(ci.IPChain, xrip)
	}
	return ci
}

func firstIPFromXFF(xff string) string {
	xff = strings.TrimSpace(xff)
	if xff == "" {
		return ""
	}
	parts := strings.Split(xff, ",")
	return strings.TrimSpace(parts[0])
}

// parseUAQuick: เบาๆ แต่พอใช้งานจริงได้
func parseUAQuick(ua, chUA, chPlatform, chMobile string) (browser, osName, device string) {
	u := strings.ToLower(ua)

	// device
	device = "Desktop"
	if strings.Contains(u, "mobile") || strings.Contains(strings.ToLower(chMobile), "?1") || strings.Contains(strings.ToLower(chMobile), "1") {
		device = "Mobile"
	}
	if strings.Contains(u, "ipad") || strings.Contains(u, "tablet") {
		device = "Tablet"
	}

	// os
	p := strings.ToLower(chPlatform)
	switch {
	case strings.Contains(p, "android") || strings.Contains(u, "android"):
		osName = "Android"
	case strings.Contains(p, "ios") || strings.Contains(u, "iphone") || strings.Contains(u, "ipad"):
		osName = "iOS"
	case strings.Contains(p, "windows") || strings.Contains(u, "windows"):
		osName = "Windows"
	case strings.Contains(p, "mac") || strings.Contains(u, "mac os x") || strings.Contains(u, "macintosh"):
		osName = "macOS"
	case strings.Contains(u, "linux"):
		osName = "Linux"
	default:
		osName = "Unknown"
	}

	// browser
	switch {
	case strings.Contains(u, "edg/"):
		browser = "Edge"
	case strings.Contains(u, "chrome/") && !strings.Contains(u, "edg/"):
		browser = "Chrome"
	case strings.Contains(u, "safari/") && !strings.Contains(u, "chrome/"):
		browser = "Safari"
	case strings.Contains(u, "firefox/"):
		browser = "Firefox"
	default:
		_ = chUA // เผื่ออนาคตใช้
		browser = "Unknown"
	}

	return
}
