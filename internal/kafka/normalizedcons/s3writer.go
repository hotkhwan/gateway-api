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

// binaryFieldNames are payload keys that may contain binary (base64-encoded) data.
var binaryFieldNames = map[string]string{
	"image":     "image/jpeg",
	"photo":     "image/jpeg",
	"frame":     "image/jpeg",
	"snapshot":  "image/jpeg",
	"thumbnail": "image/jpeg",
	"video":     "video/mp4",
	"clip":      "video/mp4",
}

// extractBinaries scans canonical.Payload for known binary field names,
// uploads each to S3, and returns a list of BinaryRef pointers.
// S3 key format: {tenantId}/{orgId}/events/{eventId}/{fieldName}
// Non-fatal: S3 errors are logged as WARN and skipped.
func extractBinaries(
	ctx context.Context,
	canonical ingestmod.CanonicalEvent,
	bucketKey string,
	tenantId, orgId string,
	log zerolog.Logger,
) []ingestmod.BinaryRef {
	if bucketKey == "" {
		return nil
	}

	var refs []ingestmod.BinaryRef

	for fieldName, contentType := range binaryFieldNames {
		val, ok := canonical.Payload[fieldName]
		if !ok {
			continue
		}
		raw, ok := val.(string)
		if !ok || raw == "" {
			continue
		}

		// Decode base64 (strip "data:...;base64," prefix if present)
		encoded := raw
		if idx := strings.Index(raw, ";base64,"); idx != -1 {
			encoded = raw[idx+8:]
			// extract content type from data URI if available
			if strings.HasPrefix(raw, "data:") {
				ct := raw[5:idx]
				if ct != "" {
					contentType = ct
				}
			}
		}

		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			// try URL-safe variant
			data, err = base64.URLEncoding.DecodeString(encoded)
			if err != nil {
				log.Warn().
					Str("eventId", canonical.EventId).
					Str("field", fieldName).
					Msg("[s3writer] field is not valid base64, skipping")
				continue
			}
		}

		objectKey := fmt.Sprintf("%s/%s/events/%s/%s", tenantId, orgId, canonical.EventId, fieldName)
		_, err = stos3minio.Upload(ctx, bucketKey, false, objectKey, data, contentType)
		if err != nil {
			log.Warn().
				Str("eventId", canonical.EventId).
				Str("field", fieldName).
				Str("objectKey", objectKey).
				Err(err).
				Msg("[s3writer] S3 upload failed, skipping (non-blocking)")
			continue
		}

		refs = append(refs, ingestmod.BinaryRef{
			ObjectId:    objectKey,
			ContentType: contentType,
			FieldName:   fieldName,
		})
	}

	return refs
}
