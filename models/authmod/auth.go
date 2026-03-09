// models/authmod/auth.go
package authmod

type Allow string

const (
	AllowRead   Allow = "r"
	AllowCreate Allow = "c"
	AllowUpdate Allow = "u"
	AllowDelete Allow = "d"
	AllowCU     Allow = "cu"
	AllowCRUD   Allow = "curd"
)

type SigninRequest struct {
	Username string `json:"username" example:"admin" binding:"required"`
	Password string `json:"password" example:"admin" binding:"required"`
	HwID     string `json:"hwId,omitempty" example:""`
}

type CRUDGrant struct {
	Read   bool `json:"read"`
	Create bool `json:"create"`
	Update bool `json:"update"`
	Delete bool `json:"delete"`
}

type MenuGrant struct {
	ID    string    `json:"id"`
	Grant CRUDGrant `json:"-"`
	// flatten fields for FE
	Read   bool `json:"read"`
	Create bool `json:"create"`
	// Update bool `json:"update"`
	// Delete bool `json:"delete"`
}

// ✅ เปลี่ยนจาก map → array
type PermissionsBlock struct {
	Menus []MenuGrant `json:"menus"`
	// (ทางเลือก) ชั่วคราวช่วย migration:
	// MenusByID map[string]CRUDGrant `json:"menus_by_id,omitempty"`
}

type SigninResponse struct {
	Sub            string            `json:"sub"`
	Username       string            `json:"username"`
	Name           string            `json:"name"`
	Email          string            `json:"email"`
	Role           string            `json:"role"`
	Permissions    *PermissionsBlock `json:"permissions,omitempty"`
	Meta           map[string]string `json:"meta,omitempty"`
	Locale         string            `json:"locale"`
	Plant          any               `json:"plant"`
	AccessToken    string            `json:"access_token"`
	RefreshToken   string            `json:"refresh_token"`
	StationID      string            `json:"stationId,omitempty"`
	StationName    string            `json:"stationName,omitempty"`
	StationHwId    string            `json:"stationHwId,omitempty"`
	StationType    string            `json:"stationType,omitempty"`
	StationAddress string            `json:"stationAddress,omitempty"`
	StationCamera  string            `json:"stationCameraUrl,omitempty"`
}

type SigninResponseWrapper struct {
	Details SigninResponse `json:"details"`
	Status  bool           `json:"status"`
}

type IntrospectResult struct {
	Active            bool   `json:"active"`
	Sub               string `json:"sub"`
	Username          string `json:"username"`
	PreferredUsername string `json:"preferred_username,omitempty"`
	GivenName         string `json:"given_name,omitempty"`
	FamilyName        string `json:"family_name,omitempty"`
	Avatar            string `json:"avatar,omitempty"`
	Email             string `json:"email"`
	Locale            string `json:"locale"`
	Exp               int64  `json:"exp"`
	Scope             string `json:"scope"`
	Role              string `json:"role,omitempty"`
	//Permissions       []Permission `json:"permissions,omitempty"` // ← introspect ใช้ของมันต่อ (ไม่เกี่ยวกับ SigninResponse)
	Permissions *PermissionsBlock `json:"permissions,omitempty"`
	ZoomLevel   int               `json:"zoom_level,omitempty"`
	MapLocation *MapLocation      `json:"map_location,omitempty"`
}

type MapLocation struct {
	Lat float64 `json:"lat,omitempty"`
	Lng float64 `json:"lng,omitempty"`
}

type Permission struct {
	Resource string `json:"resource"`
	Allow    Allow  `json:"allow"`
}

type MenuOption struct {
	ID    string `bson:"id"`
	Title string `bson:"title"`
}

type MenuListDoc struct {
	ID   string       `bson:"_id"`
	Menu []MenuOption `bson:"menu"`
}

type OAuthCallbackRequest struct {
	Code        string `json:"code"`
	RedirectUri string `json:"redirectUri"`
	HwID        string `json:"hwId,omitempty"`
}
