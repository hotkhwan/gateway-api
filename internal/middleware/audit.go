// internal/middleware/audit.go
package middleware

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/internal/repo/authzrepo"
	"github.com/hotkhwan/gateway-api/models/authzmod"
	"github.com/hotkhwan/gateway-api/models/systemmod"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v4"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

/* =========================================
 *  Config & types
 * ========================================= */

// DynamicEffectiveGetter ให้ฟังก์ชันดึงค่า EffectiveConfig แบบไดนามิก
type DynamicEffectiveGetter func(ctx context.Context) (*systemmod.EffectiveConfig, error)

// ✅ Global getter/setter สำหรับ inject จาก main.go
var effectiveGetter DynamicEffectiveGetter

func SetEffectiveGetter(fn DynamicEffectiveGetter) {
	effectiveGetter = fn
}
func EffectiveGetter() DynamicEffectiveGetter {
	return effectiveGetter
}

// AuditConfig ปรับเพิ่ม Effective เพื่ออ่านค่าปัจจุบัน (มี cache ภายนอก)
type AuditConfig struct {
	AuditRepo authzrepo.AuditLogRepo
	AuditChan chan<- *authzmod.AuditLog
	BasePath  string // optional fallback ถ้า effective ไม่มี หรือโหลดพลาด

	Effective DynamicEffectiveGetter // ถ้ามีจะใช้ดึงค่าทุก request ผ่าน cache TTL 30s
}

type auditPath struct {
	Prefix    string
	Operation string
}

// เส้นที่ต้อง audit (prefix match)
var auditPrefixes = []auditPath{
	// auth apis
	{"/auth/signin", "authSignin"},
	{"/auth/refreshToken", "authRefresh"},
	{"/auth/resetPassword", "authResetPassword"},
	{"/auth/signout", "authSignout"},
	// authz apis
	{"/authz/schema/apply", "schemaApply"},
	{"/authz/resource/grant", "resourceGrant"},
	{"/authz/resource/revoke", "resourceRevoke"},
	{"/authz/permission/check/batch", "permissionCheckBatch"},
	{"/authz/profiles", "profiles"},
	// authz permission profiles
	{"/orgs/resource/permissions", "resourcePermissionProfile"},
	{"/orgs/menu/permissions", "menuPermissionProfile"},
	// Kcontrol
	{"/kcontrol/alarms", "kcontrolAlarms"},
	{"/kcontrol", "kcontrol"},
	// third token apis
	{"/token/api/serviceAccount", "serviceAccount"},
	{"/token/api/clientCredentials", "clientCredentials"},
	// ✅ ATA control apis
	{"/ata/svms/sync", "ataSync"},
	// ✅ System
	{"/system/edge", "systemEdge"},
	{"/system/options", "systemOptions"},
	{"/system", "systemSvc"},
	// ✅ Users
	{"/users", "users"},
	// ✅ Ingest
	{"/ingest/mappingTemplates", "ingestMappingTemplates"},
	{"/ingest/management/bulk", "ingestManagementBulk"},
	{"/ingest/management", "ingestManagement"},
	{"/ingest/dlq", "ingestDlq"},
}

/* =========================================
 *  Middleware
 * ========================================= */

func init() {
	sort.SliceStable(auditPrefixes, func(i, j int) bool {
		return len(auditPrefixes[i].Prefix) > len(auditPrefixes[j].Prefix)
	})
}

func Audit(cfg AuditConfig) fiber.Handler {
	return func(c fiber.Ctx) error {
		if done, _ := c.Locals("__audit_done").(bool); done {
			return c.Next()
		}
		c.Locals("__audit_done", true)
		// audit เฉพาะ mutating
		m := c.Method()
		if m != fiber.MethodPost && m != fiber.MethodPut && m != fiber.MethodPatch && m != fiber.MethodDelete {
			return c.Next()
		}

		// ⬇️ ดึงค่าปัจจุบัน (dynamic)
		eff, _ := func() (*systemmod.EffectiveConfig, error) {
			// 1) ตัวที่ส่งมาทาง cfg (ถ้ากำหนด)
			if cfg.Effective != nil {
				return cfg.Effective(c)
			}
			// 2) global getter ที่ตั้งจาก main.go
			if g := EffectiveGetter(); g != nil {
				return g(c)
			}
			return nil, nil
		}()

		basePath := cfg.BasePath
		if eff != nil && strings.TrimSpace(eff.BasePath) != "" {
			basePath = eff.BasePath
		}
		if basePath == "" {
			basePath = "/api/v1"
		}

		fullPath := c.Path()
		short := normalizePath(fullPath, basePath)

		// match operation
		op := ""
		for _, ap := range auditPrefixes {
			if strings.HasPrefix(short, ap.Prefix) {
				op = ap.Operation
				break
			}
		}
		if op == "" {
			return c.Next()
		}

		// tracing — c implements context.Context; start a child span
		tracer := otel.Tracer("github.com/hotkhwan/gateway-api/mw/audit")
		spanCtx, span := tracer.Start(c, "Audit")
		defer span.End()
		traceId := currentTraceID(spanCtx)
		_ = logger.FromCtx(spanCtx, "audit", op)

		// payload redact
		var payload any
		if len(c.Body()) > 0 {
			_ = json.Unmarshal(c.Body(), &payload)
			payload = redact(payload)
		}

		// run next
		start := time.Now()
		err := c.Next()
		latency := time.Since(start).Milliseconds()

		atype, actorID, actorName, actorIP := resolveActor(c)
		rec := &authzmod.AuditLog{
			ActorType: atype,
			ActorId:   actorID,
			ActorName: actorName,
			ActorIp:   actorIP,

			Operation: op,
			Resource:  short,
			Method:    m,
			Path:      fullPath,
			Query:     string(c.RequestCtx().QueryArgs().QueryString()),
			Status:    c.Response().StatusCode(),
			LatencyMs: latency,
			Payload:   payload,
			TraceId:   traceId,
			CreatedAt: time.Now().UTC(),
		}

		// ==== (ออปชัน) เก็บ response ตาม policy แบบไดนามิก ====
		if eff != nil {
			capResp := strings.ToLower(strings.TrimSpace(eff.AuditCaptureResponse)) // none|errors|all
			maxBytes := eff.AuditMaxRespBytes
			onlyJSON := eff.AuditCaptureJSONOnly
			if maxBytes <= 0 {
				maxBytes = 16384
			}

			if capResp == "all" || (capResp == "errors" && rec.Status >= 400) {
				ct := string(c.Response().Header.ContentType())
				if !onlyJSON || strings.Contains(strings.ToLower(ct), "application/json") {
					body := c.Response().Body()
					if len(body) > maxBytes {
						body = body[:maxBytes]
					}
					// พยายาม parse JSON หากเป็น JSON
					if strings.Contains(strings.ToLower(ct), "application/json") {
						var out any
						if json.Unmarshal(body, &out) == nil {
							rec.Response = out
						} else {
							rec.Response = string(body)
						}
					} else {
						rec.Response = string(body)
					}
				}
			}
		}

		// persist/forward
		go persistAudit(cfg, rec)
		return err
	}
}

/* =========================================
 *  Helpers
 * ========================================= */

func normalizePath(fullPath, base string) string {
	short := fullPath
	if bp := strings.TrimRight(base, "/"); bp != "" && bp != "/" {
		short = strings.TrimPrefix(short, bp)
	}
	if !strings.HasPrefix(short, "/") {
		short = "/" + short
	}
	return short
}

func currentTraceID(ctx context.Context) string {
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		return sc.TraceID().String()
	}
	return ""
}

func persistAudit(cfg AuditConfig, a *authzmod.AuditLog) {
	if cfg.AuditRepo != nil {
		if e := cfg.AuditRepo.Create(context.Background(), a); e != nil {
			log.Error().Err(e).Msg("failed to write audit log")
		}
	}
	if cfg.AuditChan != nil {
		select {
		case cfg.AuditChan <- a:
		default:
		}
	}
}

// ----- Actor helpers -----
// internal/middleware/audit.go
func resolveActor(c fiber.Ctx) (atype, id, name, ip string) {
	ip = realIP(c)
	atype = "user" // default

	// 1) จาก Locals (AuthBearer ตั้งไว้)
	if u := c.Locals("user"); u != nil {
		if m, ok := u.(map[string]any); ok {
			pu := str(m["preferred_username"])
			cid := str(m["client_id"])
			sub := str(m["sub"])

			// service account ?
			if strings.HasPrefix(pu, "service-account-") || cid != "" {
				atype = "service"
				if cid == "" && strings.HasPrefix(pu, "service-account-") {
					cid = strings.TrimPrefix(pu, "service-account-")
				}
				id = cid
				// id = "client:" + cid
				name = firstNotEmpty(pu, cid)
				if id != "" {
					return
				}
			}

			// user
			id = firstNotEmpty(sub, str(m["preferred_username"]))
			name = firstNotEmpty(str(m["preferred_username"]), str(m["name"]))
			if id != "" || name != "" {
				return
			}
		}
	}

	// 2) Parse JWT แบบ unverified (รองรับ public เส้น)
	auth := c.Get(fiber.HeaderAuthorization)
	if strings.HasPrefix(auth, "Bearer ") {
		raw := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
		claims := jwt.MapClaims{}
		_, _, _ = new(jwt.Parser).ParseUnverified(raw, claims)

		pu := str(claims["preferred_username"])
		cid := str(claims["client_id"])
		sub := str(claims["sub"])

		if strings.HasPrefix(pu, "service-account-") || cid != "" {
			atype = "service"
			if cid == "" && strings.HasPrefix(pu, "service-account-") {
				cid = strings.TrimPrefix(pu, "service-account-")
			}
			// id = "client:" + cid
			id = cid
			name = firstNotEmpty(pu, cid)
			if id != "" {
				return
			}
		}

		atype = "user"
		id = firstNotEmpty(sub, pu)
		name = firstNotEmpty(pu, str(claims["name"]))
		if id != "" || name != "" {
			return
		}
	}

	// 3) header fallback
	if x := c.Get("X-User-ID"); x != "" {
		atype = "user"
		id = x
		name = "anonymous"
		return
	}

	// 4) anonymous
	atype = "user"
	id = "unknown"
	name = "anonymous"
	return
}

func realIP(c fiber.Ctx) string {
	if ips := c.IPs(); len(ips) > 0 {
		return ips[0]
	}
	return c.IP()
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func firstNotEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

/* =========================================
 *  Redact (recursive)
 * ========================================= */

func redact(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, vv := range val {
			lk := strings.ToLower(k)
			if lk == "authorization" || strings.Contains(lk, "token") ||
				strings.Contains(lk, "password") ||
				strings.Contains(lk, "secret") ||
				strings.Contains(lk, "secret") {
				// (strings.Contains(lk, "api") &&
				// strings.Contains(lk, "key")) {
				out[k] = "***"
				continue
			}
			out[k] = redact(vv)
		}
		return out
	case []any:
		for i := range val {
			val[i] = redact(val[i])
		}
		return val
	default:
		return v
	}
}
