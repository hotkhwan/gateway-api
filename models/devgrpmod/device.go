// models/devgrpmod/device.go
package devgrpmod

type Device struct {
	ID      string `bson:"_id"`
	WorkspaceID string `bson:"workspaceId"`
	GroupID string `bson:"groupId"`
}
