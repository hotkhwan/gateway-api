// internal/services/devicemgmtsvc/bulk.go
package devicemgmtsvc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hotkhwan/gateway-api/internal/repo/cachedevicemgmt"
	"github.com/hotkhwan/gateway-api/models/gmod"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
	"github.com/xuri/excelize/v2"
)

// XLSX column order — stable contract
var xlsxColumns = []string{
	"tenantId", "orgId",
	"sourceFamily", "entityType", "entityId",
	"deviceId", "name", "description",
	"lat", "lng", "site", "zone",
}

const (
	sheetData   = "deviceManagement"
	sheetReadme = "readme"
	maxImportRows = 5000
)

// DownloadTemplate generates an empty XLSX template with example row prefilled from auth context.
func (s *DeviceManagementService) DownloadTemplate(ctx context.Context, tenantId, orgId string) (*bytes.Buffer, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.devicemgmtsvc", "DownloadTemplate", "devicemgmtsvc", "DownloadTemplate")
	defer end()

	f := excelize.NewFile()
	defer f.Close()

	// Rename default sheet
	defaultSheet := f.GetSheetName(0)
	if err := f.SetSheetName(defaultSheet, sheetData); err != nil {
		return nil, err
	}

	// Write header row
	for i, col := range xlsxColumns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheetData, cell, col)
	}

	// Write example row
	example := []interface{}{
		tenantId, orgId,
		"AIBOX", "channel", "30",
		"CAM-LOBBY-01", "Lobby Camera 01", "Camera near front lobby",
		13.7563, 100.5018, "Head Office", "Lobby",
	}
	for i, val := range example {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(sheetData, cell, val)
	}

	// Add readme sheet
	_, _ = f.NewSheet(sheetReadme)
	readmeLines := []string{
		"Device Management Import Template",
		"",
		fmt.Sprintf("Tenant ID: %s", tenantId),
		fmt.Sprintf("Org ID: %s", orgId),
		"",
		"Rules:",
		"- sourceFamily, entityType, entityId are REQUIRED",
		"- tenantId and orgId columns are informational; server uses auth context as source of truth",
		"- If tenantId/orgId in file does not match auth context, the row will be REJECTED",
		"- Business key for upsert: (tenantId, orgId, sourceFamily, entityType, entityId)",
		"- Duplicate business keys within the file will cause an error",
		"- lat/lng must be valid float numbers if provided",
		"- entityId is always treated as string (e.g. '30' not 30)",
		"- Blank rows are skipped",
	}
	for i, line := range readmeLines {
		cell, _ := excelize.CoordinatesToCellName(1, i+1)
		_ = f.SetCellValue(sheetReadme, cell, line)
	}

	// Set active sheet to data
	idx, _ := f.GetSheetIndex(sheetData)
	f.SetActiveSheet(idx)

	buf := new(bytes.Buffer)
	if err := f.Write(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// ExportXlsx exports current device management records as an XLSX file.
func (s *DeviceManagementService) ExportXlsx(ctx context.Context, tenantId, orgId, sourceFamily, entityType string) (*bytes.Buffer, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.devicemgmtsvc", "ExportXlsx", "devicemgmtsvc", "ExportXlsx")
	defer end()

	records, err := s.repo.ListForExport(ctx, tenantId, orgId, sourceFamily, entityType)
	if err != nil {
		return nil, err
	}

	f := excelize.NewFile()
	defer f.Close()

	defaultSheet := f.GetSheetName(0)
	if err := f.SetSheetName(defaultSheet, sheetData); err != nil {
		return nil, err
	}

	// Header
	for i, col := range xlsxColumns {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheetData, cell, col)
	}

	// Data rows
	for rowIdx, rec := range records {
		row := rowIdx + 2
		vals := []interface{}{
			rec.TenantId, rec.OrgId,
			rec.SourceFamily, rec.EntityType, rec.EntityId,
			rec.DeviceId, rec.Name, rec.Description,
			rec.Lat, rec.Lng, rec.Site, rec.Zone,
		}
		for colIdx, val := range vals {
			cell, _ := excelize.CoordinatesToCellName(colIdx+1, row)
			_ = f.SetCellValue(sheetData, cell, val)
		}
	}

	buf := new(bytes.Buffer)
	if err := f.Write(buf); err != nil {
		return nil, err
	}
	return buf, nil
}

// ImportResult holds aggregated import counters.
type ImportResult struct {
	Inserted int
	Updated  int
	Skipped  int
	IDs      []string
	Errors   []gmod.BulkImportError
}

// ImportFromXlsx parses an XLSX file and upserts device management records.
func (s *DeviceManagementService) ImportFromXlsx(ctx context.Context, tenantId, orgId string, file io.Reader, dryRun bool) (*ImportResult, error) {
	ctx, end, _ := traceutil.StartLite(ctx, "gateway.devicemgmtsvc", "ImportFromXlsx", "devicemgmtsvc", "ImportFromXlsx")
	defer end()

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(file); err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	xlsx, err := excelize.OpenReader(buf)
	if err != nil {
		return nil, fmt.Errorf("failed to open xlsx: %w", err)
	}
	defer xlsx.Close()

	sheetName := xlsx.GetSheetName(0)
	rows, err := xlsx.GetRows(sheetName)
	if err != nil {
		return nil, fmt.Errorf("failed to read rows: %w", err)
	}

	if len(rows) < 2 {
		return &ImportResult{}, nil
	}

	// Parse header
	headerIdx := parseHeader(rows[0])
	if headerIdx["sourceFamily"] < 0 || headerIdx["entityType"] < 0 || headerIdx["entityId"] < 0 {
		return nil, fmt.Errorf("missing required columns: sourceFamily, entityType, entityId")
	}

	result := &ImportResult{}
	seenKeys := map[string]int{} // business key → row number (detect intra-file duplicates)

	dataRows := rows[1:]
	if len(dataRows) > maxImportRows {
		return nil, fmt.Errorf("too many rows: %d (max %d)", len(dataRows), maxImportRows)
	}

	for i, row := range dataRows {
		rowNum := i + 2 // 1-indexed, row 1 is header

		rec, rowErr := parseImportRow(headerIdx, row, rowNum, tenantId, orgId)
		if rowErr != nil {
			result.Errors = append(result.Errors, *rowErr)
			continue
		}
		if rec == nil {
			// blank row
			continue
		}

		// Check intra-file duplicate
		bk := fmt.Sprintf("%s:%s:%s", rec.SourceFamily, rec.EntityType, rec.EntityId)
		if prevRow, dup := seenKeys[bk]; dup {
			result.Errors = append(result.Errors, gmod.BulkImportError{
				Row: rowNum, RecordKey: bk, Field: "businessKey",
				Message: fmt.Sprintf("duplicate business key (first seen at row %d)", prevRow),
			})
			continue
		}
		seenKeys[bk] = rowNum

		if dryRun {
			// Simulate: check existing record
			existing, _ := s.repo.FindByEntity(ctx, tenantId, orgId, rec.SourceFamily, rec.EntityType, rec.EntityId)
			if existing == nil {
				result.Inserted++
			} else if recordChanged(existing, rec) {
				result.Updated++
			} else {
				result.Skipped++
			}
			continue
		}

		// Real upsert
		outcome, err := s.repo.UpsertByBusinessKey(ctx, rec)
		if err != nil {
			result.Errors = append(result.Errors, gmod.BulkImportError{
				Row: rowNum, RecordKey: bk, Message: err.Error(),
			})
			continue
		}

		switch outcome {
		case "inserted":
			result.Inserted++
			result.IDs = append(result.IDs, rec.DeviceMgmtId)
		case "updated":
			result.Updated++
			// Invalidate cache for updated records
			cachedevicemgmt.Invalidate(ctx, tenantId, orgId, rec.SourceFamily, rec.EntityType, rec.EntityId)
		default:
			result.Skipped++
		}
	}

	return result, nil
}

// AutoUpsertFromEvent creates or conservatively merges a device_management record from ingest event data.
// Only fills empty fields in existing records; never overwrites non-empty values.
func (s *DeviceManagementService) AutoUpsertFromEvent(
	ctx context.Context,
	tenantId, orgId, sourceFamily string,
	ref *ingestmod.DeviceIdentity,
	hints ingestmod.AutoUpsertHints,
) {
	if ref == nil || ref.Type == "" || ref.ID == "" {
		return
	}
	if sourceFamily == "" || tenantId == "" || orgId == "" {
		return
	}

	existing, _ := s.repo.FindByEntity(ctx, tenantId, orgId, sourceFamily, ref.Type, ref.ID)
	now := time.Now().UTC()

	if existing == nil {
		// Create new record
		rec := &ingestmod.DeviceManagement{
			DeviceMgmtId: uuid.NewString(),
			TenantId:     tenantId,
			OrgId:        orgId,
			SourceFamily: sourceFamily,
			EntityType:   ref.Type,
			EntityId:     ref.ID,
			DeviceId:     hints.DeviceId,
			Name:         hints.Name,
			Description:  hints.Description,
			Lat:          hints.Lat,
			Lng:          hints.Lng,
			Site:         hints.Site,
			Zone:         hints.Zone,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		_ = s.repo.Insert(ctx, rec)
		return
	}

	// Conservative merge: only fill empty fields
	changed := false
	if existing.DeviceId == "" && hints.DeviceId != "" {
		existing.DeviceId = hints.DeviceId
		changed = true
	}
	if existing.Name == "" && hints.Name != "" {
		existing.Name = hints.Name
		changed = true
	}
	if existing.Description == "" && hints.Description != "" {
		existing.Description = hints.Description
		changed = true
	}
	if existing.Lat == 0 && hints.Lat != 0 {
		existing.Lat = hints.Lat
		changed = true
	}
	if existing.Lng == 0 && hints.Lng != 0 {
		existing.Lng = hints.Lng
		changed = true
	}
	if existing.Site == "" && hints.Site != "" {
		existing.Site = hints.Site
		changed = true
	}
	if existing.Zone == "" && hints.Zone != "" {
		existing.Zone = hints.Zone
		changed = true
	}

	if !changed {
		return
	}

	existing.UpdatedAt = now
	_, _ = s.repo.UpsertByBusinessKey(ctx, existing)
	cachedevicemgmt.Invalidate(ctx, tenantId, orgId, sourceFamily, ref.Type, ref.ID)
}

// parseHeader maps column names to their index positions.
func parseHeader(row []string) map[string]int {
	idx := map[string]int{
		"tenantId": -1, "orgId": -1,
		"sourceFamily": -1, "entityType": -1, "entityId": -1,
		"deviceId": -1, "name": -1, "description": -1,
		"lat": -1, "lng": -1, "site": -1, "zone": -1,
	}
	for i, h := range row {
		key := strings.TrimSpace(strings.ToLower(h))
		// Normalize common aliases
		switch key {
		case "sourcefamily":
			key = "sourceFamily"
		case "entitytype":
			key = "entityType"
		case "entityid":
			key = "entityId"
		case "deviceid":
			key = "deviceId"
		case "tenantid":
			key = "tenantId"
		case "orgid":
			key = "orgId"
		default:
			// Already lowercase — match directly
		}
		if _, ok := idx[key]; ok {
			idx[key] = i
		}
	}
	return idx
}

// cellVal safely extracts a trimmed string from a row at given index.
func cellVal(row []string, idx int) string {
	if idx < 0 || idx >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[idx])
}

// parseImportRow validates and converts a single XLSX row into a DeviceManagement record.
func parseImportRow(headerIdx map[string]int, row []string, rowNum int, tenantId, orgId string) (*ingestmod.DeviceManagement, *gmod.BulkImportError) {
	sourceFamily := cellVal(row, headerIdx["sourceFamily"])
	entityType := cellVal(row, headerIdx["entityType"])
	entityId := cellVal(row, headerIdx["entityId"])

	// Blank row — skip silently
	if sourceFamily == "" && entityType == "" && entityId == "" {
		return nil, nil
	}

	bk := fmt.Sprintf("%s:%s:%s", sourceFamily, entityType, entityId)

	// Validate required fields
	if sourceFamily == "" {
		return nil, &gmod.BulkImportError{Row: rowNum, RecordKey: bk, Field: "sourceFamily", Message: "sourceFamily is required"}
	}
	if entityType == "" {
		return nil, &gmod.BulkImportError{Row: rowNum, RecordKey: bk, Field: "entityType", Message: "entityType is required"}
	}
	if entityId == "" {
		return nil, &gmod.BulkImportError{Row: rowNum, RecordKey: bk, Field: "entityId", Message: "entityId is required"}
	}

	// Validate scope — reject if tenantId/orgId in file doesn't match auth context
	fileTenant := cellVal(row, headerIdx["tenantId"])
	if fileTenant != "" && fileTenant != tenantId {
		return nil, &gmod.BulkImportError{Row: rowNum, RecordKey: bk, Field: "tenantId", Message: "tenantId does not match authenticated tenant context"}
	}
	fileOrg := cellVal(row, headerIdx["orgId"])
	if fileOrg != "" && fileOrg != orgId {
		return nil, &gmod.BulkImportError{Row: rowNum, RecordKey: bk, Field: "orgId", Message: "orgId does not match active org context"}
	}

	// Parse optional fields
	var lat, lng float64
	if latStr := cellVal(row, headerIdx["lat"]); latStr != "" {
		v, err := strconv.ParseFloat(latStr, 64)
		if err != nil {
			return nil, &gmod.BulkImportError{Row: rowNum, RecordKey: bk, Field: "lat", Message: "invalid float value"}
		}
		lat = v
	}
	if lngStr := cellVal(row, headerIdx["lng"]); lngStr != "" {
		v, err := strconv.ParseFloat(lngStr, 64)
		if err != nil {
			return nil, &gmod.BulkImportError{Row: rowNum, RecordKey: bk, Field: "lng", Message: "invalid float value"}
		}
		lng = v
	}

	// Normalize entityId: strip trailing ".0" from Excel numeric coercion (e.g. "30.0" → "30")
	entityId = normalizeEntityId(entityId)

	now := time.Now().UTC()
	return &ingestmod.DeviceManagement{
		DeviceMgmtId: uuid.NewString(),
		TenantId:     tenantId,
		OrgId:        orgId,
		SourceFamily: sourceFamily,
		EntityType:   entityType,
		EntityId:     entityId,
		DeviceId:     cellVal(row, headerIdx["deviceId"]),
		Name:         cellVal(row, headerIdx["name"]),
		Description:  cellVal(row, headerIdx["description"]),
		Lat:          lat,
		Lng:          lng,
		Site:         cellVal(row, headerIdx["site"]),
		Zone:         cellVal(row, headerIdx["zone"]),
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// normalizeEntityId strips trailing ".0" added by Excel for numeric values.
func normalizeEntityId(s string) string {
	if strings.HasSuffix(s, ".0") {
		trimmed := strings.TrimSuffix(s, ".0")
		if _, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return trimmed
		}
	}
	return s
}

// recordChanged checks if any mutable field differs between existing and incoming.
func recordChanged(existing *ingestmod.DeviceManagement, incoming *ingestmod.DeviceManagement) bool {
	return existing.DeviceId != incoming.DeviceId ||
		existing.Name != incoming.Name ||
		existing.Description != incoming.Description ||
		existing.Lat != incoming.Lat ||
		existing.Lng != incoming.Lng ||
		existing.Site != incoming.Site ||
		existing.Zone != incoming.Zone
}
