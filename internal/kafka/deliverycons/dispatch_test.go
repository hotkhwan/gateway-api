// internal/kafka/deliverycons/dispatch_test.go
package deliverycons

import (
	"bytes"
	"testing"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
	"github.com/rs/zerolog"
)

func newTestLogger() (zerolog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return zerolog.New(&buf), &buf
}

func enabledEvent() *ingestmod.NormalizedEvent {
	return fixtureEvent()
}

func TestGate_DisabledTemplateBlocks(t *testing.T) {
	resetDeliverySkipCounters()
	log, _ := newTestLogger()
	tmpl := &ingestmod.MappingTemplate{TemplateId: "t1", Enabled: false}
	if passesTemplateDeliveryGate(tmpl, enabledEvent(), log) {
		t.Fatal("disabled template must not pass the gate")
	}
	disabled, miss := DeliverySkipCounts()
	if disabled != 1 {
		t.Errorf("disabled counter = %d, want 1", disabled)
	}
	if miss != 0 {
		t.Errorf("delivery_rule_miss counter = %d, want 0", miss)
	}
}

func TestGate_EmptyDeliveryRulePasses(t *testing.T) {
	resetDeliverySkipCounters()
	log, _ := newTestLogger()
	tmpl := &ingestmod.MappingTemplate{TemplateId: "t1", Enabled: true}
	if !passesTemplateDeliveryGate(tmpl, enabledEvent(), log) {
		t.Fatal("enabled template with empty delivery rule must pass")
	}
	disabled, miss := DeliverySkipCounts()
	if disabled != 0 || miss != 0 {
		t.Errorf("no skip counters expected; got disabled=%d miss=%d", disabled, miss)
	}
}

func TestGate_DeliveryMatchAllMatchesPass(t *testing.T) {
	resetDeliverySkipCounters()
	log, _ := newTestLogger()
	tmpl := &ingestmod.MappingTemplate{
		TemplateId: "t1",
		Enabled:    true,
		DeliveryMatchAll: []ingestmod.MatchCondition{
			{Field: "sourceAction", Operator: "eq", Values: []string{"detected"}},
		},
	}
	if !passesTemplateDeliveryGate(tmpl, enabledEvent(), log) {
		t.Fatal("matching delivery rule should pass")
	}
}

func TestGate_DeliveryMatchAllMissesBlocks(t *testing.T) {
	resetDeliverySkipCounters()
	log, _ := newTestLogger()
	tmpl := &ingestmod.MappingTemplate{
		TemplateId: "t1",
		Enabled:    true,
		DeliveryMatchAll: []ingestmod.MatchCondition{
			{Field: "sourceAction", Operator: "eq", Values: []string{"captured"}},
		},
	}
	if passesTemplateDeliveryGate(tmpl, enabledEvent(), log) {
		t.Fatal("non-matching delivery rule should block")
	}
	disabled, miss := DeliverySkipCounts()
	if disabled != 0 {
		t.Errorf("disabled counter = %d, want 0", disabled)
	}
	if miss != 1 {
		t.Errorf("delivery_rule_miss counter = %d, want 1", miss)
	}
}

func TestGate_DeliveryMatchAnyOneMatchPasses(t *testing.T) {
	resetDeliverySkipCounters()
	log, _ := newTestLogger()
	tmpl := &ingestmod.MappingTemplate{
		TemplateId: "t1",
		Enabled:    true,
		DeliveryMatchAny: []ingestmod.MatchCondition{
			{Field: "sourceAction", Operator: "eq", Values: []string{"captured"}},  // miss
			{Field: "source.deviceId", Operator: "eq", Values: []string{"51"}},     // match
		},
	}
	if !passesTemplateDeliveryGate(tmpl, enabledEvent(), log) {
		t.Fatal("matchAny with one matching condition should pass")
	}
}

func TestGate_DisabledShortCircuitsBeforeRuleEvaluation(t *testing.T) {
	// Even if the delivery rule would pass, Enabled=false must still block —
	// confirms the Enabled check runs first (plan decision D4).
	resetDeliverySkipCounters()
	log, _ := newTestLogger()
	tmpl := &ingestmod.MappingTemplate{
		TemplateId: "t1",
		Enabled:    false,
		DeliveryMatchAll: []ingestmod.MatchCondition{
			{Field: "sourceAction", Operator: "eq", Values: []string{"detected"}},
		},
	}
	if passesTemplateDeliveryGate(tmpl, enabledEvent(), log) {
		t.Fatal("disabled template must block even when delivery rule would pass")
	}
	disabled, miss := DeliverySkipCounts()
	if disabled != 1 {
		t.Errorf("disabled counter = %d, want 1", disabled)
	}
	if miss != 0 {
		t.Errorf("delivery_rule_miss counter = %d, want 0 (rule should not have been evaluated)", miss)
	}
}
