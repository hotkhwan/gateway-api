// models/kwatmod/watchlist.go
package kwatmod

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ส่ง JSON 200 สำหรับผลลัพธ์แบบ pagination ที่ใช้ generic
func SendPagination[T any](c *fiber.Ctx, data []T, pagination WatchlistPagination) error {
	// ถ้า data เป็น nil บังคับให้เป็น slice ว่าง
	if data == nil {
		data = []T{}
	}
	return c.Status(fiber.StatusOK).JSON(WatchlistPaginationDbResponse[T]{
		Details:    data,
		Pagination: pagination,
		Status:     true,
	})
}

type SyncIbocError struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type SyncIbocSummary struct {
	TraceID string          `json:"traceID,omitempty"`
	Total   int             `json:"total"`
	Updated int             `json:"updated"`
	Skipped int             `json:"skipped"`
	Errors  []SyncIbocError `json:"errors,omitempty"`
}

// SyncIbocSummaryResponse เป็นรูปแบบผลลัพธ์สำหรับ Swagger (แทน generic)
type SyncIbocSummaryResponse struct {
	Details SyncIbocSummary `json:"details"`
	Status  bool            `json:"status"`
}

type WatchlistCreateRequest struct {
	ID           primitive.ObjectID `json:"-"`
	Type         string             `form:"type" validate:"required"`
	PersonalType string             `form:"personalType"`
	CrimesType   string             `form:"crimesType"`
	IdCard       string             `form:"idcard"`
	Passport     string             `form:"passport"`
	AlertType    string             `form:"alertType"`
	AlertDesc    string             `form:"alertDesc"`
	// IdCard           string `form:"idcard" validate:"required_without=Passport"`
	// Passport         string `form:"passport" validate:"required_without=IdCard"`
	TitleName        string `form:"titlename"`
	SubTitleName     string `form:"subTitlename"`
	FirstName        string `form:"firstname" validate:"required"`
	LastName         string `form:"lastname" validate:"required"`
	NickName         string `form:"nickname"`
	Sex              string `form:"sex"`
	Birthday         string `form:"birthday"`
	Age              string `form:"age"`
	FatherName       string `form:"fatherName"`
	FatherIdCard     string `form:"fatherIdcard"`
	MotherName       string `form:"motherName"`
	MotherIdCard     string `form:"motherIdcard"`
	MaritalStatus    string `form:"maritalStatus"`
	DeathStatus      string `form:"deathStatus"`
	DateOfDeath      string `form:"dateOfDeath"`
	PoliceRegion     string `form:"policeRegion"`
	PoliceProvincial string `form:"policeProvincial"`
	PoliceStation    string `form:"policeStation"`
	UserRecorder     string `form:"userRecorder"`
	UserPosition     string `form:"userPosition"`
	Status           string `form:"status"`
	Source           string `form:"source"`
	State            string `form:"state"`
	PhotoFile        []byte `json:"-" validate:"-"`
	PhotoFileName    string `json:"-" validate:"-"`
	PhotoContentType string `json:"-" validate:"-"`
	PhotoURL         string `json:"photoUrl,omitempty"`
	PhotoSource      string `json:"photoSource,omitempty"`
	IsDeleted        bool   `bson:"isDeleted,omitempty" json:"isDeleted"`
}

type Watchlist struct {
	ID      primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TraceID string             `json:"traceID"`
	// ——— ข้อมูลบุคคล ———
	Type             string `bson:"type,omitempty" json:"type"`
	PersonalType     string `bson:"personalType,omitempty" json:"personalType"`
	CrimesType       string `bson:"crimesType,omitempty" json:"crimesType"`
	Country          string `bson:"country,omitempty" json:"country"`
	Nation           string `bson:"nation,omitempty" json:"nation"`
	IdCard           string `bson:"idcard,omitempty" json:"idcard"`
	Passport         string `bson:"passport,omitempty" json:"passport"`
	TitleName        string `bson:"titlename,omitempty" json:"titlename"`
	SubTitleName     string `bson:"subTitlename,omitempty" json:"subTitlename"`
	FirstName        string `bson:"firstname,omitempty" json:"firstname"`
	LastName         string `bson:"lastname,omitempty" json:"lastname"`
	NickName         string `bson:"nickname,omitempty" json:"nickname"`
	TitleEn          string `bson:"titleEn,omitempty" json:"titleEn,omitempty"`
	FirstNameEn      string `bson:"firstNameEn,omitempty" json:"firstNameEn,omitempty"`
	LastNameEn       string `bson:"lastNameEn,omitempty" json:"lastNameEn,omitempty"`
	FullName         string `bson:"fullName,omitempty" json:"fullName,omitempty"`
	Sex              string `bson:"sex,omitempty" json:"sex"`
	Birthday         string `bson:"birthday,omitempty" json:"birthday"`
	Age              string `bson:"age,omitempty" json:"age"`
	FatherName       string `bson:"fatherName,omitempty" json:"fatherName"`
	FatherIdCard     string `bson:"fatherIdcard,omitempty" json:"fatherIdcard"`
	MotherName       string `bson:"motherName,omitempty" json:"motherName"`
	MotherIdCard     string `bson:"motherIdcard,omitempty" json:"motherIdcard"`
	MaritalStatus    string `bson:"maritalStatus,omitempty" json:"maritalStatus"`
	DeathStatus      string `bson:"deathStatus,omitempty" json:"deathStatus"`
	DateOfDeath      string `bson:"dateOfDeath,omitempty" json:"dateOfDeath"`
	PoliceRegion     string `bson:"policeRegion,omitempty" json:"policeRegion"`
	PoliceProvincial string `bson:"policeProvincial,omitempty" json:"policeProvincial"`
	PoliceStation    string `bson:"policeStation,omitempty" json:"policeStation"`
	UserRecorder     string `bson:"userRecorder,omitempty" json:"userRecorder"`
	UserPosition     string `bson:"userPosition,omitempty" json:"userPosition"`
	AlertType        string `bson:"alertType,omitempty" json:"alertType"`
	AlertDesc        string `bson:"alertDesc,omitempty" json:"alertDesc"`

	// ——— ไฟล์ ———
	PhotoKey string `bson:"photoKey,omitempty" json:"photoKey,omitempty"`
	// legacy (ถ้ายังมีข้อมูลเก่า)
	PhotoURL     string `bson:"photoUrl,omitempty" json:"photoUrl,omitempty"`
	PhotoChanged bool   `json:"photoChanged,omitempty" bson:"photoChanged,omitempty"`

	// ——— เมตา ———
	State     string     `bson:"state,omitempty" json:"state"` // active|deleted
	CreatedAt time.Time  `bson:"createdAt,omitempty" json:"createdAt"`
	UpdatedAt time.Time  `bson:"updatedAt,omitempty" json:"updatedAt"`
	DeletedAt *time.Time `bson:"deletedAt,omitempty" json:"deletedAt,omitempty"`
	Rev       int64      `bson:"rev,omitempty" json:"rev,omitempty"`

	// ——— external (ถ้ามี ใช้ map[string]any ไปก่อน) ———
	External map[string]any `bson:"external,omitempty" json:"external,omitempty"`
}

// ✅ เพิ่ม method แปลงเป็น Mongo object
func (r WatchlistCreateRequest) ToWatchlist(id primitive.ObjectID, photoURL string, createdAt time.Time) Watchlist {
	return Watchlist{
		ID:               id,
		Type:             r.Type,
		PersonalType:     r.PersonalType,
		CrimesType:       r.CrimesType,
		IdCard:           r.IdCard,
		Passport:         r.Passport,
		TitleName:        r.TitleName,
		SubTitleName:     r.SubTitleName,
		FirstName:        r.FirstName,
		LastName:         r.LastName,
		NickName:         r.NickName,
		Sex:              r.Sex,
		Birthday:         r.Birthday,
		Age:              r.Age,
		FatherName:       r.FatherName,
		FatherIdCard:     r.FatherIdCard,
		MotherName:       r.MotherName,
		MotherIdCard:     r.MotherIdCard,
		MaritalStatus:    r.MaritalStatus,
		DeathStatus:      r.DeathStatus,
		DateOfDeath:      r.DateOfDeath,
		PoliceRegion:     r.PoliceRegion,
		PoliceProvincial: r.PoliceProvincial,
		PoliceStation:    r.PoliceStation,
		UserRecorder:     r.UserRecorder,
		UserPosition:     r.UserPosition,
		PhotoURL:         photoURL,
		CreatedAt:        createdAt,
		AlertType:        r.AlertType,
		AlertDesc:        r.AlertDesc,
	}
}

// WatchlistResponse represents perwatchlistson document returned to API
type WatchlistResponse struct {
	ID               string `bson:"_id,omitempty" json:"id"`
	PersonKey        string `bson:"personKey,omitempty" json:"personKey"`
	TraceID          string `bson:"traceID,omitempty" json:"traceID"`
	Type             int    `bson:"type,omitempty" json:"type"`
	PersonalType     int    `bson:"personalType,omitempty" json:"personalType"`
	CrimesType       int    `bson:"crimesType,omitempty" json:"crimesType"`
	Country          string `bson:"country,omitempty" json:"country"`
	Nation           string `bson:"nation,omitempty" json:"nation"`
	IdCard           string `bson:"idcard,omitempty" json:"idcard"`
	Passport         string `bson:"passport,omitempty" json:"passport"`
	TitleName        string `bson:"titlename,omitempty" json:"titlename"`
	SubTitleName     string `bson:"subTitlename,omitempty" json:"subTitlename"`
	FirstName        string `bson:"firstname,omitempty" json:"firstname"`
	LastName         string `bson:"lastname,omitempty" json:"lastname"`
	NickName         string `bson:"nickname,omitempty" json:"nickname"`
	TitleEn          string `bson:"titleEn,omitempty" json:"titleEn,omitempty"`
	FirstNameEn      string `bson:"firstNameEn,omitempty" json:"firstNameEn,omitempty"`
	LastNameEn       string `bson:"lastNameEn,omitempty" json:"lastNameEn,omitempty"`
	FullName         string `bson:"fullName,omitempty" json:"fullName,omitempty"`
	Sex              string `bson:"sex,omitempty" json:"sex"`
	Birthday         string `bson:"birthday,omitempty" json:"birthday"`
	Age              int    `bson:"age,omitempty" json:"age"`
	FatherName       string `bson:"fatherName,omitempty" json:"fatherName"`
	FatherIdCard     string `bson:"fatherIdcard,omitempty" json:"fatherIdcard"`
	MotherName       string `bson:"motherName,omitempty" json:"motherName"`
	MotherIdCard     string `bson:"motherIdcard,omitempty" json:"motherIdcard"`
	MaritalStatus    string `bson:"maritalStatus,omitempty" json:"maritalStatus"`
	DeathStatus      int    `bson:"deathStatus,omitempty" json:"deathStatus"`
	DateOfDeath      string `bson:"dateOfDeath,omitempty" json:"dateOfDeath"`
	PoliceRegion     int    `bson:"policeRegion,omitempty" json:"policeRegion"`
	PoliceProvincial int    `bson:"policeProvincial,omitempty" json:"policeProvincial"`
	PoliceStation    int    `bson:"policeStation,omitempty" json:"policeStation"`
	UserRecorder     string `bson:"userRecorder,omitempty" json:"userRecorder"`
	UserPosition     string `bson:"userPosition,omitempty" json:"userPosition"`
	PhotoKey         string `json:"photoKey,omitempty"`
	PhotoURL         string `json:"photoUrl,omitempty"`
	PhotoFileName    string `json:"photoFileName,omitempty"`
	PhotoContentType string `json:"photoContentType,omitempty"`
	AlertType        string `json:"alertType"`
	AlertDesc        string `json:"alertDesc"`

	// เมตาเพิ่ม
	State     string    `json:"state,omitempty"`
	Rev       int64     `json:"rev,omitempty"` // ✅ เพิ่มตรงนี้
	CreatedAt time.Time `bson:"createdAt,omitempty" json:"createdAt"`
	UpdatedAt time.Time `bson:"updatedAt,omitempty" json:"updatedAt"`
	// DeletedAt time.Time      `bson:"deletedAt,omitempty" json:"deletedAt,omitempty"`
	External       map[string]any `bson:"external,omitempty" json:"external,omitempty"`
	Source         string         `bson:"source,omitempty" json:"source"`
	Warrants       any            `bson:"warrants,omitempty" json:"warrants,omitempty"`
	WarrantsPerson int            `json:"warrantsPerson" bson:"-"`
}

// WatchlistUpdateRequest ใช้สำหรับอัปเดต watchlist
type WatchlistUpdateRequest struct {
	Type             *string    `json:"type,omitempty"`
	PersonalType     *string    `json:"personalType,omitempty"`
	CrimesType       *string    `json:"crimesType,omitempty"`
	IdCard           *string    `json:"idcard,omitempty"`
	Passport         *string    `json:"passport,omitempty"`
	TitleName        *string    `json:"titlename,omitempty"`
	SubTitleName     *string    `json:"subTitlename,omitempty"`
	FirstName        *string    `json:"firstname,omitempty"`
	LastName         *string    `json:"lastname,omitempty"`
	NickName         *string    `json:"nickname,omitempty"`
	Sex              *string    `json:"sex,omitempty"`
	Birthday         *string    `json:"birthday,omitempty"`
	Age              *string    `json:"age,omitempty"`
	FatherName       *string    `json:"fatherName,omitempty"`
	FatherIdCard     *string    `json:"fatherIdcard,omitempty"`
	MotherName       *string    `json:"motherName,omitempty"`
	MotherIdCard     *string    `json:"motherIdcard,omitempty"`
	MaritalStatus    *string    `json:"maritalStatus,omitempty"`
	DeathStatus      *string    `json:"deathStatus,omitempty"`
	DateOfDeath      *string    `json:"dateOfDeath,omitempty"`
	PoliceRegion     *string    `json:"policeRegion,omitempty"`
	PoliceProvincial *string    `json:"policeProvincial,omitempty"`
	PoliceStation    *string    `json:"policeStation,omitempty"`
	UserRecorder     *string    `json:"userRecorder,omitempty"`
	UserPosition     *string    `json:"userPosition,omitempty"`
	Status           *string    `json:"status,omitempty"`
	State            *string    `json:"state,omitempty"`
	UpdatedAt        *time.Time `json:"updatedAt,omitempty"`
	AlertType        *string    `json:"alertType"`
	AlertDesc        *string    `json:"alertDesc"`

	PhotoFile     []byte `json:"-"` // optional
	PhotoFileName string `json:"-"`
}

type WatchlistRow struct {
	IDCard           string
	FullName         string
	Titlename        string
	Sex              string
	Nation           string
	Source           string
	CrimesType       int
	AlertDesc        string
	PoliceRegion     string
	PoliceProvincial string
	PoliceStation    string
	WarrantNoYear    string
	WarrantOrg       string
	WarrantStatus    string
	FirstSeenAt      string
	UpdatedAt        string
	ExternalWatchman string
	ExternalIboC     string
	ExternalIboCDev  string
	PhotoFaceKey     string
	PhotoOriginKey   string
}

type WatchlistPagination struct {
	Page            int    `json:"page"`
	PerPages        int    `json:"perPages"`
	TotalRecords    int    `json:"totalRecords"`
	TotalPages      int    `json:"totalPages"`
	SortField       string `json:"sortField"`
	SortOrder       string `json:"sortOrder"`
	TotalFaceImages int    `json:"totalFaceImages,omitempty"`
	TotalPersons    int    `json:"totalPersons,omitempty"`
	TotalWarrants   int    `json:"totalWarrants,omitempty"`
}

type WatchlistPaginationDbResponse[T any] struct {
	Details    []T                 `json:"details"` // 👈 ใช้ slice ตรงๆ (ไม่เป็น pointer)
	Pagination WatchlistPagination `json:"pagination"`
	Status     bool                `json:"status"`
}
