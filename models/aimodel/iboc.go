package aimodel

import "time"

type IbocMessage struct {
	Time string `json:"time"`
	// เมตาเพิ่ม
	TypeName      string          `form:"typeName" json:"typeName"`
	ArrestWarrant string          `form:"arrestWarrant" json:"arrestWarrant"`
	State         string          `json:"state,omitempty"`
	Rev           int64           `json:"rev,omitempty"` // ✅ เพิ่มตรงนี้
	ID            string          `json:"id"`
	Timestamp     time.Time       `json:"timestamp"`
	Event         Event           `json:"event"`
	Message       string          `json:"message"`
	Camera        Camera          `json:"camera"`
	Scenario      Scenario        `json:"scenario"`
	Trigger       Trigger         `json:"trigger"`
	FrameImage    string          `json:"frameImage"`
	Analytics     Analytics       `json:"analytics"`
	ObjectTrack   ObjectTrackings `json:"objectTrackings"`
}

// Event (e.g., {"kind":"alert","action":"vehicle-enter-restricted-area"})
type Event struct {
	Kind   string `json:"kind"`
	Action string `json:"action"`
}

// Camera
type Camera struct {
	ID       string      `json:"id"`
	AgentID  string      `json:"agentId"`
	Name     string      `json:"name"`
	Location Coordinates `json:"location"`
	FPS      int         `json:"fps"`
	Width    int         `json:"width"`
	Height   int         `json:"height"`
	Tags     []string    `json:"tags"`
	Metadata CamMetadata `json:"metadata"`
}

type Coordinates struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type CamMetadata struct {
	District string `json:"district"`
	IP       string `json:"ip"`
	Analyze  string `json:"analyze"`
	Point    string `json:"point"`
}

// Scenario
type Scenario struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Tags      []string          `json:"tags"`
	Metadata  map[string]string `json:"metadata"` // dynamic
	Triggers  []string          `json:"triggers"`
	Analytics ScenarioAnalytics `json:"analytics"`
}

type ScenarioAnalytics struct {
	AttributeClassification EnableFlag `json:"attributeClassification"`
	LicensePlateRecognition EnableFlag `json:"licensePlateRecognition"`
	Movement                EnableFlag `json:"movement"`
	ObjectReidentification  EnableFlag `json:"objectReidentification"`
	VehicleSearch           EnableFlag `json:"vehicleSearch"`
}

type EnableFlag struct {
	Enabled bool `json:"enabled"`
}

// Trigger
type Trigger struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	StartTime time.Time              `json:"startTime"`
	Age       int                    `json:"age"`
	Data      map[string]interface{} `json:"data"`
	Event     string                 `json:"event"`
	Relation  string                 `json:"relation"`
	Tags      []string               `json:"tags"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// Analytics
type Analytics struct {
	AttributeClassification AttributeClassification `json:"attributeClassification"`
	LicensePlateRecognition LicensePlateRecognition `json:"licensePlateRecognition"`
	VehicleClassification   VehicleClassification   `json:"vehicleClassification"`
}

type AttributeClassification struct {
	Image      string              `json:"image"`
	Attributes AttributeAttributes `json:"attributes"`
}

type AttributeAttributes struct {
	WearingHelmet Decision `json:"wearingHelmet"`
	Movement      Decision `json:"movement"`
	RiderType     Decision `json:"riderType"`
}

type LicensePlateRecognition struct {
	Image          string        `json:"image"`
	LicensePlate   PlateInfo     `json:"licensePlate"`
	Classification PlateClassify `json:"classification"`
}

type PlateInfo struct {
	Plate  PlateDetail `json:"plate"`
	Region Decision    `json:"region"`
}

type PlateDetail struct {
	Dimensions PlateDimensions `json:"dimensions"`
	Score      float64         `json:"score"`
	Value      string          `json:"value"`
}

type PlateDimensions struct {
	Area   int `json:"area"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

type PlateClassify struct {
	ThaiDLTVehicleType Decision `json:"thaiDltVehicleType"`
}

type VehicleClassification struct {
	Image      string          `json:"image"`
	Dimensions PlateDimensions `json:"dimensions"`
	Result     VehicleResult   `json:"result"`
}

type VehicleResult struct {
	BodyType    Decision `json:"bodyType"`
	Orientation Decision `json:"orientation"`
	Color       Decision `json:"color"`
	Year        Decision `json:"year"`
	IsVehicle   Decision `json:"isVehicle"`
	Model       Decision `json:"model"`
	Make        Decision `json:"make"`
}

type Decision struct {
	Score float64 `json:"score"`
	Label string  `json:"label"`
}

// Object Trackings
type ObjectTrackings struct {
	Recording RecordingData `json:"recording"`
	Start     TrackSnapshot `json:"start"`
	End       TrackSnapshot `json:"end"`
}

type RecordingData struct {
	URL string `json:"url"`
}

type TrackSnapshot struct {
	Timestamp time.Time   `json:"timestamp"`
	Images    TrackImages `json:"images"`
}

type TrackImages struct {
	Frame   string `json:"frame"`
	Plate   string `json:"plate"`
	Vehicle string `json:"vehicle"`
}
