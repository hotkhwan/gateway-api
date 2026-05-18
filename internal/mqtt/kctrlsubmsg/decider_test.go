package kctrlsubmsg

import (
	"context"
	"testing"

	"github.com/hotkhwan/gateway-api/internal/services/kctrlregistrysvc"
)

// fakeDecider records the hwId passed in so we can verify the wiring.
type fakeDecider struct {
	lastHwId string
	out      kctrlregistrysvc.Decision
}

func (f *fakeDecider) Decide(_ context.Context, hwId string) kctrlregistrysvc.Decision {
	f.lastHwId = hwId
	return f.out
}

func TestSetRegistryDecider_StoreAndLoad(t *testing.T) {
	// Reset to nil first so the test doesn't bleed into other handler tests.
	t.Cleanup(func() { SetRegistryDecider(nil) })

	d := &fakeDecider{out: kctrlregistrysvc.Decision{Action: kctrlregistrysvc.ActionEnrich, OrgId: "org-1"}}
	SetRegistryDecider(d)

	got := loadDecider()
	if got == nil {
		t.Fatal("expected decider loaded, got nil")
	}
	dec := got.Decide(context.Background(), "h-1")
	if dec.Action != kctrlregistrysvc.ActionEnrich || dec.OrgId != "org-1" {
		t.Errorf("decider passthrough failed: %+v", dec)
	}
	if d.lastHwId != "h-1" {
		t.Errorf("hwId not forwarded: %q", d.lastHwId)
	}
}

func TestSetRegistryDecider_NilDisables(t *testing.T) {
	t.Cleanup(func() { SetRegistryDecider(nil) })

	SetRegistryDecider(&fakeDecider{})
	SetRegistryDecider(nil)

	if loadDecider() != nil {
		t.Error("expected nil after SetRegistryDecider(nil)")
	}
}
