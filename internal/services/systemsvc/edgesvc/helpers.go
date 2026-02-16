// internal/services/systemsvc/edgesvc/helpers.go
package edgesvc

import (
	"fmt"
	"strings"

	"github.com/hotkhwan/gateway-api/models/systemmod"
)

func normalizeEdgeType(t string) (systemmod.EdgeType, error) {
	tt := systemmod.EdgeType(strings.ToLower(strings.TrimSpace(t)))
	switch tt {
	case systemmod.EdgeATA, systemmod.EdgeSVMS, systemmod.EdgeIBOC:
		return tt, nil
	default:
		return "", fmt.Errorf("unsupported edgeType: %s", t)
	}
}
