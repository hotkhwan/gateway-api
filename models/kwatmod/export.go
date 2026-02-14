// models/kwatmod/export.go
package kwatmod

import "time"

type ExportJobStatus string

const (
	ExportStatusPending   ExportJobStatus = "PENDING"
	ExportStatusRunning   ExportJobStatus = "RUNNING"
	ExportStatusSucceeded ExportJobStatus = "SUCCEEDED"
	ExportStatusFailed    ExportJobStatus = "FAILED"
)

type ExportKind string

const (
	ExportKindWatchlist ExportKind = "watchlist"
)

type ExportWatchlistParams struct {
	Search  string `json:"search,omitempty" query:"search"`
	From    string `json:"from,omitempty"   query:"from"` // YYYY-MM-DD
	To      string `json:"to,omitempty"     query:"to"`   // YYYY-MM-DD
	Limit   int64  `json:"limit,omitempty"  query:"limit"`
	OnlyIds string `json:"onlyIds,omitempty" query:"onlyIds"` // comma separated
}

type ExportResult struct {
	Bucket   string `json:"bucket,omitempty" bson:"bucket,omitempty"`
	Key      string `json:"key,omitempty"    bson:"key,omitempty"`
	Size     int64  `json:"size,omitempty"   bson:"size,omitempty"`
	FileName string `json:"fileName,omitempty" bson:"fileName,omitempty"`
	URL      string `json:"url,omitempty"    bson:"url,omitempty"`
}

type ExportJob struct {
	ID        string            `bson:"_id,omitempty" json:"id"`
	Kind      ExportKind        `bson:"kind"   json:"kind"`
	Status    ExportJobStatus   `bson:"status" json:"status"`
	Params    any               `bson:"params" json:"params"`
	Result    *ExportResult     `bson:"result,omitempty" json:"result,omitempty"`
	Error     string            `bson:"error,omitempty"  json:"error,omitempty"`
	CreatedAt time.Time         `bson:"createdAt" json:"createdAt"`
	StartedAt *time.Time        `bson:"startedAt,omitempty" json:"startedAt,omitempty"`
	EndedAt   *time.Time        `bson:"endedAt,omitempty"  json:"endedAt,omitempty"`
	Meta      map[string]string `bson:"meta,omitempty"   json:"meta,omitempty"`
}
