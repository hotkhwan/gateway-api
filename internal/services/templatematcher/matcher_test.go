// internal/services/templatematcher/matcher_test.go
package templatematcher

import (
	"testing"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

func cond(field, op string, values ...string) ingestmod.MatchCondition {
	return ingestmod.MatchCondition{Field: field, Operator: op, Values: values}
}

func TestEvaluate_EmptyPasses(t *testing.T) {
	if !Evaluate(nil, nil, map[string]any{"anything": 1}) {
		t.Fatal("empty matchAll and matchAny should pass")
	}
	if !Evaluate([]ingestmod.MatchCondition{}, []ingestmod.MatchCondition{}, map[string]any{}) {
		t.Fatal("empty slices should pass")
	}
}

func TestEvaluate_MatchAllAND(t *testing.T) {
	bag := map[string]any{"a": "1", "b": "2"}
	all := []ingestmod.MatchCondition{cond("a", "eq", "1"), cond("b", "eq", "2")}
	if !Evaluate(all, nil, bag) {
		t.Fatal("all conditions satisfied, expected true")
	}
	all2 := []ingestmod.MatchCondition{cond("a", "eq", "1"), cond("b", "eq", "9")}
	if Evaluate(all2, nil, bag) {
		t.Fatal("one condition fails, expected false")
	}
}

func TestEvaluate_MatchAnyOR(t *testing.T) {
	bag := map[string]any{"a": "1"}
	any := []ingestmod.MatchCondition{cond("x", "eq", "9"), cond("a", "eq", "1")}
	if !Evaluate(nil, any, bag) {
		t.Fatal("one condition matches, expected true")
	}
	any2 := []ingestmod.MatchCondition{cond("x", "eq", "9"), cond("y", "eq", "8")}
	if Evaluate(nil, any2, bag) {
		t.Fatal("no condition matches, expected false")
	}
}

func TestEvaluate_AllAndAnyBothRequired(t *testing.T) {
	bag := map[string]any{"a": "1", "b": "2"}
	all := []ingestmod.MatchCondition{cond("a", "eq", "1")}
	anyPass := []ingestmod.MatchCondition{cond("b", "eq", "2")}
	if !Evaluate(all, anyPass, bag) {
		t.Fatal("AND passes and OR passes, expected true")
	}
	anyFail := []ingestmod.MatchCondition{cond("b", "eq", "999")}
	if Evaluate(all, anyFail, bag) {
		t.Fatal("AND passes but OR fails, expected false")
	}
	allFail := []ingestmod.MatchCondition{cond("a", "eq", "999")}
	if Evaluate(allFail, anyPass, bag) {
		t.Fatal("AND fails, expected false regardless of OR")
	}
}

func TestEvaluate_Operators(t *testing.T) {
	bag := map[string]any{"x": "hello-world"}
	cases := []struct {
		name string
		cond ingestmod.MatchCondition
		want bool
	}{
		{"eq match", cond("x", "eq", "hello-world"), true},
		{"eq miss", cond("x", "eq", "nope"), false},
		{"in match", cond("x", "in", "a", "hello-world", "b"), true},
		{"in miss", cond("x", "in", "a", "b"), false},
		{"contains match", cond("x", "contains", "world"), true},
		{"contains miss", cond("x", "contains", "space"), false},
		{"prefix match", cond("x", "prefix", "hello"), true},
		{"prefix miss", cond("x", "prefix", "world"), false},
		{"unknown op", cond("x", "regex", "anything"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate([]ingestmod.MatchCondition{tc.cond}, nil, bag)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestEvaluate_DottedPath(t *testing.T) {
	bag := map[string]any{
		"source": map[string]any{
			"deviceId": "51",
			"vendor":   "Dahua",
		},
		"payload": map[string]any{
			"action": "captured",
		},
	}
	if !Evaluate([]ingestmod.MatchCondition{cond("source.deviceId", "eq", "51")}, nil, bag) {
		t.Fatal("source.deviceId should resolve")
	}
	if !Evaluate([]ingestmod.MatchCondition{cond("payload.action", "eq", "captured")}, nil, bag) {
		t.Fatal("payload.action should resolve")
	}
}

func TestEvaluate_MissingFieldFails(t *testing.T) {
	bag := map[string]any{"a": "1"}
	if Evaluate([]ingestmod.MatchCondition{cond("missing", "eq", "1")}, nil, bag) {
		t.Fatal("missing field should not match")
	}
	if Evaluate([]ingestmod.MatchCondition{cond("a.deep.path", "eq", "1")}, nil, bag) {
		t.Fatal("non-map traversal should not match")
	}
}

func TestEvaluate_NumericCoerceToString(t *testing.T) {
	// Existing behavior: fmt.Sprintf("%v") stringifies numbers; equality is string compare.
	bag := map[string]any{"n": 51}
	if !Evaluate([]ingestmod.MatchCondition{cond("n", "eq", "51")}, nil, bag) {
		t.Fatal("int 51 should compare equal to string \"51\" via Sprintf")
	}
}
