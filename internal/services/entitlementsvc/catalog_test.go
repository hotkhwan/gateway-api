package entitlementsvc

import "testing"

func TestCatalogDefaults(t *testing.T) {
	cat := NewRuntimeEntitlementCatalog()

	cases := []struct {
		name     string
		planCode string
		want     string
	}{
		{"freemium", "freemium", "freemium"},
		{"pro", "pro", "pro"},
		{"enterprise", "enterprise", "enterprise"},
		{"appliance", "appliance", "appliance"},
		{"unknown falls back to freemium", "mystery-plan", "freemium"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ent := cat.Default(tc.planCode)
			if ent.PlanCode != tc.want {
				t.Fatalf("PlanCode: got %q want %q", ent.PlanCode, tc.want)
			}
			if ent.MaxEventsPerSecond <= 0 {
				t.Fatalf("MaxEventsPerSecond must be > 0, got %d", ent.MaxEventsPerSecond)
			}
			if ent.MaxPayloadBytes <= 0 {
				t.Fatalf("MaxPayloadBytes must be > 0, got %d", ent.MaxPayloadBytes)
			}
		})
	}
}

func TestCatalogForProfile(t *testing.T) {
	cat := NewRuntimeEntitlementCatalog()

	// appliance and enterprise both resolve to unlimited defaults so
	// co-located deployments don't need to ship a platform license before
	// they can accept ingest.
	if got := cat.ForProfile("appliance").MaxAssets; got != -1 {
		t.Fatalf("appliance MaxAssets: got %d want -1 (unlimited)", got)
	}
	if got := cat.ForProfile("enterprise").MaxAssets; got != -1 {
		t.Fatalf("enterprise MaxAssets: got %d want -1 (unlimited)", got)
	}

	// saasPublic and unknown profiles fall back to freemium — the
	// subscription overlay is what widens the limits later.
	if got := cat.ForProfile("saasPublic").PlanCode; got != "freemium" {
		t.Fatalf("saasPublic PlanCode fallback: got %q want freemium", got)
	}
	if got := cat.ForProfile("").PlanCode; got != "freemium" {
		t.Fatalf("empty profile PlanCode: got %q want freemium", got)
	}
}
