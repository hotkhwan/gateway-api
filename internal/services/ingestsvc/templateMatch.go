// internal/services/ingestsvc/template_match.go
package ingestsvc

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hotkhwan/gateway-api/internal/repo/cachetemplate"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/hotkhwan/gateway-api/internal/services/templatematcher"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/rs/zerolog"
)

// TemplateMatcher resolves a mapping template from a fingerprint (eventType + keyHash)
// and applies its FieldMappings to a raw event body.
//
// Hot path: Redis cache first → MongoDB on miss.
type TemplateMatcher struct {
	repo *ingestrepo.MappingTemplateRepo
	log  zerolog.Logger
}

// NewTemplateMatcher creates a TemplateMatcher.
func NewTemplateMatcher(repo *ingestrepo.MappingTemplateRepo, log zerolog.Logger) *TemplateMatcher {
	if repo == nil {
		panic("TemplateMatcher: repo is required")
	}
	return &TemplateMatcher{repo: repo, log: log}
}

// Match looks up a mapping template for an incoming event.
//
// deviceType is the raw body's "deviceType" field (may be empty).
// Matching is flexible — see FindByMatch for criteria.
//
// Returns:
//   - (template, true, nil)  — template found
//   - (nil, false, nil)      — no template matches (not an error, fall through to pending)
//   - (nil, false, err)      — transient error (caller should fall through to pending)
func (m *TemplateMatcher) Match(ctx context.Context, orgId, eventType, keyHash, deviceType, deviceId string) (*ingestmod.MappingTemplate, bool, error) {
	// 1) Redis cache check
	if cached, found := cachetemplate.GetTemplateId(ctx, orgId, eventType, keyHash); found {
		if cachetemplate.IsNoMatch(cached) {
			m.log.Debug().
				Str("orgId", orgId).
				Str("eventType", eventType).
				Str("keyHash", keyHash).
				Msg("[TemplateMatcher] fingerprint: no-match sentinel (cache)")
			return nil, false, nil
		}
		// Cache hit: resolve full template from Mongo
		tmpl, err := m.repo.FindById(ctx, orgId, cached)
		if err != nil {
			// Template deleted after cache entry was written — treat as no-match
			m.log.Warn().
				Str("orgId", orgId).
				Str("templateId", cached).
				Err(err).
				Msg("[TemplateMatcher] cached templateId not found in Mongo, evicting")
			_ = cachetemplate.SetNoMatch(ctx, orgId, eventType, keyHash)
			return nil, false, nil
		}
		m.log.Debug().
			Str("orgId", orgId).
			Str("templateId", tmpl.TemplateId).
			Str("keyHash", keyHash).
			Bool("cacheHit", true).
			Msg("[TemplateMatcher] fingerprint cache hit")
		return tmpl, true, nil
	}

	// 2) MongoDB lookup (flexible: empty template fields act as wildcards)
	tmpl, err := m.repo.FindByMatch(ctx, orgId, eventType, keyHash, deviceType, deviceId)
	if err != nil {
		if err == ingestrepo.ErrTemplateNotFound {
			_ = cachetemplate.SetNoMatch(ctx, orgId, eventType, keyHash)
			m.log.Debug().
				Str("orgId", orgId).
				Str("eventType", eventType).
				Str("keyHash", keyHash).
				Str("deviceType", deviceType).
				Msg("[TemplateMatcher] no template found, caching sentinel")
			return nil, false, nil
		}
		m.log.Error().Err(err).
			Str("orgId", orgId).
			Str("eventType", eventType).
			Str("keyHash", keyHash).
			Msg("[TemplateMatcher] Mongo lookup error")
		return nil, false, err
	}

	// 3) Cache templateId for future hits
	_ = cachetemplate.SetTemplateId(ctx, orgId, eventType, keyHash, tmpl.TemplateId)

	m.log.Info().
		Str("orgId", orgId).
		Str("templateId", tmpl.TemplateId).
		Str("templateName", tmpl.Name).
		Str("eventType", eventType).
		Str("keyHash", keyHash).
		Str("deviceType", deviceType).
		Bool("cacheHit", false).
		Msg("[TemplateMatcher] template matched via fingerprint")

	return tmpl, true, nil
}

// MatchByFamily performs V2 template matching by sourceFamily.
// Loads all templates for orgId + sourceFamily, evaluates matchAll/matchAny against matchBag.
// Returns highest priority match.
//
// matchBag is an enriched map produced by ingestsvc.buildMatchBag — it contains:
//   - Raw payload fields at top level (e.g. channelId, deviceId, eventAttribute.*) for backward compat
//   - Canonical fields under "source.*" (e.g. source.deviceId, source.deviceType, source.sn, source.zone)
//
// This lets users author match conditions against canonical concepts they see in the UI
// (source.deviceId == "51") instead of vendor-specific raw keys (channelId == 51).
func (m *TemplateMatcher) MatchByFamily(ctx context.Context, tenantId, orgId, sourceFamily, fingerprint string, matchBag map[string]any) (*ingestmod.MappingTemplate, bool, error) {
	// 1) Redis V2 cache check
	if cached, found := cachetemplate.GetMatchV2(ctx, tenantId, orgId, sourceFamily, fingerprint); found {
		if cached == nil {
			// confirmed no-match sentinel
			return nil, false, nil
		}
		tmpl, err := m.repo.FindById(ctx, orgId, cached.TemplateId)
		if err != nil {
			cachetemplate.SetNoMatchV2(ctx, tenantId, orgId, sourceFamily, fingerprint)
			return nil, false, nil
		}
		return tmpl, true, nil
	}

	// 2) Load all templates for this org + sourceFamily (sorted by priority desc)
	templates, err := m.repo.FindBySourceFamily(ctx, orgId, sourceFamily)
	if err != nil {
		m.log.Error().Err(err).
			Str("orgId", orgId).
			Str("sourceFamily", sourceFamily).
			Msg("[TemplateMatcherV2] failed to load templates")
		return nil, false, err
	}
	if len(templates) == 0 {
		cachetemplate.SetNoMatchV2(ctx, tenantId, orgId, sourceFamily, fingerprint)
		return nil, false, nil
	}

	// 3) Evaluate matchAll/matchAny for each template (already sorted by priority)
	for _, tmpl := range templates {
		if templatematcher.Evaluate(tmpl.MatchAll, tmpl.MatchAny, matchBag) {
			cachetemplate.SetMatchV2(ctx, tenantId, orgId, &cachetemplate.MatchV2CacheValue{
				TemplateId:     tmpl.TemplateId,
				FinalEventType: tmpl.FinalEventType,
				SourceFamily:   sourceFamily,
				Fingerprint:    fingerprint,
			})
			m.log.Info().
				Str("orgId", orgId).
				Str("sourceFamily", sourceFamily).
				Str("templateId", tmpl.TemplateId).
				Str("finalEventType", tmpl.FinalEventType).
				Int("priority", tmpl.Priority).
				Msg("[TemplateMatcherV2] template matched")
			return tmpl, true, nil
		}
	}

	// No match
	cachetemplate.SetNoMatchV2(ctx, tenantId, orgId, sourceFamily, fingerprint)
	m.log.Debug().
		Str("orgId", orgId).
		Str("sourceFamily", sourceFamily).
		Str("fingerprint", fingerprint).
		Msg("[TemplateMatcherV2] no template matched")
	return nil, false, nil
}

// ApplyMappings applies a template's FieldMappings to rawBody.
//
// Returns:
//   - mapped        — target fields resolved from rawBody
//   - missingFields — TargetPaths of required mappings whose SourcePath was absent
func (m *TemplateMatcher) ApplyMappings(rawBody map[string]any, mappings []ingestmod.FieldMapping) (mapped map[string]any, missingFields []string) {
	mapped = make(map[string]any, len(mappings))
	for _, fm := range mappings {
		val, found := getNestedValue(rawBody, fm.SourcePath)
		if !found {
			if fm.Required {
				missingFields = append(missingFields, fm.TargetPath)
			}
			continue
		}
		transformed := applyTransform(val, fm.Transform)
		setNestedValue(mapped, fm.TargetPath, transformed)

		// Generate <targetPath>_label when ValueCodes are defined.
		if len(fm.ValueCodes) > 0 {
			key := fmt.Sprintf("%v", transformed)
			if label, ok := fm.ValueCodes[key]; ok {
				setNestedValue(mapped, fm.TargetPath+"_label", label)
			}
		}
	}
	return mapped, missingFields
}

// ============================================================
// path helpers
// ============================================================

// getNestedValue resolves a dotted path (e.g. "gps.lat") inside obj.
//
// A numeric segment indexes into a JSON array ([]any) — so vendor payloads that
// nest fields under an array (e.g. Dahua "Events.0.Data.TrafficCar.VehicleColor")
// are mappable. Returns (nil, false) on any missing key, out-of-range index, or
// type mismatch.
func getNestedValue(obj map[string]any, path string) (any, bool) {
	return resolvePath(obj, path)
}

// resolvePath walks cur by the dotted path, descending through both maps
// (string keys) and slices (numeric indices).
func resolvePath(cur any, path string) (any, bool) {
	if path == "" {
		return cur, true
	}
	head, rest, _ := strings.Cut(path, ".")

	switch node := cur.(type) {
	case map[string]any:
		v, ok := node[head]
		if !ok {
			return nil, false
		}
		return resolvePath(v, rest)
	case []any:
		idx, err := strconv.Atoi(head)
		if err != nil || idx < 0 || idx >= len(node) {
			return nil, false
		}
		return resolvePath(node[idx], rest)
	default:
		return nil, false
	}
}

// setNestedValue sets a dotted path inside obj, creating intermediate maps as needed.
func setNestedValue(obj map[string]any, path string, val any) {
	parts := strings.SplitN(path, ".", 2)
	if len(parts) == 1 {
		obj[parts[0]] = val
		return
	}
	child, ok := obj[parts[0]].(map[string]any)
	if !ok {
		child = make(map[string]any)
		obj[parts[0]] = child
	}
	setNestedValue(child, parts[1], val)
}

// ============================================================
// transform helpers
// ============================================================

// applyTransform coerces val to the requested type.
// Supported: "string", "float64", "int", "bool", "timestamp".
// Unknown / empty transform returns the value unchanged.
func applyTransform(val any, transform string) any {
	switch strings.ToLower(strings.TrimSpace(transform)) {
	case "string":
		return fmt.Sprintf("%v", val)

	case "float64", "float":
		switch v := val.(type) {
		case float64:
			return v
		case float32:
			return float64(v)
		case string:
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				return f
			}
		case int:
			return float64(v)
		case int32:
			return float64(v)
		case int64:
			return float64(v)
		}

	case "int":
		switch v := val.(type) {
		case int:
			return v
		case int32:
			return int(v)
		case int64:
			return int(v)
		case float64:
			return int(v)
		case string:
			if i, err := strconv.Atoi(v); err == nil {
				return i
			}
		}

	case "bool":
		switch v := val.(type) {
		case bool:
			return v
		case string:
			b, _ := strconv.ParseBool(v)
			return b
		case float64:
			return v != 0
		}

	case "timestamp":
		switch v := val.(type) {
		case string:
			for _, layout := range []string{
				time.RFC3339Nano,
				time.RFC3339,
				"2006-01-02T15:04:05",
				"2006-01-02 15:04:05",
			} {
				if t, err := time.Parse(layout, v); err == nil {
					return t.UTC()
				}
			}
		case float64:
			return time.Unix(int64(v), 0).UTC()
		case int64:
			return time.Unix(v, 0).UTC()
		}
	}

	return val
}
