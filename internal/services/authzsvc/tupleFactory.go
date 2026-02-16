// internal/services/authzsvc/tupleFactory.go
package authzsvc

func TupleFactoryOrgBootstrap(orgId, userId string) []map[string]interface{} {
	ownersGroupId := "owners-" + orgId
	defaultDG := "default-" + orgId

	return []map[string]interface{}{
		{
			"entity":   map[string]string{"type": "organization", "id": orgId},
			"relation": "admin",
			"subject":  map[string]string{"type": "user", "id": userId},
		},
		{
			"entity":   map[string]string{"type": "orgGroup", "id": ownersGroupId},
			"relation": "parentOrg",
			"subject":  map[string]string{"type": "organization", "id": orgId},
		},
		{
			"entity":   map[string]string{"type": "orgGroup", "id": ownersGroupId},
			"relation": "member",
			"subject":  map[string]string{"type": "user", "id": userId},
		},
		{
			"entity":   map[string]string{"type": "deviceGroup", "id": defaultDG},
			"relation": "parentOrg",
			"subject":  map[string]string{"type": "organization", "id": orgId},
		},
		{
			"entity":   map[string]string{"type": "deviceGroup", "id": defaultDG},
			"relation": "editor",
			"subject":  map[string]string{"type": "orgGroup", "id": ownersGroupId},
		},
	}
}
