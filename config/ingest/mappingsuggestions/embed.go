// config/ingest/mappingsuggestions/embed.go
package mappingsuggestions

import "embed"

// FS holds all JSON suggestion files embedded at compile time.
//
//go:embed AIBOX PVS
var FS embed.FS
