// internal/repo/ingestrepo/template_test.go
package ingestrepo

import (
	"reflect"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

// TestBackfillEnabledFilter_OnlyMatchesMissingField pins the filter shape so
// the backfill cannot silently flip operator-set `enabled: false` back to true
// (post-review High finding). The only cohort targeted is docs where the
// `enabled` field is absent AND the template owns at least one delivery target.
func TestBackfillEnabledFilter_OnlyMatchesMissingField(t *testing.T) {
	got := backfillEnabledFilter()

	want := bson.M{
		"enabled":         bson.M{"$exists": false},
		"deliveryTargets": bson.M{"$exists": true, "$not": bson.M{"$size": 0}},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("backfillEnabledFilter drift:\n got  = %#v\n want = %#v", got, want)
	}
}

// TestBackfillEnabledFilter_DoesNotUseNeTrue guards against regression to
// `{enabled: {$ne: true}}`, which would include explicit `enabled: false`
// docs (operator intent) in the update set.
func TestBackfillEnabledFilter_DoesNotUseNeTrue(t *testing.T) {
	f := backfillEnabledFilter()
	enabledClause, ok := f["enabled"].(bson.M)
	if !ok {
		t.Fatalf("enabled clause should be bson.M, got %T", f["enabled"])
	}
	if _, hasNe := enabledClause["$ne"]; hasNe {
		t.Fatal("filter must not use $ne — explicit enabled=false would be overwritten, violating operator intent")
	}
	exists, ok := enabledClause["$exists"].(bool)
	if !ok || exists != false {
		t.Fatalf("expected $exists:false, got %#v", enabledClause)
	}
}

// TestBackfillEnabledFilter_RequiresDeliveryTargets guards that templates
// without delivery targets are not touched.
func TestBackfillEnabledFilter_RequiresDeliveryTargets(t *testing.T) {
	f := backfillEnabledFilter()
	dt, ok := f["deliveryTargets"].(bson.M)
	if !ok {
		t.Fatalf("deliveryTargets clause should be bson.M, got %T", f["deliveryTargets"])
	}
	if exists, _ := dt["$exists"].(bool); !exists {
		t.Errorf("expected $exists:true on deliveryTargets, got %#v", dt)
	}
	not, ok := dt["$not"].(bson.M)
	if !ok {
		t.Fatalf("deliveryTargets $not clause missing: %#v", dt)
	}
	size, ok := not["$size"].(int)
	if !ok || size != 0 {
		t.Errorf("expected deliveryTargets: $not:{$size:0}, got %#v", not)
	}
}
