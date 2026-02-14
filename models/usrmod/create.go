// models/usrmod/create.go
package usrmod

type CreateUserRequest struct {
	Username    string      `json:"username"`
	FirstName   string      `json:"firstName"`
	LastName    string      `json:"lastName"`
	Email       string      `json:"email"`
	Role        string      `json:"role"`
	Avatar      string      `json:"avatar"`
	Department  string      `json:"department"`
	RefId       string      `json:"refId"`
	Locale      string      `json:"locale"`
	Password    string      `json:"password"`
	Permission  []string    `json:"permission"`
	MapLocation MapLocation `json:"mapLocation"`
	ZoomLevel   int         `json:"zoomLevel"`
	PerPages    int         `json:"perPages"`
	Enabled     bool        `json:"enabled"`
}
