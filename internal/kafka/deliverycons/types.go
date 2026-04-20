// internal/kafka/deliverycons/types.go
package deliverycons

import (
	"github.com/hotkhwan/gateway-api/internal/repo/dlqrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestdetailsrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/targetrepo"
	"github.com/rs/zerolog"
)

// ConsumerDeps holds all dependencies injected into the delivery consumer.
// EventDetailsRepo is optional — used to re-hydrate binaryRefs when the
// incoming normalized.events payload omits them (happens on the klynx-api
// republish path where the minimal klynxEvent struct drops binaryRefs).
type ConsumerDeps struct {
	TargetRepo       *targetrepo.TargetRepo
	TemplateRepo     *ingestrepo.MappingTemplateRepo
	DLQRepo          *dlqrepo.DLQRepo
	EventDetailsRepo *ingestdetailsrepo.EventDetailsRepo
	Logger           zerolog.Logger
}
