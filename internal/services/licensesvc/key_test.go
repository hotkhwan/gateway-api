package licensesvc

import (
	"regexp"
	"strings"
	"testing"
)

func TestGenerateFormatAndUniqueness(t *testing.T) {
	secret := "unit-test-secret"
	key, err := Generate(secret)
	if err != nil {
		t.Fatalf("Generate: unexpected error %v", err)
	}
	if !ValidateFormat(key) {
		t.Fatalf("Generate produced key %q which does not match %s", key, KeyFormat)
	}

	pattern := regexp.MustCompile(`^[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}-[0-9A-F]{4}$`)
	if !pattern.MatchString(key) {
		t.Fatalf("Generate produced non-uppercase-hex key: %q", key)
	}

	// Two sequential generations must not collide (entropy is 16 bytes; this
	// asserts Generate pulls fresh randomness per call).
	k1, _ := Generate(secret)
	k2, _ := Generate(secret)
	if k1 == k2 {
		t.Fatalf("Generate returned identical keys %q %q — entropy is not refreshed", k1, k2)
	}
}

func TestGenerateRequiresSecret(t *testing.T) {
	_, err := Generate("")
	if err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("Generate with empty secret: want ErrSecretRequired, got %v", err)
	}
}

func TestValidateFormat(t *testing.T) {
	cases := []struct {
		key  string
		want bool
	}{
		{"ABCD-1234-EF56-7890", true},
		{"abcd-1234-ef56-7890", false}, // lowercase not accepted
		{"ABCD-1234-EF56", false},      // too few segments
		{"ABCD-1234-EF56-7890-EXTRA", false},
		{"ABCDX-1234-EF56-7890", false}, // wrong segment length
		{"ABCG-1234-EF56-7890", false},  // G is not hex
		{"", false},
	}
	for _, tc := range cases {
		if got := ValidateFormat(tc.key); got != tc.want {
			t.Fatalf("ValidateFormat(%q): got %v want %v", tc.key, got, tc.want)
		}
	}
}
