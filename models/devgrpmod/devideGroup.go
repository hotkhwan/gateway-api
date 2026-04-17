// models/devgrpmod/devideGroup.go
package devgrpmod

import "time"

type DeviceGroup struct {
	ID        string    `bson:"_id"`
	WorkspaceID string    `bson:"workspaceId"`
	Name      string    `bson:"name"`
	Public    bool      `bson:"public"`
	CreatedAt time.Time `bson:"createdAt"`
}
