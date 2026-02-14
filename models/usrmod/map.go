// models/usrmod/map.go
package usrmod

type MapLocation struct {
	Lat string `json:"lat" bson:"lat"`
	Lng string `json:"lng" bson:"lng"`
}

type MapSetting struct {
	MapLocation MapLocation `json:"mapLocation" bson:"mapLocation"`
	ZoomLevel   int         `json:"zoomLevel" bson:"zoomLevel"`
}
