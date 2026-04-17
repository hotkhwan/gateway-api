// internal/services/devicesvc/cameraImport.go
package devicesvc

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/hotkhwan/gateway-api/internal/logger"
	"github.com/hotkhwan/gateway-api/models/devmod"
)

// ============================================================
// ImportResult — summary ของ import ทั้ง session
// ============================================================

type ImportResult struct {
	TotalRows         int                       `json:"totalRows"`
	Inserted          int                       `json:"inserted"`
	InvalidRows       []string                  `json:"invalidRows,omitempty"`       // row numbers ที่ parse fail
	DuplicateIPInFile []string                  `json:"duplicateIPInFile,omitempty"` // IP ซ้ำภายในไฟล์
	DuplicateIPInDB   []string                  `json:"duplicateIPInDB,omitempty"`   // IP ที่มีใน DB แล้ว
	PermifySyncFailed []string                  `json:"permifySyncFailed,omitempty"` // device IDs ที่ permify fail
	Results           []devmod.BulkImportResult `json:"results"`
}

// ============================================================
// ImportFromCSV
// ============================================================

func (s *CameraService) ImportFromCSV(
	ctx context.Context,
	tenantId, workspaceId, callerID string,
	file io.Reader,
) (*ImportResult, error) {
	log := logger.FromCtx(ctx, "devicesvc", "CameraService.ImportFromCSV")

	if err := s.guardManageOrg(ctx, tenantId, workspaceId, callerID); err != nil {
		return nil, err
	}

	parsed, dupIPInFile, invalidRows, err := parseCSVToItems(ctx, file)
	if err != nil {
		log.Error().Err(err).Msg("❌ CSV parse error")
		return nil, fmt.Errorf("csv parse error: %w", err)
	}

	return s.doImport(ctx, tenantId, workspaceId, callerID, parsed, dupIPInFile, invalidRows)
}

// ============================================================
// ImportFromXLSX
// ============================================================

func (s *CameraService) ImportFromXLSX(
	ctx context.Context,
	tenantId, workspaceId, callerID string,
	file io.Reader,
) (*ImportResult, error) {
	log := logger.FromCtx(ctx, "devicesvc", "CameraService.ImportFromXLSX")

	if err := s.guardManageOrg(ctx, tenantId, workspaceId, callerID); err != nil {
		return nil, err
	}

	parsed, dupIPInFile, invalidRows, err := parseXLSXToItems(ctx, file)
	if err != nil {
		log.Error().Err(err).Msg("❌ XLSX parse error")
		return nil, fmt.Errorf("xlsx parse error: %w", err)
	}

	return s.doImport(ctx, tenantId, workspaceId, callerID, parsed, dupIPInFile, invalidRows)
}

// ============================================================
// doImport — shared logic หลัง parse
// ============================================================

func (s *CameraService) doImport(
	ctx context.Context,
	tenantId, workspaceId, callerID string,
	parsed []devmod.BulkImportItem,
	dupIPInFile []string,
	invalidRows []string,
) (*ImportResult, error) {
	log := logger.FromCtx(ctx, "devicesvc", "CameraService.doImport")

	result := &ImportResult{
		TotalRows:         len(parsed) + len(invalidRows),
		InvalidRows:       invalidRows,
		DuplicateIPInFile: dupIPInFile,
	}

	// ตรวจ IP ซ้ำใน DB
	ips := make([]string, 0, len(parsed))
	for _, item := range parsed {
		if item.IP != "" {
			ips = append(ips, item.IP)
		}
	}
	dupIPInDB, err := s.repo.FindDuplicateIPs(ctx, workspaceId, ips)
	if err != nil {
		log.Warn().Err(err).Msg("⚠️ DuplicateIP check failed, continuing")
	}
	result.DuplicateIPInDB = dupIPInDB
	dupIPSet := make(map[string]bool)
	for _, ip := range dupIPInDB {
		dupIPSet[ip] = true
	}

	// กรอง row ที่ IP ซ้ำใน DB ออก
	toInsert := make([]devmod.BulkImportItem, 0, len(parsed))
	for i, item := range parsed {
		if item.IP != "" && dupIPSet[item.IP] {
			result.Results = append(result.Results, devmod.BulkImportResult{
				Row:     i + 2,
				Name:    item.Name,
				Success: false,
				Error:   fmt.Sprintf("duplicate IP in DB: %s", item.IP),
			})
			continue
		}
		toInsert = append(toInsert, item)
	}

	if len(toInsert) == 0 {
		log.Info().Msg("ℹ️ No valid rows to insert after dedup")
		return result, nil
	}

	// BulkCreate (Mongo + Permify)
	bulk, err := s.BulkCreate(ctx, tenantId, workspaceId, callerID, toInsert)
	if err != nil {
		return nil, err
	}

	result.Inserted = bulk.Inserted
	result.PermifySyncFailed = bulk.PermifySyncFailed
	result.Results = append(result.Results, bulk.Results...)

	log.Info().
		Int("total", result.TotalRows).
		Int("inserted", result.Inserted).
		Int("invalidRows", len(result.InvalidRows)).
		Int("dupInFile", len(result.DuplicateIPInFile)).
		Int("dupInDB", len(result.DuplicateIPInDB)).
		Msg("✅ Import complete")

	return result, nil
}

// ============================================================
// parseCSVToItems — reuse parse logic จาก devsvc เดิม
// ============================================================

func parseCSVToItems(ctx context.Context, file io.Reader) ([]devmod.BulkImportItem, []string, []string, error) {
	raw, dupIPs, invalidRows, err := parseCSVRaw(ctx, file)
	if err != nil {
		return nil, nil, nil, err
	}
	items := rawToItems(raw)
	return items, dupIPs, invalidRows, nil
}

func parseXLSXToItems(ctx context.Context, file io.Reader) ([]devmod.BulkImportItem, []string, []string, error) {
	raw, dupIPs, invalidRows, err := parseXLSXRaw(ctx, file)
	if err != nil {
		return nil, nil, nil, err
	}
	items := rawToItems(raw)
	return items, dupIPs, invalidRows, nil
}

// rawToItems แปลง []map[string]interface{} → []BulkImportItem
func rawToItems(raw []map[string]interface{}) []devmod.BulkImportItem {
	items := make([]devmod.BulkImportItem, 0, len(raw))
	for _, r := range raw {
		item := devmod.BulkImportItem{Raw: r}
		if v, ok := r["name"].(string); ok {
			item.Name = strings.TrimSpace(v)
		}
		if v, ok := r["url"].(string); ok {
			item.URL = strings.TrimSpace(v)
		}
		if v, ok := r["ip"].(string); ok {
			item.IP = strings.TrimSpace(v)
		}
		if v, ok := r["lat"].(float64); ok {
			item.Lat = v
		}
		if v, ok := r["long"].(float64); ok {
			item.Lng = v
		}
		if v, ok := r["district"].(string); ok {
			item.District = strings.TrimSpace(v)
		}
		if v, ok := r["brand"].(string); ok {
			item.Brand = strings.TrimSpace(v)
		}
		items = append(items, item)
	}
	return items
}

// ============================================================
// Compatibility wrapper — ใช้ logic เดิมจาก devsvc/import.go
// ย้าย ParseCSVFile / ParseXLSXFile มาไว้ที่นี่
// (หรือ import จาก package เดิม ถ้า backward compat ต้องการ)
// ============================================================

// parseCSVRaw / parseXLSXRaw — เรียก logic เดิม
// ย้ายฟังก์ชัน ParseCSVFile, ParseXLSXFile จาก devsvc/import.go มาใส่ที่นี่
// หรือถ้า keep เดิม ให้ import package แล้วเรียก
// เพื่อความ clean จัดไว้ใน file นี้เลย

func parseCSVRaw(ctx context.Context, file io.Reader) ([]map[string]interface{}, []string, []string, error) {
	// ใช้ logic เดิมจาก devsvc.ParseCSVFile
	// Copy มาที่นี่ หรือ refactor เป็น shared util
	// ตัวอย่าง: เรียกผ่าน package เดิม
	// return devsvc.ParseCSVFile(ctx, file)
	// ---
	// ถ้าย้าย logic มาที่นี่เลย (recommended):
	return parseFileRaw(ctx, file, "csv")
}

func parseXLSXRaw(ctx context.Context, file io.Reader) ([]map[string]interface{}, []string, []string, error) {
	return parseFileRaw(ctx, file, "xlsx")
}
