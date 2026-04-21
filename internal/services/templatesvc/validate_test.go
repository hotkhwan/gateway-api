// internal/services/templatesvc/validate_test.go
package templatesvc

import (
	"strings"
	"testing"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

func mc(field, op string, values ...string) ingestmod.MatchCondition {
	return ingestmod.MatchCondition{Field: field, Operator: op, Values: values}
}

func TestValidateDeliveryRule_AcceptsCanonicalFields(t *testing.T) {
	err := validateDeliveryRule("deliveryMatchAll", []ingestmod.MatchCondition{
		mc("sourceAction", "eq", "captured"),
		mc("source.deviceId", "in", "51", "52"),
		mc("eventClass", "eq", "alert"),
		mc("payload.helmet", "eq", "1"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateDeliveryRule_RejectsRawPrefix(t *testing.T) {
	err := validateDeliveryRule("deliveryMatchAll", []ingestmod.MatchCondition{
		mc("raw.sn", "eq", "X"),
	})
	if err == nil {
		t.Fatal("expected rejection of raw.* field")
	}
	if !IsInvalidDeliveryRule(err) {
		t.Fatalf("expected InvalidDeliveryRuleError, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "raw.sn") {
		t.Errorf("error should name the offending field; got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "delivery stage") {
		t.Errorf("error should mention delivery stage; got %q", err.Error())
	}
}

func TestValidateDeliveryRule_RejectsBareRawToken(t *testing.T) {
	err := validateDeliveryRule("deliveryMatchAll", []ingestmod.MatchCondition{
		mc("raw", "eq", "X"),
	})
	if err == nil || !IsInvalidDeliveryRule(err) {
		t.Fatalf("expected InvalidDeliveryRuleError for bare 'raw'; got %v", err)
	}
}

func TestValidateDeliveryRule_RejectsUnknownOperator(t *testing.T) {
	err := validateDeliveryRule("deliveryMatchAll", []ingestmod.MatchCondition{
		mc("sourceAction", "regex", "^capt"),
	})
	if err == nil || !IsInvalidDeliveryRule(err) {
		t.Fatalf("expected InvalidDeliveryRuleError for unknown operator; got %v", err)
	}
	if !strings.Contains(err.Error(), "regex") {
		t.Errorf("error should name unsupported operator; got %q", err.Error())
	}
}

func TestValidateDeliveryRule_RejectsEmptyField(t *testing.T) {
	err := validateDeliveryRule("deliveryMatchAll", []ingestmod.MatchCondition{
		mc("", "eq", "x"),
	})
	if err == nil || !IsInvalidDeliveryRule(err) {
		t.Fatalf("expected rejection for empty field; got %v", err)
	}
}

func TestValidateDeliveryRule_RejectsEmptyValues(t *testing.T) {
	err := validateDeliveryRule("deliveryMatchAll", []ingestmod.MatchCondition{
		mc("sourceAction", "eq"),
	})
	if err == nil || !IsInvalidDeliveryRule(err) {
		t.Fatalf("expected rejection for empty values; got %v", err)
	}
}

func TestValidateDeliveryRules_ChecksBothLists(t *testing.T) {
	// matchAll passes, matchAny fails — combined validator must report matchAny error.
	err := validateDeliveryRules(
		[]ingestmod.MatchCondition{mc("sourceAction", "eq", "captured")},
		[]ingestmod.MatchCondition{mc("raw.sn", "eq", "X")},
	)
	if err == nil || !strings.Contains(err.Error(), "deliveryMatchAny") {
		t.Fatalf("expected deliveryMatchAny error, got %v", err)
	}
}

func TestValidateDeliveryRule_EmptyIsAccepted(t *testing.T) {
	if err := validateDeliveryRule("deliveryMatchAll", nil); err != nil {
		t.Fatalf("nil slice should be accepted: %v", err)
	}
	if err := validateDeliveryRule("deliveryMatchAll", []ingestmod.MatchCondition{}); err != nil {
		t.Fatalf("empty slice should be accepted: %v", err)
	}
}

func TestValidateDeliveryRule_AllOperators(t *testing.T) {
	for _, op := range []string{"eq", "in", "contains", "prefix", "EQ", " Prefix "} {
		err := validateDeliveryRule("deliveryMatchAll", []ingestmod.MatchCondition{
			mc("sourceAction", op, "x"),
		})
		if err != nil {
			t.Errorf("operator %q should be accepted: %v", op, err)
		}
	}
}

