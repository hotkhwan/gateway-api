// config/ataconf.go
package config

import (
	"os"
	"strings"
)

// ATAConfig: เก็บ config ทั้งหมดที่เกี่ยวกับ ATA Edge AI
type ATAConfig struct {
	Username    string
	Password    string
	APIURL      string
	APIKey      string
	APISecret   string
	InsecureTLS bool
}

// ATA: instance global ให้ service อื่นเรียกใช้ได้เลย
var ATA ATAConfig

// โหลดค่าจาก ENV ตอน start service (เรียกจาก main.go)
func LoadATAConfigFromEnv() {
	ATA = ATAConfig{
		Username:    os.Getenv("ATA_USERNAME"),
		Password:    os.Getenv("ATA_PASSWORD"),
		APIURL:      strings.TrimRight(os.Getenv("ATA_API_URL"), "/"),
		APIKey:      os.Getenv("ATA_API_KEY"),
		APISecret:   os.Getenv("ATA_API_SECRET"),
		InsecureTLS: parseEnvBool(os.Getenv("ATA_INSECURE_TLS")),
	}
}

// รองรับ true/false, 1/0, yes/no (case-insensitive)
func parseEnvBool(v string) bool {
	v = strings.TrimSpace(strings.ToLower(v))
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}
