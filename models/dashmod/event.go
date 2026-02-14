// models/dashmod/event.go
package dashmod

type DashboardEventStats struct {
	Total        int `json:"total"`
	Vehicle      int `json:"vehicle"`
	Face         int `json:"face"`
	MotorVehicle int `json:"motorVehicle"`
	NoneVehicle  int `json:"noneVehicle"`
}

type DashboardSimpleStats struct {
	Total   int `json:"total"`
	Online  int `json:"online"`
	Offline int `json:"offline"`
}

type DashboardPeopleCountingStats struct {
	Total int `json:"total"`
	In    int `json:"in"`
	Out   int `json:"out"`
}

type DashboardNotificationItem struct {
	ID            string  `json:"id,omitempty"`
	ImageURL      string  `json:"imageUrl"`
	VideoClipURL  string  `json:"videoClipUrl"`
	Type          string  `json:"type"`
	Similarity    float64 `json:"similarity"`
	Name          string  `json:"name"`
	DeviceName    string  `json:"deviceName"`
	Lat           float64 `json:"lat,omitempty"`
	Long          float64 `json:"long,omitempty"`
	Timestamp     string  `json:"timestamp"`
	ListType      int     `json:"listType,omitempty"`
	ListTypeLabel string  `json:"listTypeLabel,omitempty"`
}

type DashboardDetails struct {
	Event          DashboardEventStats          `json:"event"`
	Camera         DashboardSimpleStats         `json:"camera"`
	Kcontrol       DashboardSimpleStats         `json:"kcontrol"`
	PeopleCounting DashboardPeopleCountingStats `json:"peopleCountingArea"`
	Notification   []DashboardNotificationItem  `json:"notification"`
	Blacklist      []DashboardNotificationItem  `json:"blacklist"`
}
