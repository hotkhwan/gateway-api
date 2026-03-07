// internal/kafka/deliverycons/types.go
package deliverycons

import (
	"github.com/hotkhwan/gateway-api/internal/repo/dlqrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/hotkhwan/gateway-api/internal/repo/targetrepo"
	"github.com/rs/zerolog"
)

// ConsumerDeps holds all dependencies injected into the delivery consumer.
type ConsumerDeps struct {
	TargetRepo   *targetrepo.TargetRepo
	TemplateRepo *ingestrepo.MappingTemplateRepo
	DLQRepo      *dlqrepo.DLQRepo
	Logger       zerolog.Logger
}
