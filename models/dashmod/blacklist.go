// models/dashmod/event.go
package dashmod

type UpdateBlacklistReq struct {
	ID           string         `json:"-"`
	Acknowledged *bool          `json:"acknowledged,omitempty"`
	IsDeleted    *bool          `json:"isDeleted,omitempty"`
	Sop          map[string]any `json:"sop,omitempty"`
}
