// models/optmod/listoptions.go
package optmod

// ===== ตัวเลือกแบบ id=int (เผื่อจะอ้างอิงในฝั่ง kwatch) =====
type KVItemInt struct {
	ID       int      `json:"id" bson:"id"`
	Title    string   `json:"title,omitempty" bson:"title,omitempty"`
	GroupIds []string `json:"groupIds,omitempty" bson:"groupIds,omitempty"`
}

// ===== ตัวเลือกแบบ id=string (ใช้กับ ksearch หรือหลายภาษา) =====
type KVItemStr struct {
	ID      string `json:"id" bson:"id"`
	Title   string `json:"title,omitempty" bson:"title,omitempty"`
	TitleEn string `json:"titleEn,omitempty" bson:"titleEn,omitempty"`
	TitleTh string `json:"titleTh,omitempty" bson:"titleTh,omitempty"`
}

// ===== ตัวเลือกของ ns=kwatch (id เป็น int) =====
type OptionsWatchman struct {
	Type          *[]KVItemInt `json:"type,omitempty" bson:"type,omitempty"`
	PersonType    *[]KVItemInt `json:"personType,omitempty" bson:"personType,omitempty"`
	TypeNoti      *[]KVItemInt `json:"typeNoti,omitempty" bson:"typeNoti,omitempty"`
	CrimesType    *[]KVItemInt `json:"crimesType,omitempty" bson:"crimesType,omitempty"`
	TitleName     *[]KVItemInt `json:"titlename,omitempty" bson:"titlename,omitempty"`
	MaritalStatus *[]KVItemInt `json:"maritalStatus,omitempty" bson:"maritalStatus,omitempty"`
	Sex           *[]KVItemInt `json:"sex,omitempty" bson:"sex,omitempty"`
	DeathStatus   *[]KVItemInt `json:"deathStatus,omitempty" bson:"deathStatus,omitempty"`
	PoliceRegion  *[]KVItemInt `json:"policeRegion,omitempty" bson:"policeRegion,omitempty"`
}

// ===== ตัวเลือกทั่วไป (id เป็น string; รองรับหลายภาษา) =====
type Options struct {
	Type       *[]KVItemStr `json:"type,omitempty" bson:"type,omitempty"`
	PersonType *[]KVItemStr `json:"personType,omitempty" bson:"personType,omitempty"`
	TypeNoti   *[]KVItemStr `json:"typeNoti,omitempty" bson:"typeNoti,omitempty"`
}

// ===== โครงสร้าง document ใน MongoDB (ไม่ห่อ listOptions) =====
type OptionsDoc struct {
	ID        string `json:"id" bson:"_id"` // _id = "<kind>.<ns>"
	CreatedAt int64  `json:"createdAt,omitempty" bson:"createdAt,omitempty"`
	UpdatedAt int64  `json:"updatedAt,omitempty" bson:"updatedAt,omitempty"`
	// หมายเหตุ: ฟิลด์อื่น ๆ จะเป็น dynamic ตามที่ client PUT มา
}

// ===== Payload / Result แบบ dynamic (ไม่ validate ชนิด) =====
type NamespacedOptionsPayload map[string]map[string]any // {"kwatch": {...}, "ksearch": {...}}
type NamespacedOptions map[string]map[string]any        // สำหรับคืนค่าไป controller
