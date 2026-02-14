package klivesvc

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"klynx/internal/gateways/mediagw"
	"klynx/internal/logger"
)

// -------- lazy init mediagw client (อยู่ใน klivesvc) --------
var (
	zlmClient *mediagw.Client
	zlmOnce   sync.Once
)

func getZLMClient() *mediagw.Client {
	zlmOnce.Do(func() {
		c := mediagw.NewFromEnv("ZKT")
		if c != nil && c.BaseURL != "" && strings.HasSuffix(c.BaseURL, "/media/index/api") {
			// normalize ให้เป็น /index/api
			c.BaseURL = strings.TrimSuffix(c.BaseURL, "/media/index/api") + "/index/api"
		}
		zlmClient = c
	})
	return zlmClient
}

// KillZLMByPeerIP:
// 1) kick_sessions?peer_ip=... (ไม่กรอง local_port เพื่อให้ WebRTC/HTTP โดนด้วย)
// 2) ถ้า hit=0 หรือยังเหลือ -> fallback getAllSession + kick_session ทีละ id
func KillZLMByPeerIP(peerIP string) (int, error) {
	peerIP = strings.TrimSpace(peerIP)
	if peerIP == "" {
		return 0, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()

	log := logger.FromCtx(ctx, "klivesvc", "KillZLMByPeerIP")

	client := getZLMClient()
	if client == nil || !client.Configured() {
		return 0, fmt.Errorf("mediagw not configured")
	}

	// ✅ ไม่กรอง local_port (สำคัญมากสำหรับ WebRTC ที่ local_port ไม่ใช่ 554)
	const noPortFilter = 0

	// 1) primary: kick_sessions by peer_ip
	// out, status, err := client.KickSessions(ctx, peerIP, noPortFilter)
	// if err == nil && status == 200 {
	// 	hit := getIntAny(out["count_hit"])
	// 	log.Info().
	// 		Str("peer_ip", peerIP).
	// 		Int("local_port", noPortFilter).
	// 		Int("count_hit", hit).
	// 		Msg("🧨 kick_sessions executed")

	// 	// ถ้า hit>0 ส่วนใหญ่จบแล้ว
	// 	if hit > 0 {
	// 		return hit, nil
	// 	}
	// }
	out, status, err := client.KickSessions(ctx, peerIP, noPortFilter)
	if err == nil && status == 200 {
		hit := getIntAny(out["count_hit"])
		log.Info().
			Str("peer_ip", peerIP).
			Int("count_hit", hit).
			Msg("🧨 kick_sessions executed")
		time.Sleep(120 * time.Millisecond)
	} else {
		log.Warn().Err(err).
			Int("status", status).
			Str("peer_ip", peerIP).
			Msg("kick_sessions failed")
	}

	// 2) fallback: kick_session ทีละตัว (กันเคส kick_sessions ไม่ kill webRtc บางเวอร์ชัน/บาง type)
	sess, st2, err2 := client.GetAllSession(ctx)
	if err2 != nil || st2 != 200 {
		return 0, fmt.Errorf("getAllSession failed status=%d err=%v", st2, err2)
	}

	data, _ := sess["data"].([]any)
	if len(data) == 0 {
		return 0, nil
	}

	hit2 := 0
	for _, it := range data {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		ip := strings.TrimSpace(getStrAny(m["peer_ip"]))
		if ip != peerIP {
			continue
		}

		// ถ้าจะยิงเฉพาะ WebRTC เท่านั้น เปิดบรรทัดนี้:
		// if strings.TrimSpace(getStrAny(m["typeid"])) != "mediakit::WebRtcSession" { continue }

		id := strings.TrimSpace(getStrAny(m["id"]))
		if id == "" {
			id = strings.TrimSpace(getStrAny(m["identifier"]))
		}
		if id == "" {
			continue
		}

		o, st3, e3 := client.KickSession(ctx, id)
		if e3 != nil || st3 != 200 {
			log.Warn().Err(e3).
				Int("status", st3).
				Str("peer_ip", peerIP).
				Str("id", id).
				Interface("out", o).
				Msg("kick_session failed")
			continue
		}
		hit2++
	}

	log.Info().
		Str("peer_ip", peerIP).
		Int("count_hit", hit2).
		Msg("🧨 kick_session fallback executed")

	return hit2, nil
}

// ------- helpers -------

func getStrAny(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func getIntAny(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case float64:
		return int(t)
	case float32:
		return int(t)
	default:
		return 0
	}
}
