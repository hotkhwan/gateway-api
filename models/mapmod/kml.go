// models/mapmod/kml.go
package mapmod

import "time"

type KMLFile struct {
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	Size         FileSize  `json:"size"`
	FileType     string    `json:"fileType"`
	Description  string    `json:"description"`
	LastModified time.Time `json:"lastModified"`
}

type FileSize struct {
	Bytes     int64  `json:"bytes"`
	Formatted string `json:"formatted"`
}
