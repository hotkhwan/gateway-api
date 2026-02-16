// internal/services/devsvc/import.go
package devsvc

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/hotkhwan/gateway-api/config"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

/* =========================
 * Header normalization
 * ========================= */
var headerAlias = map[string]string{
	// latitude
	"lat":      "lat",
	"latitude": "lat",

	// longitude (รองรับสะกดผิดยอดฮิต)
	"long":       "long",
	"lon":        "long",
	"lng":        "long",
	"longtitude": "long",
	"longitude":  "long",

	// stream url
	"streamurl":  "url",
	"stream_url": "url",
	"url":        "url",

	// optional fields
	"name":        "name",
	"description": "description",
	"desc":        "description",
	"group":       "groups",
	"groups":      "groups",

	// explicit ip (ถ้าเคยมีคอลัมน์นี้)
	"ip": "ip",
}

// ฟิลด์จำเป็นตามไฟล์ตัวอย่าง
// หมายเหตุ: lat/long จะตรวจเพิ่มว่าเป็น number ด้วย (ดูฟังก์ชัน coerceLatLong)
var requiredFields = []string{"name", "url", "lat", "long"}

func canonKey(s string) string {
	k := strings.ToLower(strings.TrimSpace(s))
	if v, ok := headerAlias[k]; ok {
		return v
	}
	return k
}

var ipv4Re = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)

/* =========================
 * Helpers
 * ========================= */

// parse สตริงให้เป็น float64 (ตัดช่องว่าง/คอมมา)
func parseFloatString(s string) (float64, error) {
	ss := strings.TrimSpace(s)
	// ตัด comma กรณีเป็น thousands separator
	ss = strings.ReplaceAll(ss, ",", "")
	return strconv.ParseFloat(ss, 64)
}

// รับค่า interface{} แล้วพยายามแปลงเป็น float64
func parseFloatAny(v interface{}) (float64, error) {
	switch t := v.(type) {
	case float64:
		return t, nil
	case float32:
		return float64(t), nil
	case int, int32, int64:
		return strconv.ParseFloat(fmt.Sprint(t), 64)
	case string:
		return parseFloatString(t)
	default:
		return 0, fmt.Errorf("unsupported type for float: %T", v)
	}
}

// บังคับให้ lat/long เป็น number; ถ้าพัง -> error
func coerceLatLong(item map[string]interface{}) error {
	latRaw, ok1 := item["lat"]
	lonRaw, ok2 := item["long"]
	if !ok1 || !ok2 {
		return errors.New("missing lat/long")
	}
	lat, err := parseFloatAny(latRaw)
	if err != nil {
		return fmt.Errorf("invalid lat: %v", err)
	}
	lon, err := parseFloatAny(lonRaw)
	if err != nil {
		return fmt.Errorf("invalid long: %v", err)
	}
	item["lat"] = lat
	item["long"] = lon
	return nil
}

func extractIPFromItem(item map[string]interface{}) string {
	// 1) ใช้ ip ถ้ามี
	if ip, ok := item["ip"].(string); ok && strings.TrimSpace(ip) != "" {
		return strings.TrimSpace(ip)
	}
	// 2) ลอง parse จาก url
	if raw, ok := item["url"].(string); ok && strings.TrimSpace(raw) != "" {
		uStr := strings.TrimSpace(raw)

		if u, err := url.Parse(uStr); err == nil && u.Host != "" {
			h := u.Host
			if i := strings.IndexByte(h, ':'); i >= 0 {
				h = h[:i]
			}
			if h != "" {
				return h
			}
		}
		if m := ipv4Re.FindString(uStr); m != "" {
			return m
		}
	}
	return ""
}

/* =========================
 * CSV
 * ========================= */
func ParseCSVFile(ctx context.Context, file io.Reader) ([]map[string]interface{}, []string, []string, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/devsvc", "devices.ParseCSVFile", "devsvc", "ParseCSVFile")
	defer end()

	_, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	log.Info().Msg("📥 Parsing CSV file")

	r := csv.NewReader(file)
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1

	records, err := r.ReadAll()
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to read CSV")
		return nil, nil, nil, err
	}
	if len(records) < 2 {
		log.Warn().Msg("⚠️ No data rows found in CSV")
		return nil, nil, nil, errors.New("no data rows found")
	}

	// normalize headers
	rawHeaders := records[0]
	headers := make([]string, len(rawHeaders))
	for i, h := range rawHeaders {
		headers[i] = canonKey(h)
	}

	valid := []map[string]interface{}{}
	invalidRows := []string{}
	ipMap := map[string]int{}

	for i, row := range records[1:] {
		item := map[string]interface{}{}
		for j, val := range row {
			if j < len(headers) {
				key := headers[j]
				if key == "" {
					continue
				}
				item[key] = strings.TrimSpace(val)
			}
		}

		// ตรวจฟิลด์จำเป็นเบื้องต้น (name/url ต้องไม่ว่าง)
		missing := false
		for _, field := range requiredFields {
			if v, ok := item[field]; !ok || strings.TrimSpace(fmt.Sprint(v)) == "" {
				missing = true
				break
			}
		}
		if missing {
			invalidRows = append(invalidRows, fmt.Sprint(i+2))
			continue
		}

		// บังคับ lat/long เป็น float64
		if err := coerceLatLong(item); err != nil {
			invalidRows = append(invalidRows, fmt.Sprintf("%d", i+2))
			continue
		}

		// ดึง IP สำหรับตรวจซ้ำ
		if ip := extractIPFromItem(item); ip != "" {
			item["ip"] = ip
			ipMap[ip]++
		}

		valid = append(valid, item)
	}

	// IP ที่ซ้ำในไฟล์
	suspected := []string{}
	for ip, count := range ipMap {
		if count > 1 {
			suspected = append(suspected, ip)
		}
	}

	log.Info().
		Int("valid", len(valid)).
		Int("invalid", len(invalidRows)).
		Int("suspected_dup_ip", len(suspected)).
		Msg("✅ Parsed CSV file")

	return valid, suspected, invalidRows, nil
}

/* =========================
 * XLSX
 * ========================= */
func ParseXLSXFile(ctx context.Context, file io.Reader) ([]map[string]interface{}, []string, []string, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/devsvc", "devices.ParseXLSXFile", "devsvc", "ParseXLSXFile")
	defer end()

	_, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	log.Info().Msg("📥 Parsing XLSX file")

	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(file); err != nil {
		log.Error().Err(err).Msg("❌ Failed to read XLSX buffer")
		return nil, nil, nil, err
	}

	xlsx, err := excelize.OpenReader(buf)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to open XLSX file")
		return nil, nil, nil, err
	}

	sheetName := xlsx.GetSheetName(0)
	rows, err := xlsx.GetRows(sheetName)
	if err != nil || len(rows) < 2 {
		log.Warn().Err(err).Msg("⚠️ No data rows found in XLSX")
		return nil, nil, nil, errors.New("no rows or read error")
	}

	// normalize headers
	rawHeaders := rows[0]
	headers := make([]string, len(rawHeaders))
	for i, h := range rawHeaders {
		headers[i] = canonKey(h)
	}

	valid := []map[string]interface{}{}
	invalidRows := []string{}
	ipMap := map[string]int{}

	for i, row := range rows[1:] {
		item := map[string]interface{}{}
		for j, val := range row {
			if j < len(headers) {
				key := headers[j]
				if key == "" {
					continue
				}
				item[key] = strings.TrimSpace(val)
			}
		}

		// ตรวจฟิลด์จำเป็นเบื้องต้น
		missing := false
		for _, field := range requiredFields {
			if v, ok := item[field]; !ok || strings.TrimSpace(fmt.Sprint(v)) == "" {
				missing = true
				break
			}
		}
		if missing {
			invalidRows = append(invalidRows, fmt.Sprint(i+2))
			continue
		}

		// บังคับ lat/long เป็น float64
		if err := coerceLatLong(item); err != nil {
			invalidRows = append(invalidRows, fmt.Sprintf("%d", i+2))
			continue
		}

		// ดึง IP สำหรับตรวจซ้ำ
		if ip := extractIPFromItem(item); ip != "" {
			item["ip"] = ip
			ipMap[ip]++
		}

		valid = append(valid, item)
	}

	// IP ที่ซ้ำในไฟล์
	suspected := []string{}
	for ip, count := range ipMap {
		if count > 1 {
			suspected = append(suspected, ip)
		}
	}

	log.Info().
		Int("valid", len(valid)).
		Int("invalid", len(invalidRows)).
		Int("suspected_dup_ip", len(suspected)).
		Msg("✅ Parsed XLSX file")

	return valid, suspected, invalidRows, nil
}

/* =========================
 * Duplicate check in DB
 * ========================= */
func FindDuplicateIPs(ctx context.Context, devices []map[string]interface{}) ([]string, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/devsvc", "devices.FindDuplicateIPs", "devsvc", "FindDuplicateIPs")
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	log.Info().Int("device_count", len(devices)).Msg("🔎 Checking for duplicate IPs in DB")

	ips := []string{}
	for _, d := range devices {
		if ip, ok := d["ip"].(string); ok && strings.TrimSpace(ip) != "" {
			ips = append(ips, ip)
		}
	}

	if len(ips) == 0 {
		log.Info().Msg("ℹ️ No IPs provided to check duplicates")
		return nil, nil
	}

	coll := config.MongoClient.Database("klynx").Collection("camera")
	cursor, err := coll.Find(ctx, bson.M{"ip": bson.M{"$in": ips}}, options.Find().SetProjection(bson.M{"ip": 1}))
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to find IPs in DB")
		return nil, err
	}
	defer cursor.Close(ctx)

	found := []string{}
	for cursor.Next(ctx) {
		var doc struct {
			IP string `bson:"ip"`
		}
		if err := cursor.Decode(&doc); err == nil {
			found = append(found, doc.IP)
		}
	}

	log.Info().Int("found_duplicates", len(found)).Msg("✅ Finished checking for duplicate IPs")
	return found, nil
}

/* =========================
 * Insert
 * ========================= */
func InsertDevices(ctx context.Context, devices []map[string]interface{}) (int, error) {
	ctx, end, log := traceutil.StartLite(ctx, "github.com/hotkhwan/gateway-api/devsvc", "devices.InsertDevices", "devsvc", "InsertDevices")
	defer end()

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	log.Info().Int("insert_count", len(devices)).Msg("📦 Inserting devices to DB")

	docs := make([]interface{}, len(devices))
	for i, d := range devices {
		docs[i] = d
	}

	coll := config.MongoClient.Database("klynx").Collection("camera")
	res, err := coll.InsertMany(ctx, docs)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to insert devices")
		return 0, err
	}

	log.Info().Int("inserted", len(res.InsertedIDs)).Msg("✅ Devices inserted successfully")
	return len(res.InsertedIDs), nil
}
