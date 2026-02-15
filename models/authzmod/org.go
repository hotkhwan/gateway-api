ß// models/authzmod/org.go
package authzmod

type Organization struct {
	OrgId      string `bson:"orgId" json:"orgId"`
	TenantId   string `bson:"tenantId" json:"tenantId"`
	Name       string `bson:"name" json:"name"`
	CreatedBy  string `bson:"createdBy" json:"createdBy"`
	CreatedAt  int64  `bson:"createdAt" json:"createdAt"`
	SyncStatus string `bson:"syncStatus" json:"syncStatus"`
}
