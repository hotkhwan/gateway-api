// models/kctrlmodel/request.go
package kaimod

type Detection struct {
	Details Details `json:"details"` // สะกดผิดใน JSON ต้องใช้ตามจริง
}

type Details struct {
	Analyze Analyze `json:"analyze"`
	Camera  Camera  `json:"camera"`
}

type Analyze struct {
	Detection AnalyzeDetection `json:"detection"`
}

type AnalyzeDetection struct {
	ID    string `json:"id"`
	Event string `json:"event"`
	Time  string `json:"time"`
	State string `json:"state,omitempty"`
	Rev   int64  `json:"rev,omitempty"`

	NameMethod string     `json:"nameMethod"`
	Timestamp  string     `json:"timeStamp"`
	StreamID   int        `json:"streamId"`
	StreamURL  string     `json:"streamUrl"`
	Confidence float64    `json:"confidence"`
	Labels     string     `json:"labels"`
	TrackingID int        `json:"trackingId"`
	BBox       []int      `json:"bbox"` // [x1, y1, x2, y2]
	Bucket     string     `json:"bucket"`
	ImageFull  string     `json:"imageFull"`
	ImageCrop  string     `json:"imageCrop"`
	NameFile   string     `json:"nameFile"`
	LPR        *LprResult `json:"lpr,omitempty"`     // ✅ ผลลัพธ์จาก AI
	LprFlag    *bool      `json:"lprFlag,omitempty"` // ✅ flag ว่ามี LPR หรือไม่
	LprResult  *LprResult `json:"lprResult,omitempty"`
	// Face       *FaceResult `json:"face,omitempty"`     // ✅ ผลลัพธ์จาก AI
	FaceFlag *bool `json:"faceFlag,omitempty"` // ยังไม่รองรับ
	// FaceResult *FaceResult `json:"faceResult,omitempty"`
}

type Camera struct {
	ID             string  `json:"id"`
	Name           string  `json:"name"`
	User           string  `json:"user"`
	Password       string  `json:"password"`
	URL            string  `json:"url"`
	District       string  `json:"district"`
	Lat            float64 `json:"lat"`
	Lng            float64 `json:"lng"`
	Brand          string  `json:"brand"`
	Status         bool    `json:"status"`
	State          string  `json:"state"`
	DateTimeCreate string  `json:"dateTimeCreate"`
	DateTimeUpdate string  `json:"dateTimeUpdate"`
}

type AIResult struct {
	ID    string `json:"id"`
	Event string `json:"event"`
	Time  string `json:"time"`
	State string `json:"state,omitempty"`
	Rev   int64  `json:"rev,omitempty"`

	Details     Details `json:"details"`
	EventType   string  `json:"eventType"`
	ProcessedAt string  `json:"processedAt"`
	EventID     string  `json:"eventId"`
	Lpr         bool    `json:"lpr"`
	Face        bool    `json:"face"`
}

type LprResult struct {
	LicensePlate *struct {
		PlateNumber   string  `json:"plateNumber"`
		PlateProvince string  `json:"plateProvince"`
		Confidence    float64 `json:"confidence"`
		Location      struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"location"`
	} `json:"licensePlate"` // ✅ รองรับ null ได้

	VehicleInfo *struct {
		Brand struct {
			Name       string  `json:"name"`
			Confidence float64 `json:"confidence"`
		} `json:"brand"`
		Color struct {
			Name       string  `json:"name"`
			Confidence float64 `json:"confidence"`
		} `json:"color"`
		Type struct {
			Name       string  `json:"name"`
			Confidence float64 `json:"confidence"`
		} `json:"type"`
	} `json:"vehicleInfo"` // ✅ รองรับ null ได้
}

type SourceLpr struct {
	LicensePlate *struct {
		PlateNumber   string  `json:"plate_number"`
		PlateProvince string  `json:"plate_province"`
		Confidence    float64 `json:"confidence"`
		Location      struct {
			X      int `json:"x"`
			Y      int `json:"y"`
			Width  int `json:"width"`
			Height int `json:"height"`
		} `json:"location"`
	} `json:"license_plate"` // ✅ รองรับ null ได้

	VehicleInfo *struct {
		Brand struct {
			Name       string  `json:"name"`
			Confidence float64 `json:"confidence"`
		} `json:"brand"`
		Color struct {
			Name       string  `json:"name"`
			Confidence float64 `json:"confidence"`
		} `json:"color"`
		Type struct {
			Name       string  `json:"name"`
			Confidence float64 `json:"confidence"`
		} `json:"type"`
	} `json:"vehicle_info"` // ✅ รองรับ null ได้
}
