// models/authzmod/orgUnit.go
package authzmod

type OrgUnit struct {
	UnitId    string `bson:"unitId"`
	OrgId     string `bson:"orgId"`
	TenantId  string `bson:"tenantId"`
	Name      string `bson:"name"`
	IsRoot    bool   `bson:"isRoot"`
	CreatedBy string `bson:"createdBy"`
	CreatedAt int64  `bson:"createdAt"`
}
