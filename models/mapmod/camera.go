// models/mapmod/camera.go
package mapmod

// MapDeviceItem ใช้สำหรับส่งไปหน้า map เวลาเป็นจุดเดี่ยว ๆ
type MapDeviceItem struct {
	ID          string  `json:"id"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	URL         string  `json:"url"`
	IP          string  `json:"ip"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Groups      string  `json:"groups"`
	AtaWsFlvUrl string  `json:"ataWsFlvUrl"`
	Brand       string  `json:"brand"`
	Status      bool    `json:"status"`
}

// MapClusterItem ใช้สำหรับ cluster หลายจุดรวมกัน
type MapClusterItem struct {
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
	Count int     `json:"count"`
}

// MapDetails คือ payload หลักสำหรับ endpoint /devices/map
// ตัวอย่าง:
//
//	{
//	  "details": {
//	    "clusters": [...],
//	    "devices": [...]
//	  }
//	}
type MapDetails struct {
	Clusters []MapClusterItem `json:"clusters"`
	Devices  []MapDeviceItem  `json:"devices"`
}
