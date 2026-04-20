// models/aimodel/atamod.go
package aimodel

import (
	"encoding/json"
)

// ATAMessage: dynamic map ใช้กับ webhook พวก /atahook
type ATAMessage = map[string]any

// PusherEvent: envelope สำหรับ /atahook และ /atahook/pusher-event
type PusherEvent struct {
	Event   string          `json:"event"`
	Channel string          `json:"channel,omitempty"`
	Data    json.RawMessage `json:"data"`
	TimeMs  int64           `json:"time_ms,omitempty"`
}

/* ================================
 *  REST API models (Login / Device / Channel)
 * ================================ */

// Generic response จาก ATA API
type ATAResponse[T any] struct {
	ErrCode int    `json:"errCode"`
	ErrMsg  string `json:"errMsg"`
	Data    T      `json:"data"`
}

// ----- Login -----

type ATALoginRequest struct {
	Name     string `json:"name"`
	Password string `json:"password"`
}

type ATALoginData struct {
	ID           int    `json:"id"`
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
	Role         int    `json:"role"`
	Name         string `json:"name"`
}

// ----- Device (Edge box) -----

type ATADevice struct {
	ID         int64  `json:"id"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
	IP         string `json:"ip"`
	Name       string `json:"name"`
	SN         string `json:"sn"`
	Address    string `json:"address"`
	Longitude  string `json:"longitude"`
	Latitude   string `json:"latitude"`
	GB28181ID  string `json:"gb28181Id"`
	Online     bool   `json:"online"`
	Type       string `json:"type"`
	SWVersion  string `json:"swVersion"`
	IsCheck    int    `json:"isCheck"`
	IsEdit     int    `json:"isEdit"`
	IsCheckCh  int    `json:"isCheckChannel"`
	IsRegister bool   `json:"isRegister"`
}

// GET /api/v1/device/get-devices body
type ATADeviceListRequest struct {
	PageIndex int    `json:"pageIndex"`
	PageSize  int    `json:"pageSize"`
	Name      string `json:"name,omitempty"`
	SN        string `json:"sn,omitempty"`
}

// resp data.devices[]
type ATADeviceListData struct {
	Total     int         `json:"total"`
	PageIndex int         `json:"pageIndex"`
	PageSize  int         `json:"pageSize"`
	Devices   []ATADevice `json:"devices"`
	List      []ATADevice `json:"list"` // บางเวอร์ชันใช้ list
}

// ----- Channel -----

type ATAChannel struct {
	ID        int64      `json:"id"`
	CreatedAt int64      `json:"createdAt"`
	UpdatedAt int64      `json:"updatedAt"`
	Name      string     `json:"name"`
	MainUrl   string     `json:"mainUrl"`
	Zone      string     `json:"zone"`
	Enable    int        `json:"enable"`
	GB28181ID string     `json:"gb28181Id"`
	Longitude string     `json:"longitude"`
	Latitude  string     `json:"latitude"`
	DeviceId  int64      `json:"deviceId"`
	Device    *ATADevice `json:"device,omitempty"`
	Width     int        `json:"width"`
	Height    int        `json:"height"`
	Codec     int        `json:"codec"`
	Tasks     []int      `json:"tasks"`
	TaskInfos []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"taskInfos"`
	IsCheck int    `json:"isCheck"`
	IsEdit  int    `json:"isEdit"`
	SN      string `json:"sn"`
}

// POST /api/v1/channel/get-channels body
type ATAChannelListRequest struct {
	PageIndex int    `json:"pageIndex"`
	PageSize  int    `json:"pageSize"`
	DeviceId  int64  `json:"deviceId,omitempty"`
	Name      string `json:"name,omitempty"`
}

// resp data.list[]
type ATAChannelListData struct {
	Total     int          `json:"total"`
	PageIndex int          `json:"pageIndex"`
	PageSize  int          `json:"pageSize"`
	List      []ATAChannel `json:"list"`
}

// POST /api/v1/channel/get-channel body
type ATAChannelDetailRequest struct {
	ID int64 `json:"id"`
}

// pagination ใช้โครงเดียวกับ devmod ได้ แต่แยกไว้เพื่อไม่ผูก package กัน
type Pagination struct {
	Page         int    `json:"page"`
	PerPage      int    `json:"perPage"`
	TotalRecords int64  `json:"totalRecords"`
	TotalPages   int64  `json:"totalPages"`
	SortField    string `json:"sortField"`
	SortOrder    string `json:"sortOrder"`
}

type PeopleCountingSummary struct {
	Total int64 `json:"total"`
	In    int64 `json:"in"`
	Out   int64 `json:"out"`
}

type PeopleCountingItem struct {
	ID        string `json:"id"`
	SN        string `json:"sn,omitempty"`
	ChannelID int64  `json:"channelId,omitempty"`
	DateTime  string `json:"dateTime,omitempty"`

	Direction   string   `json:"direction,omitempty"`
	RegionNames []string `json:"regionNames,omitempty"`

	// ✅ ใช้ single image
	Timestamp string `json:"timestamp"`
	ImageUrl  string `json:"imageUrl"`

	// Metadata
	Lat                   string `json:"lat,omitempty"`
	Lng                   string `json:"lng,omitempty"`
	RegionRois            []any  `json:"regionRois,omitempty"`
	PictureCoordinates    []any  `json:"pictureCoordinates,omitempty"`
	Zone                  string `json:"zone,omitempty"`
	CameraName            string `json:"cameraName,omitempty"`
	Type                  string `json:"type,omitempty"`
	EventAttributeDetails any    `json:"eventAttributeDetails,omitempty"`
}

type PeopleCountingSummaryResponse struct {
	Details    []PeopleCountingItem  `json:"details"`
	Pagination Pagination            `json:"pagination"`
	Summary    PeopleCountingSummary `json:"summary"`
	Status     bool                  `json:"status"`
}

// ... (ของเดิมทั้งหมดเหมือนเดิม)

type PeopleCountingSummaryReq struct {
	DateTime  string `json:"dateTime"`
	SN        string `json:"sn"`
	ChannelID int64  `json:"channelId"`
	Search    string `json:"search"`

	Direction     string `json:"direction"`     // legacy single/regex
	DirectionCode int64  `json:"directionCode"` // 1=in 2=out (override)

	// ✅ multi-select
	Cameras    []string `json:"cameras"`    // camera=cam1,cam2 หรือ camera=cam1&camera=cam2
	Directions []string `json:"directions"` // direction=in,out หรือ direction=in&direction=out

	Page      int    `json:"page"`
	PerPage   int    `json:"perPage"`
	SortOrder string `json:"sortOrder"`
}

type BlacklistSummaryReq struct {
	DateTime  string `json:"dateTime,omitempty"`
	SN        string `json:"sn,omitempty"`
	ChannelID int64  `json:"channelId,omitempty"`
	Search    string `json:"search,omitempty"` // search by cameraName (address) regex
	ListType  int    `json:"listType,omitempty"`
	ListName  string `json:"listName,omitempty"`
	Page      int    `json:"page,omitempty"`
	PerPage   int    `json:"perPage,omitempty"`
	SortOrder string `json:"sortOrder,omitempty"`
}

type BlacklistSummary struct {
	Total     int64 `json:"total"`
	Blacklist int64 `json:"blacklist,omitempty"`
	Whitelist int64 `json:"whitelist,omitempty"`
	Redlist   int64 `json:"redlist,omitempty"`
	Other     int64 `json:"other,omitempty"` // เผื่อมีค่าอื่น
}

type BlacklistItem struct {
	ID        string `json:"id"`
	SN        string `json:"sn"`
	ChannelID int64  `json:"channelId"`

	DateTime  string `json:"dateTime"`  // เดิม
	Timestamp string `json:"timestamp"` // ✅ ใหม่ (RFC3339)

	CameraName string `json:"cameraName"`
	ImageUrl   string `json:"imageUrl"` // ✅ เปลี่ยนเป็น string

	Lat string `json:"lat"` // ✅ ใหม่
	Lng string `json:"lng"` // ✅ ใหม่

	PictureCoordinates    []any `json:"pictureCoordinates,omitempty"`    // ✅ ใหม่
	EventAttributeDetails any   `json:"eventAttributeDetails,omitempty"` // ✅ ใหม่

	Zone string `json:"zone"`
	Type string `json:"type"`

	Name            string  `json:"name"`
	ListType        any     `json:"listType"`
	Similarity      float64 `json:"similarity"`
	IDCard          string  `json:"idCard"`
	FeatureImageUrl string  `json:"featureImageUrl"`
}

type BlacklistSummaryResponse struct {
	Details    []BlacklistItem  `json:"details"`
	Pagination Pagination       `json:"pagination"`
	Summary    BlacklistSummary `json:"summary"`
	Status     bool             `json:"status"`
	Message    string           `json:"message,omitempty"`
}

// ✅ response wrapper ตามมาตรฐานคุณ
type StandardResponse struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Status  bool        `json:"status"`
	Details interface{} `json:"details,omitempty"`
}

type NotificationAllReq struct {
	DateTime  string `json:"dateTime"`
	SN        string `json:"sn"`
	ChannelID int64  `json:"channelId"`
	Search    string `json:"search"`
	Page      int    `json:”page”`
	PerPage   int    `json:”perPage”`
	SortOrder string `json:”sortOrder”`
}

// summary แบบ “นับตาม type” (เพราะเราไม่จำกัด type แล้ว)
type NotificationTypeCount struct {
	Type  string `json:"type"`
	Count int64  `json:"count"`
}

type NotificationAllSummary struct {
	Total  int64                   `json:"total"`
	ByType []NotificationTypeCount `json:"byType"`
}

type NotificationAllData struct {
	Summary    NotificationAllSummary `json:"summary"`
	Pagination Pagination             `json:"pagination"`
	Details    []BlacklistItem        `json:"details"`
}
