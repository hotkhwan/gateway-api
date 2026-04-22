// internal/services/templatesvc/bootstrap.go
package templatesvc

import (
	"context"

	"github.com/hotkhwan/gateway-api/internal/repo/ingestrepo"
	"github.com/hotkhwan/gateway-api/utils/traceutil"
)

// BackfillEnabledForDeliveringTemplates is a package-level startup hook used
// by bootstrap (main.go) to flip enabled=true on legacy mapping templates
// whose delivery should keep working under the new template-level delivery
// gate. Idempotent, non-fatal — logs failures but does not block startup.
//
// Plan decision D10 requires this to run before the delivery consumer starts
// enforcing Template.Enabled.
func BackfillEnabledForDeliveringTemplates(ctx context.Context) {
	ctx, end, log := traceutil.StartLite(ctx, "gateway.templatesvc", "BackfillEnabledForDeliveringTemplates", "templatesvc", "Backfill")
	defer end()

	repo := ingestrepo.NewMappingTemplateRepo()
	matched, modified, err := repo.BackfillEnabledForTemplatesWithDeliveryTargets(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("[templatesvc] backfill enabled skipped — continuing startup")
		return
	}
	log.Info().
		Int64("matched", matched).
		Int64("modified", modified).
		Msg("[templatesvc] backfill enabled complete")
}
