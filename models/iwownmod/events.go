// models/iwownmod/events.go
package iwownmod

// ---------- Response ----------

type IwownResponseCode struct {
	ReturnCode int
}

type IwownResponseObject struct {
	ReturnCode int
	Data       interface{}
}

// ---------- Call Log Model ----------

type IwownCallRecord struct {
	Status     int    `json:"status"`
	CallNumber string `json:"call_number"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
}

type IwownCallLogWithAlarm struct {
	AlarmTime string             `json:"alarm_time"`
	Lat       string             `json:"lat"`
	Lon       string             `json:"lon"`
	CallLogs  []*IwownCallRecord `json:"call_logs"`
}

type IwownDeviceCallLogs struct {
	Deviceid       string                   `json:"deviceid"`
	NormalCallLogs []*IwownCallRecord       `json:"normal_call_logs"`
	Sos            []*IwownCallLogWithAlarm `json:"sos"`
}

// ---------- DeviceInfo / Status / Sleep ----------

type IwownDeviceInfo struct {
	Deviceid          string `json:"deviceid"`
	Imsi              string `json:"imsi"`
	SN                string `json:"sn"`
	Mac               string `json:"mac"`
	NetType           string `json:"net_type"`
	NetOperator       string `json:"net_operator"`
	WearingStatus     string `json:"wearing_status"`
	Model             string `json:"model"`
	Version           string `json:"version"`
	Sim1IccId         string `json:"sim1_iccid"`
	Sim1CellId        string `json:"sim1_cellid"`
	Sim1NetAdhere     string `json:"sim1_netadhere"`
	NetworkStatus     string `json:"network_status"`
	BandDetail        string `json:"band_detail"`
	RefSignal         string `json:"refsignal"`
	Band              string `json:"band"`
	CommunicationMode string `json:"communication_mode"`
	WatchEvent        int    `json:"watch_event"`
}

type IwownDeviceStatus struct {
	DeviceId  string
	EventTime string
	Status    string
}

type IwownSleepResult struct {
	Deviceid     string `json:"deviceid"`
	SleepDate    string `json:"sleep_date"`
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time"`
	DeepSleep    int    `json:"deep_sleep"`
	LightSleep   int    `json:"light_sleep"`
	WeakSleep    int    `json:"weak_sleep"`
	EyemoveSleep int    `json:"eyemove_sleep"`
	Score        int    `json:"score"`
	OsahsRisk    int    `json:"osahs_risk"`
	Spo2Score    int    `json:"spo2_score"`
	SleepHr      int    `json:"sleep_hr"`
}

// ---------- Binary Frame Event (Kafka payload) ----------

type IwownBinaryFrameEvent struct {
	Kind     string `json:"kind"`     // "pb" หรือ "alarm"
	Index    int    `json:"index"`    // ลำดับ frame ภายใน payload
	DeviceID string `json:"deviceId"` // device id 15 bytes แรก
	Opt      uint16 `json:"opt"`      // opt flag 0x0A, 0x80, 0x10, 0x12 ฯลฯ
	Payload  []byte `json:"payload"`  // raw protobuf data
}
