func TupleFactoryOrgBootstrap(orgId, userId string) []permify.Tuple {

	ownersGroupId := "owners-" + orgId
	defaultDG := "default-" + orgId

	return []permify.Tuple{

		// org admin
		NewTuple("organization", orgId, "admin", "user", userId),

		// owners group parent
		NewTuple("orgGroup", ownersGroupId, "parentOrg", "organization", orgId),

		// owners group member
		NewTuple("orgGroup", ownersGroupId, "member", "user", userId),

		// default deviceGroup parent
		NewTuple("deviceGroup", defaultDG, "parentOrg", "organization", orgId),

		// default deviceGroup editor
		NewTuple("deviceGroup", defaultDG, "editor", "orgGroup", ownersGroupId),
	}
}
