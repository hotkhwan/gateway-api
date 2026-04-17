// models/usrmod/response.go
package usrmod

import "time"

type PaginationSuccessUsers struct {
	Details    []KeycloakUser `json:"details"`
	Pagination struct {
		Page         int    `json:"currentPage"`
		PerPage      int    `json:"perPage"`
		TotalRecords int    `json:"totalRecords"`
		TotalPages   int    `json:"totalPages"`
		SortField    string `json:"sortField"`
		SortOrder    string `json:"sortOrder"`
	} `json:"pagination"`
	Status bool `json:"status"`
}

// Raw ที่อ่านตรงจาก Keycloak /admin/.../users (briefRepresentation=false)
type KeycloakUserRaw struct {
	ID               string              `json:"id"`
	Username         string              `json:"username"`
	FirstName        string              `json:"firstName"`
	LastName         string              `json:"lastName"`
	Email            string              `json:"email"`
	Enabled          bool                `json:"enabled"`
	PerPage          int                 `json:"perPage"`
	Attributes       map[string][]string `json:"attributes"`
	CreatedTimestamp int64               `json:"createdTimestamp"` // ms since epoch
}

// ที่เราส่งให้ frontend ใช้
type KeycloakUser struct {
	ID          string       `json:"id"`
	Username    string       `json:"username"`
	FirstName   string       `json:"firstName"`
	LastName    string       `json:"lastName"`
	Email       string       `json:"email"`
	Enabled     bool         `json:"enabled"`
	Avatar      string       `json:"avatar,omitempty"`
	Locale      string       `json:"locale,omitempty"`
	Role        string       `json:"role,omitempty"`
	Plant       string       `json:"plant,omitempty"`
	PerPage     int          `json:"perPage,omitempty"`
	RefId       string       `json:"refId,omitempty"`
	Department  string       `json:"department,omitempty"`
	RealmRoles  []string     `json:"realmRoles,omitempty"`
	MapLocation *MapLocation `json:"mapLocation,omitempty"`
	ZoomLevel   int          `json:"zoomLevel,omitempty"`
	CreatedAt   *time.Time   `json:"createdAt,omitempty"`
}
