// internal/services/devicesvc/cameraImportParse.go
package devicesvc

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
)

// ============================================================
// Header normalization — รองรับ typo และ alias
// ============================================================

var cameraHeaderAlias = map[string]string{
	"lat": "lat", "latitude": "lat",
	"long": "long", "lon": "long", "lng": "long",
	"longtitude": "long", "longitude": "long",
	"streamurl": "url", "stream_url": "url", "url": "url",
	"name": "name", "description": "description", "desc": "description",
	"group": "groups", "groups": "groups",
	"ip": "ip", "brand": "brand", "district": "district",
}

var cameraRequiredFields = []string{"name", "url", "lat", "long"}

var ipv4Pattern = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)

func canonCameraKey(s string) string {
	k := strings.ToLower(strings.TrimSpace(s))
	if v, ok := cameraHeaderAlias[k]; ok {
		return v
	}
	return k
}

// ============================================================
// parseFileRaw — entry point
// ============================================================

func parseFileRaw(ctx context.Context, file io.Reader, format string) (
	valid []map[string]interface{},
	dupIPs []string,
	invalidRows []string,
	err error,
) {
	_, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	var rows [][]string

	switch format {
	case "csv":
		rows, err = readCSVRows(file)
	case "xlsx":
		rows, err = readXLSXRows(file)
	default:
		err = errors.New("unsupported format: " + format)
	}
	if err != nil {
		return nil, nil, nil, err
	}
	if len(rows) < 2 {
		return nil, nil, nil, errors.New("no data rows found")
	}

	// normalize headers
	headers := make([]string, len(rows[0]))
	for i, h := range rows[0] {
		headers[i] = canonCameraKey(h)
	}

	ipCount := map[string]int{}

	for i, row := range rows[1:] {
		rowNum := i + 2 // 1=header
		item := map[string]interface{}{}
		for j, val := range row {
			if j < len(headers) && headers[j] != "" {
				item[headers[j]] = strings.TrimSpace(val)
			}
		}

		// ตรวจ required fields
		if hasMissingField(item, cameraRequiredFields) {
			invalidRows = append(invalidRows, fmt.Sprint(rowNum))
			continue
		}

		// coerce lat/long
		if err := coerceCameraLatLong(item); err != nil {
			invalidRows = append(invalidRows, fmt.Sprint(rowNum))
			continue
		}

		// extract IP
		if ip := extractCameraIP(item); ip != "" {
			item["ip"] = ip
			ipCount[ip]++
		}

		valid = append(valid, item)
	}

	// IP ซ้ำภายในไฟล์
	for ip, count := range ipCount {
		if count > 1 {
			dupIPs = append(dupIPs, ip)
		}
	}

	return valid, dupIPs, invalidRows, nil
}

// ============================================================
// readers
// ============================================================

func readCSVRows(file io.Reader) ([][]string, error) {
	r := csv.NewReader(file)
	r.TrimLeadingSpace = true
	r.FieldsPerRecord = -1
	return r.ReadAll()
}

func readXLSXRows(file io.Reader) ([][]string, error) {
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(file); err != nil {
		return nil, err
	}
	xlsx, err := excelize.OpenReader(buf)
	if err != nil {
		return nil, err
	}
	sheetName := xlsx.GetSheetName(0)
	return xlsx.GetRows(sheetName)
}

// ============================================================
// helpers
// ============================================================

func hasMissingField(item map[string]interface{}, required []string) bool {
	for _, field := range required {
		v, ok := item[field]
		if !ok || strings.TrimSpace(fmt.Sprint(v)) == "" {
			return true
		}
	}
	return false
}

func coerceCameraLatLong(item map[string]interface{}) error {
	latRaw, ok1 := item["lat"]
	lonRaw, ok2 := item["long"]
	if !ok1 || !ok2 {
		return errors.New("missing lat/long")
	}
	lat, err := parseAnyFloat(latRaw)
	if err != nil {
		return fmt.Errorf("invalid lat: %w", err)
	}
	lon, err := parseAnyFloat(lonRaw)
	if err != nil {
		return fmt.Errorf("invalid long: %w", err)
	}
	item["lat"] = lat
	item["long"] = lon
	return nil
}

func parseAnyFloat(v interface{}) (float64, error) {
	switch t := v.(type) {
	case float64:
		return t, nil
	case float32:
		return float64(t), nil
	case string:
		s := strings.TrimSpace(strings.ReplaceAll(t, ",", ""))
		return strconv.ParseFloat(s, 64)
	default:
		return strconv.ParseFloat(fmt.Sprint(t), 64)
	}
}

func extractCameraIP(item map[string]interface{}) string {
	if ip, ok := item["ip"].(string); ok && ip != "" {
		return strings.TrimSpace(ip)
	}
	if raw, ok := item["url"].(string); ok && raw != "" {
		if u, err := url.Parse(strings.TrimSpace(raw)); err == nil && u.Host != "" {
			h := u.Host
			if i := strings.IndexByte(h, ':'); i >= 0 {
				h = h[:i]
			}
			if h != "" {
				return h
			}
		}
		if m := ipv4Pattern.FindString(raw); m != "" {
			return m
		}
	}
	return ""
}
