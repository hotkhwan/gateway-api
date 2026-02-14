// models/crimod.go
package models

import "time"

type Warrant struct {
	WarrantNo        string     `bson:"warrantNo,omitempty"`
	WarrantYear      string     `bson:"warrantYear,omitempty"`
	WarrantOrgCode   string     `bson:"warrantOrgCode,omitempty"`
	WarrantOrg       string     `bson:"warrantOrg,omitempty"`
	CrimeNo          string     `bson:"crimeNo,omitempty"`
	CrimeYear        string     `bson:"crimeYear,omitempty"`
	Charge           string     `bson:"charge,omitempty"`
	WarrantDate      *time.Time `bson:"warrantDate,omitempty"`
	WarrantDueDate   *time.Time `bson:"warrantDueDate,omitempty"`
	WarrantLimitYear string     `bson:"warrantLimitYear,omitempty"`
	Police           string     `bson:"police,omitempty"`
	PolisPos         string     `bson:"polisPos,omitempty"`
	PoliceStation    string     `bson:"policeStation,omitempty"`
	PoliceRegion     string     `bson:"policeRegion,omitempty"`
	PoliceProvincial string     `bson:"policeProvincial,omitempty"`
	BhCode           string     `bson:"bhCode,omitempty"`
	BkCode           string     `bson:"bkCode,omitempty"`
	OrgCode          string     `bson:"orgCode,omitempty"`
	CrimeLocation    string     `bson:"crimeLocation,omitempty"`
	StatusWarrant    string     `bson:"statusWarrant,omitempty"`
}

type CrimeImport struct {
	// identity
	CrimeId    string `bson:"crimeId,omitempty"`
	IdNo       string `bson:"idcard,omitempty"`
	PassportNo string `bson:"passport,omitempty"`
	PersonKey  string `bson:"personKey,omitempty"` // = IdNo || PassportNo

	// bio
	FullName     string     `bson:"fullName,omitempty"`
	TitleString  string     `bson:"titlename,omitempty"`
	FirstName    string     `bson:"firstname,omitempty"`
	LastName     string     `bson:"lastname,omitempty"`
	TitleEn      string     `bson:"titleEn,omitempty"`
	FirstNameEn  string     `bson:"firstNameEn,omitempty"`
	MiddleNameEn string     `bson:"middleNameEn,omitempty"`
	LastNameEn   string     `bson:"lastNameEn,omitempty"`
	Sex          string     `bson:"sex,omitempty"`
	Birthday     *time.Time `bson:"birthday,omitempty"`
	Nation       string     `bson:"nation,omitempty"`
	Country      string     `bson:"country,omitempty"`

	// status
	StatusSuspectArrest string `bson:"statusSuspectArrest,omitempty"`
	IsReqSequester      string `bson:"isReqSequester,omitempty"`

	// import marks
	ImportId string `bson:"importId,omitempty"`
	Source   string `bson:"source,omitempty"`

	// warrants (many)
	Warrants []Warrant `bson:"warrants,omitempty"`
}

type ImportSummary struct {
	Inserted        int64
	Updated         int64
	Deleted         int64
	DuplicatesCount int      // จำนวนคีย์ที่ซ้ำ (keys with >1 rows)
	DuplicatesWhere []string // รายการคีย์ซ้ำ + แถว
	// ใหม่
	FileRows         int // แถวทั้งหมดในไฟล์ (header ไม่นับ)
	Usable           int // แถวที่มี key (id หรือ passport) ใช้ได้
	MissingKey       int // ไม่มีทั้ง id/passport
	BadIdLen         int // id ความยาว != 13 (แต่ยังนับ usable ถ้ามี passport)
	UniquePersons    int // คนไม่ซ้ำหลังรวม
	DupIdWithWarrant int // = Usable - UniquePersons (จำนวนแถวส่วนเกินเพราะคนเดียวหลายบรรทัด)
	DupKeys          int // = DuplicatesCount (alias ชัด ๆ)
}
