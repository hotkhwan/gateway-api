// internal/services/mapsvc/klm.go
package mapsvc

import (
	"context"
	"encoding/base64"
	"sort"
	"time"

	"klynx/config"
	"klynx/internal/repo/stos3minio"
	"klynx/models/gmod"
	"klynx/models/mapmod"
	"klynx/utils"
	"klynx/utils/traceutil"

	"github.com/minio/minio-go/v7"
)

func ListMAPPosition(ctx context.Context, page int, perPages int, filters map[string]string, sortOrder string) ([]mapmod.KMLFile, gmod.Pagination, error) {
	ctx, endSpan, log := traceutil.StartLite(
		ctx,
		"klynx/mapsvc",
		"maps.ListMAPPosition",
		"mapsvc", "ListMAPPosition",
	)
	defer endSpan()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	const prefix = "maps/kml/"
	bucket := config.S3PublicBuckets["map"]

	log.Info().
		Int("page", page).
		Int("perPages", perPages).
		Str("sortOrder", sortOrder).
		Str("bucket", bucket).
		Msg("📥 Listing KML map files from S3")

	objectCh := config.S3Client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	})

	var allObjects []minio.ObjectInfo
	skipped := 0
	for obj := range objectCh {
		if obj.Err != nil {
			log.Warn().Err(obj.Err).Str("key", obj.Key).Msg("⚠️ Skipping object due to read error")
			skipped++
			continue
		}
		allObjects = append(allObjects, obj)
	}

	log.Info().
		Int("total_found", len(allObjects)).
		Int("skipped_errors", skipped).
		Msg("📦 Fetched objects from S3")

	// 🔢 Sort
	sort.Slice(allObjects, func(i, j int) bool {
		if sortOrder == "asc" {
			return allObjects[i].LastModified.Before(allObjects[j].LastModified)
		}
		return allObjects[i].LastModified.After(allObjects[j].LastModified)
	})

	// ⏱ Pagination
	total := len(allObjects)
	startIdx := (page - 1) * perPages
	if startIdx > total {
		startIdx = total
	}
	endIdx := startIdx + perPages
	if endIdx > total {
		endIdx = total
	}
	selected := allObjects[startIdx:endIdx]

	log.Debug().
		Int("start", startIdx).
		Int("end", endIdx).
		Int("selected", len(selected)).
		Msg("🧮 Pagination result")

	var results []mapmod.KMLFile
	for _, obj := range selected {
		info, err := config.S3Client.StatObject(ctx, bucket, obj.Key, minio.StatObjectOptions{})
		if err != nil {
			log.Warn().Err(err).Str("key", obj.Key).Msg("⚠️ Skipping StatObject due to error")
			continue
		}

		fileType := info.UserMetadata["x-amz-meta-filetype"]
		description := "No description"
		if encoded := info.UserMetadata["x-amz-meta-description"]; encoded != "" {
			if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil {
				description = string(decoded)
			} else {
				log.Warn().Err(err).Str("key", obj.Key).Msg("⚠️ Failed to decode base64 description")
			}
		}

		results = append(results, mapmod.KMLFile{
			Name: obj.Key[len(prefix):],
			Path: stos3minio.GetS3URL("analytic", obj.Key),
			Size: mapmod.FileSize{
				Bytes:     obj.Size,
				Formatted: utils.FormatFileSize(obj.Size),
			},
			FileType:     fileType,
			Description:  description,
			LastModified: obj.LastModified,
		})
	}

	log.Info().
		Int("returned", len(results)).
		Msg("✅ Returning KML file list with pagination")

	pagination := gmod.Pagination{
		Page:         page,
		PerPages:     perPages,
		TotalRecords: total,
		TotalPages:   (total + perPages - 1) / perPages,
		SortOrder:    sortOrder,
	}

	return results, pagination, nil
}
