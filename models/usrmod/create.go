// models/usrmod/create.go
package usrmod

type CreateUserRequest struct {
	Username    string      `json:"username"`
	FirstName   string      `json:"firstName,omitempty"`
	LastName    string      `json:"lastName,omitempty"`
	Email       string      `json:"email,omitempty"`
	Role        string      `json:"role,omitempty"`
	Avatar      string      `json:"avatar,omitempty"`
	Department  string      `json:"department,omitempty"`
	RefId       string      `json:"refId,omitempty"`
	Locale      string      `json:"locale,omitempty"`
	Password    string      `json:"password"`
	Permission  []string    `json:"permission,omitempty"`
	MapLocation MapLocation `json:"mapLocation,omitempty"`
	ZoomLevel   int         `json:"zoomLevel,omitempty"`
	PerPage     int         `json:"perPage,omitempty"`
	Enabled     bool        `json:"enabled"`
}
