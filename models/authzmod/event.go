package authzmod

// RelationshipEvent ใช้ทั้ง Producer และ Consumer
type RelationshipEvent struct {
	Type       string `json:"type"` // เช่น group_member_added, sensor_relocated
	GroupID    string `json:"group_id,omitempty"`
	UserID     string `json:"user_id,omitempty"`
	ResourceID string `json:"resource_id,omitempty"`
	NewGroupID string `json:"new_group_id,omitempty"`
	Timestamp  int64  `json:"timestamp"` // UnixMilli
}
