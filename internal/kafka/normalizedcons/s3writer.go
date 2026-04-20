// internal/kafka/normalizedcons/s3writer.go
package normalizedcons

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/repo/stos3minio"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/rs/zerolog"
)

// binaryFieldNames are payload keys that may contain a single binary (base64-encoded) string.
var binaryFieldNames = map[string]string{
	"image":     "image/jpeg",
	"photo":     "image/jpeg",
	"frame":     "image/jpeg",
	"snapshot":  "image/jpeg",
	"thumbnail": "image/jpeg",
	"video":     "video/mp4",
	"clip":      "video/mp4",
}

// binaryArrayFieldNames are payload keys that contain an array of base64-encoded strings.
// Each element is uploaded as a separate S3 object with a numeric index suffix.
var binaryArrayFieldNames = map[string]string{
	"pictureBase64List": "image/jpeg",
}

// binaryArrayFieldOutputNames overrides the S3 object key prefix for array fields.
// When absent, the payload field name is used as-is.
var binaryArrayFieldOutputNames = map[string]string{
	"pictureBase64List": "pictureList",
}

// extractBinaries scans canonical.Payload for known binary field names,
// uploads each to S3, and returns a list of BinaryRef pointers.
// S3 key format: {orgId}/events/{eventId}/{fieldName}{ext}
// Non-fatal: S3 errors are logged as WARN and skipped.
func extractBinaries(
	ctx context.Context,
	canonical ingestmod.CanonicalEvent,
	bucketKey string,
	orgId string,
	log zerolog.Logger,
) []ingestmod.BinaryRef {
	if bucketKey == "" {
		return nil
	}

	var refs []ingestmod.BinaryRef

	// Single-value binary fields
	for fieldName, expectedContentType := range binaryFieldNames {
		val, ok := canonical.Payload[fieldName]
		if !ok {
			continue
		}
		raw, ok := val.(string)
		if !ok || raw == "" {
			continue
		}
		objectKey := fmt.Sprintf("%s/events/%s/%s", orgId, canonical.EventId, fieldName)
		if ref := uploadBase64(ctx, bucketKey, raw, objectKey, fieldName, expectedContentType, 0, canonical.EventId, log); ref != nil {
			refs = append(refs, *ref)
		}
	}

	// Array binary fields (e.g. AIBOX pictureBase64List)
	for fieldName, expectedContentType := range binaryArrayFieldNames {
		val, ok := canonical.Payload[fieldName]
		if !ok {
			continue
		}
		arr, ok := val.([]any)
		if !ok || len(arr) == 0 {
			continue
		}
		outputName := fieldName
		if renamed, ok := binaryArrayFieldOutputNames[fieldName]; ok {
			outputName = renamed
		}
		for i, elem := range arr {
			raw, ok := elem.(string)
			if !ok || raw == "" {
				continue
			}
			objectKey := fmt.Sprintf("%s/events/%s/%s_%d", orgId, canonical.EventId, outputName, i)
			indexedField := fmt.Sprintf("%s_%d", fieldName, i)
			if ref := uploadBase64(ctx, bucketKey, raw, objectKey, indexedField, expectedContentType, i, canonical.EventId, log); ref != nil {
				refs = append(refs, *ref)
			}
		}
	}

	return refs
}

// uploadBase64 decodes a base64 string (with optional data URI prefix), derives the
// final contentType from the object key extension (production-grade — not trusting input),
// validates kind/contentType consistency, then uploads to S3.
// Returns a BinaryRef on success, nil on failure (failure is logged as WARN, non-fatal).
func uploadBase64(
	ctx context.Context,
	bucketKey, raw, objectKey, fieldName, expectedContentType string,
	sourceIndex int,
	eventId string,
	log zerolog.Logger,
) *ingestmod.BinaryRef {
	encoded := raw
	// Extract actual contentType from data URI prefix when present (e.g. "data:image/jpeg;base64,...")
	contentType := expectedContentType
	if idx := strings.Index(raw, ";base64,"); idx != -1 {
		encoded = raw[idx+8:]
		if strings.HasPrefix(raw, "data:") {
			if ct := raw[5:idx]; ct != "" {
				contentType = ct
			}
		}
	}

	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		data, err = base64.URLEncoding.DecodeString(encoded)
		if err != nil {
			log.Warn().
				Str("eventId", eventId).
				Str("field", fieldName).
				Msg("[s3writer] field is not valid base64, skipping")
			return nil
		}
	}

	// Append file extension to object key so detectContentType can verify from key.
	ext := extensionForContentType(contentType)
	objectKeyWithExt := objectKey + ext

	// Derive final contentType from key extension — do not trust raw input blindly.
	detected := detectContentType(objectKeyWithExt)
	if detected != "application/octet-stream" {
		contentType = detected
	}

	// Derive kind and role; validate consistency.
	kind := deriveKind(contentType)
	role := deriveRole(fieldName)
	if err := validateKindContentType(kind, contentType); err != nil {
		log.Warn().
			Str("eventId", eventId).
			Str("field", fieldName).
			Str("kind", kind).
			Str("contentType", contentType).
			Err(err).
			Msg("[s3writer] kind/contentType mismatch, skipping")
		return nil
	}

	if _, err = stos3minio.Upload(ctx, bucketKey, false, objectKeyWithExt, data, contentType); err != nil {
		log.Warn().
			Str("eventId", eventId).
			Str("field", fieldName).
			Str("objectKey", objectKeyWithExt).
			Err(err).
			Msg("[s3writer] S3 upload failed, skipping (non-blocking)")
		return nil
	}

	return &ingestmod.BinaryRef{
		ObjectId:    objectKeyWithExt,
		Bucket:      bucketKey,
		ContentType: contentType,
		FieldName:   fieldName,
		Kind:        kind,
		Role:        role,
		SourceIndex: sourceIndex,
	}
}

// detectContentType returns the MIME type inferred from the object key's file extension.
// This is the authoritative content type — derive from the key, not from caller input.
func detectContentType(key string) string {
	switch {
	case strings.HasSuffix(key, ".jpg"), strings.HasSuffix(key, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(key, ".png"):
		return "image/png"
	case strings.HasSuffix(key, ".gif"):
		return "image/gif"
	case strings.HasSuffix(key, ".webp"):
		return "image/webp"
	case strings.HasSuffix(key, ".mp4"):
		return "video/mp4"
	case strings.HasSuffix(key, ".webm"):
		return "video/webm"
	default:
		return "application/octet-stream"
	}
}

// extensionForContentType returns the canonical file extension for a known MIME type.
func extensionForContentType(ct string) string {
	switch ct {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	default:
		return ""
	}
}

// deriveKind returns the media kind from a MIME type.
func deriveKind(contentType string) string {
	switch {
	case strings.HasPrefix(contentType, "image/"):
		return "image"
	case strings.HasPrefix(contentType, "video/"):
		return "video"
	default:
		return "binary"
	}
}

// deriveRole returns the semantic role of a binary field based on its field name.
// Array fields carry the index suffix (e.g. "pictureBase64List_0").
func deriveRole(fieldName string) string {
	switch {
	case strings.HasPrefix(fieldName, "thumbnail"):
		return "thumbnail"
	case strings.HasPrefix(fieldName, "snapshot"), strings.HasPrefix(fieldName, "frame"):
		return "snapshot"
	case strings.HasPrefix(fieldName, "video"), strings.HasPrefix(fieldName, "clip"):
		return "clip"
	case strings.HasPrefix(fieldName, "pictureBase64List"):
		return "capture"
	default:
		// "image", "photo" and unrecognised fields → full capture
		return "full"
	}
}

// validateKindContentType returns an error when the kind and contentType are inconsistent.
func validateKindContentType(kind, contentType string) error {
	switch kind {
	case "image":
		if !strings.HasPrefix(contentType, "image/") {
			return fmt.Errorf("kind=image but contentType=%q", contentType)
		}
	case "video":
		if !strings.HasPrefix(contentType, "video/") {
			return fmt.Errorf("kind=video but contentType=%q", contentType)
		}
	}
	return nil
}
