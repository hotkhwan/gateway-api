// models/kwatmod/event.go
package kwatmod

import "time"

type WatchlistEvent struct {
	ID      string `json:"id"`
	Event   string `json:"event"`
	TraceID string `json:"traceId"`
	Time    string `json:"time"`
	Rev     int64  `json:"rev"`
	State   string `json:"state"`

	PhotoKey               string `json:"photoKey,omitempty"`
	PhotoContentType       string `json:"photoContentType,omitempty"`
	PhotoOriginKey         string `json:"photoOriginKey,omitempty"`
	PhotoOriginContentType string `json:"photoOriginContentType,omitempty"`
	PhotoFaceKey           string `json:"photoFaceKey,omitempty"`

	// (NEW) เผื่อโค้ดอื่นอ้างอิง (เช่น aiutil/ibocmapper.go)
	PhotoURL      string `json:"photoURL,omitempty"`
	PhotoFileName string `json:"photoFileName,omitempty"`

	// INT-like fields
	Type             int `json:"type,omitempty"`
	PersonalType     int `json:"personalType,omitempty"`
	CrimesType       int `json:"crimesType,omitempty"`
	Age              int `json:"age,omitempty"`
	DeathStatus      int `json:"deathStatus,omitempty"`
	PoliceRegion     int `json:"policeRegion,omitempty"`
	PoliceProvincial int `json:"policeProvincial,omitempty"`
	PoliceStation    int `json:"policeStation,omitempty"`

	// STRING fields
	IdCard        string `json:"idcard,omitempty"`
	PersonKey     string `json:"personKey,omitempty"` // ⬅️ เพิ่ม
	Passport      string `json:"passport,omitempty"`
	TitleName     string `json:"titlename,omitempty"`
	SubTitleName  string `json:"subTitlename,omitempty"`
	FirstName     string `json:"firstname,omitempty"`
	LastName      string `json:"lastname,omitempty"`
	NickName      string `json:"nickname,omitempty"`
	Sex           string `json:"sex,omitempty"`
	Birthday      string `json:"birthday,omitempty"`
	FatherName    string `json:"fatherName,omitempty"`
	FatherIdCard  string `json:"fatherIdcard,omitempty"`
	MotherName    string `json:"motherName,omitempty"`
	MotherIdCard  string `json:"motherIdcard,omitempty"`
	MaritalStatus string `json:"maritalStatus,omitempty"`
	DateOfDeath   string `json:"dateOfDeath,omitempty"`
	UserRecorder  string `json:"userRecorder,omitempty"`
	UserPosition  string `json:"userPosition,omitempty"`
	AlertType     string `json:"alertType,omitempty"`
	AlertDesc     string `json:"alertDesc,omitempty"`

	External    map[string]any `json:"external,omitempty"`
	IBOCTop     ExternalNS     `json:"iboc,omitempty"`
	IBOCDevTop  ExternalNS     `json:"ibocDev,omitempty"`
	WatchmanTop struct {
		ID     string `json:"id,omitempty"`
		IDCard string `json:"idCard,omitempty"`
	} `json:"watchman,omitempty"`
	// (NEW) ใช้ส่งชื่อสถานีตำรวจ (string) ให้ handler กรณีไม่มี int code
	StationTitleFallback string `json:"stationTitleFallback,omitempty"`
}

type CreateWatchlistIBOCRequest struct {
	FirstName     string                 `json:"firstName"` // required by IBOC
	LastName      string                 `json:"lastName,omitempty"`
	Email         string                 `json:"email,omitempty"`
	IdentityDocId string                 `json:"identityDocId,omitempty"`
	Organization  string                 `json:"organization,omitempty"`
	Enabled       bool                   `json:"enabled,omitempty"`
	Tags          []string               `json:"tags,omitempty"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// -------- External (ต่อเนมสเปซเดียว) --------
// ใช้กับฟังก์ชันอย่าง SetExternalNS / MarkExternalState ที่เขียน external.<ns>.* โดยตรง
type ExternalNS struct {
	ID       string    `bson:"id,omitempty" json:"id,omitempty"`
	FaceID   string    `bson:"faceId,omitempty" json:"faceId,omitempty"`
	State    string    `bson:"state,omitempty" json:"state,omitempty"`       // "active" | "updated" | "deleted" | "error"
	SyncedAt time.Time `bson:"syncedAt,omitempty" json:"syncedAt,omitempty"` // เวลา sync ล่าสุด
}

// เพื่อความเข้ากันได้กับโค้ดเดิมที่อ้าง kwatmod.ExternalRef (เช่น externalHelper.go)
type ExternalRef = ExternalNS

// -------- External (รวมทุกเนมสเปซที่เราดูแล) --------
type ExternalAll struct {
	IBOC    ExternalNS `bson:"iboc,omitempty" json:"iboc,omitempty"`
	IBOCDev ExternalNS `bson:"ibocdev,omitempty" json:"ibocdev,omitempty"`
	// เพิ่มเนมสเปซอื่นในอนาคตได้ เช่น Watchman ถ้าต้องการเก็บเป็น object
}
