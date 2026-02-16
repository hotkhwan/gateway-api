// internal/services/authzsvc/tupleFactory.go
package authzsvc

func TupleFactoryOrgBootstrap(orgId, rootUnitId, userId string) []map[string]interface{} {

	return []map[string]interface{}{
		{
			"entity": map[string]string{
				"type": "organization",
				"id":   orgId,
			},
			"relation": "admin",
			"subject": map[string]string{
				"type": "user",
				"id":   userId,
			},
		},
		{
			"entity": map[string]string{
				"type": "organization",
				"id":   orgId,
			},
			"relation": "member",
			"subject": map[string]string{
				"type": "user",
				"id":   userId,
			},
		},
		{
			"entity": map[string]string{
				"type": "orgUnit",
				"id":   rootUnitId,
			},
			"relation": "admin",
			"subject": map[string]string{
				"type": "user",
				"id":   userId,
			},
		},
		{
			"entity": map[string]string{
				"type": "orgUnit",
				"id":   rootUnitId,
			},
			"relation": "parentOrg",
			"subject": map[string]string{
				"type": "organization",
				"id":   orgId,
			},
		},
	}
}
