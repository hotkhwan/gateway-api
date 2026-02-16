// models/devgrpmod/devmap.go
package devgrpmod

type MapDeviceItem struct {
	ID       string  `json:"id"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	IsOwner  bool    `json:"isOwner"`
	IsPublic bool    `json:"isPublic"`
}
