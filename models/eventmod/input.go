package eventmod

// EventUpdateInput represents metadata updates for an event
type EventUpdateInput struct {
	Name      string  `json:"name,omitempty"`
	Lat       float64 `json:"lat,omitempty"`
	Lng       float64 `json:"lng,omitempty"`
	EventType string  `json:"eventType,omitempty"`
}

// ListEventsInput represents list events query parameters
type ListEventsInput struct {
	TenantId   string `json:"tenantId"`
	OrgId      string `json:"orgId"`
	StatusName string `json:"statusName"` // "pending", "approved", "rejected", "all"
	EventType  string `json:"eventType"`  // optional filter
	Page       int    `json:"page"`
	PerPage    int    `json:"perPage"`
	SortField  string `json:"sortField"`
	SortOrder  string `json:"sortOrder"` // "asc" or "desc"
}

// PaginatedResult represents paginated query result
type PaginatedResult struct {
	Items       interface{} `json:"items"`
	Total       int64       `json:"total"`
	TotalPages  int         `json:"totalPages"`
	CurrentPage int         `json:"currentPage"`
}
