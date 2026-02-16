// models/devgrpmod/device.go
package devgrpmod

type Device struct {
	ID      string `bson:"_id"`
	OrgID   string `bson:"orgId"`
	GroupID string `bson:"groupId"`
}
