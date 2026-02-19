// internal/services/authzsvc/orgUnitValidation.go
package authzsvc

import (
	"context"
	"fmt"
)

func (s *OrgUnitService) ValidateNoCycle(
	ctx context.Context,
	tenantId string,
	orgId string,
	unitId string,
	newParentId string,
) error {

	if unitId == newParentId {
		return fmt.Errorf("cannot set self as parent")
	}

	descendants, err := s.orgUnitRepo.FindDescendants(ctx, tenantId, orgId, unitId)
	if err != nil {
		return err
	}

	for _, d := range descendants {
		if d.UnitId == newParentId {
			return fmt.Errorf("cannot create cycle in orgUnit tree")
		}
	}

	return nil
}
