package router

import "testing"

func TestStripeBillingEnabled(t *testing.T) {
	cases := []struct {
		profile string
		stripe  string
		want    bool
	}{
		{"saasPublic", "true", true},
		{"saasPublic", "TRUE", true},
		{"saasPublic", "false", false},
		{"saasPublic", "", false},
		{"appliance", "true", false}, // appliance never registers local billing
		{"enterprise", "true", false},
		{"", "true", false},
	}
	for _, tc := range cases {
		t.Setenv("DEPLOYMENT_PROFILE", tc.profile)
		t.Setenv("STRIPE_BILLING_ENABLED", tc.stripe)
		if got := stripeBillingEnabled(); got != tc.want {
			t.Fatalf("profile=%q stripe=%q: got %v want %v", tc.profile, tc.stripe, got, tc.want)
		}
	}
}

func TestEnvFlag(t *testing.T) {
	cases := []struct {
		val  string
		want bool
	}{
		{"true", true},
		{"True", true},
		{"TRUE", true},
		{"false", false},
		{"1", false}, // intentionally strict — only "true" enables
		{"", false},
	}
	for _, tc := range cases {
		t.Setenv("TEST_FLAG_FOR_ENV_FLAG", tc.val)
		if got := envFlag("TEST_FLAG_FOR_ENV_FLAG"); got != tc.want {
			t.Fatalf("envFlag(%q): got %v want %v", tc.val, got, tc.want)
		}
	}
}
