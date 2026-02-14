// models/kwatmod/iboc.go
package kwatmod

// WatchlistCoreForIBOC: ฟิลด์หลัก ๆ ที่ต้องใช้คำนวณ title ต่าง ๆ เพื่อ sync กลับ IBOC
type WatchlistCoreForIBOC struct {
	Type                 int    `bson:"type" json:"type,omitempty"`
	CrimesType           int    `bson:"crimesType" json:"crimesType,omitempty"`
	IdCard               string `bson:"idcard" json:"idcard,omitempty"`
	TitleName            string `bson:"titleName" json:"titleName,omitempty"`
	SubTitleName         string `bson:"subTitleName" json:"subTitleName,omitempty"`
	FirstName            string `bson:"firstName" json:"firstName,omitempty"`
	LastName             string `bson:"lastName" json:"lastName,omitempty"`
	NickName             string `bson:"nickName" json:"nickName,omitempty"`
	Sex                  string `bson:"sex" json:"sex,omitempty"`
	Birthday             string `bson:"birthday" json:"birthday,omitempty"`
	Age                  int    `bson:"age" json:"age,omitempty"`
	FatherName           string `bson:"fatherName" json:"fatherName,omitempty"`
	FatherIdCard         string `bson:"fatherIdcard" json:"fatherIdcard,omitempty"`
	MotherName           string `bson:"motherName" json:"motherName,omitempty"`
	MotherIdCard         string `bson:"motherIdcard" json:"motherIdcard,omitempty"`
	MaritalStatus        string `bson:"maritalStatus" json:"maritalStatus,omitempty"`
	DateOfDeath          string `bson:"dateOfDeath" json:"dateOfDeath,omitempty"`
	Passport             string `bson:"passport" json:"passport,omitempty"`
	DeathStatus          int    `bson:"deathStatus" json:"deathStatus,omitempty"`
	UserRecorder         string `bson:"userRecorder" json:"userRecorder,omitempty"`
	UserPosition         string `bson:"userPosition" json:"userPosition,omitempty"`
	AlertType            string `bson:"alertType" json:"alertType,omitempty"`
	AlertDesc            string `bson:"alertDesc" json:"alertDesc,omitempty"`
	PoliceRegion         int    `bson:"policeRegion" json:"policeRegion,omitempty"`
	PoliceProvincial     int    `bson:"policeProvincial" json:"policeProvincial,omitempty"`
	PoliceStation        int    `bson:"policeStation" json:"policeStation,omitempty"`
	StationTitleFallback string `bson:"stationTitleFallback" json:"stationTitleFallback,omitempty"`
	Warrants             []struct {
		PoliceStation string `bson:"policeStation" json:"policeStation,omitempty"`
	} `bson:"warrants" json:"warrants,omitempty"`
}
