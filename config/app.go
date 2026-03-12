// config/app.go
package config

import "os"

type AppConfig struct {
	AppName    string
	AppVersion string
}

func LoadAppConfig() AppConfig {
	version := os.Getenv("APP_VERSION")
	if version == "" {
		version = "dev"
	}

	return AppConfig{
		AppName:    "gateway",
		AppVersion: version,
	}
}
