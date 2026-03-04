package ingestmod

import "time"

// IngestDashboardStats สรุปสถิติสำหรับ ingest dashboard
type IngestDashboardStats struct {
	PendingEvents  int `json:"pendingEvents"`
	ApprovedEvents int `json:"approvedEvents"`
	RejectedEvents int `json:"rejectedEvents"`
	TotalEvents    int `json:"totalEvents"`

	ByEventType map[string]int `json:"byEventType"`
	ByPriority  map[string]int `json:"byPriority"`

	Geo          GeoMeta         `json:"geo"`
	GeoCell      GeoCellMeta     `json:"geoCell"`
	ByAdminArea1 map[string]int  `json:"byAdminArea1"`
	ByGeoCell    []GeoCellBucket `json:"byGeoCell"`

	RecentEvents []RecentEventSummary `json:"recentEvents"`

	// DateTime range ที่ใช้งาน (RFC3339)
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

// GeoMeta ข้อมูลเมตาข้อมูลภูมิศาสตร์
type GeoMeta struct {
	CountryCode string `json:"countryCode"` // TH
	AdminLevel  int    `json:"adminLevel"`  // 1
	IdScheme    string `json:"idScheme"`    // ISO_3166_2
}

// GeoCellMeta ข้อมูลเมตาของ geo cell scheme
type GeoCellMeta struct {
	Scheme    string `json:"scheme"`    // geohash
	Precision int    `json:"precision"` // 5
}

// GeoCellBucket ข้อมูลสถิติตาม geo cell
type GeoCellBucket struct {
	Cell  string  `json:"cell"` // wx4g0
	Count int     `json:"count"`
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
}

// RecentEventSummary สรุปข้อมูล event ล่าสุด
type RecentEventSummary struct {
	ID        string    `json:"id"`
	EventId   string    `json:"eventId"`
	Name      string    `json:"name"`
	EventType string    `json:"eventType"`
	Status    string    `json:"status"` // pending/approved/rejected
	Priority  string    `json:"priority,omitempty"`
	CreatedAt time.Time `json:"createdAt"`

	Lat float64 `json:"lat,omitempty"`
	Lng float64 `json:"lng,omitempty"`

	SourceIp string `json:"sourceIp,omitempty"`
}

// GetIngestDashboardInput input parameters สำหรับ get ingest dashboard
type GetIngestDashboardInput struct {
	TenantId  string `json:"-"`
	OrgId     string `json:"-"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
	Status    string `json:"status"`
	EventType string `json:"eventType"`
}
