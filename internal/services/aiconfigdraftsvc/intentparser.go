// internal/services/aiconfigdraftsvc/intentparser.go
package aiconfigdraftsvc

import (
	"regexp"
	"strings"
	"time"
	"unicode"
)

var (
	// highEntropyRe matches long base64-like strings (potential tokens/keys).
	highEntropyRe = regexp.MustCompile(`[A-Za-z0-9+/=_\-]{40,}`)
	// discordWebhookRe matches Discord webhook URLs.
	discordWebhookRe = regexp.MustCompile(`https://discord(?:app)?\.com/api/webhooks/[^\s]+`)
	// lineTokenRe matches LINE channel access tokens (170+ characters).
	lineTokenRe = regexp.MustCompile(`[A-Za-z0-9+/]{170,}`)
	// conditionRe matches patterns like "alarm = X", "type is X", "when X equals Y".
	conditionRe = regexp.MustCompile(`(?i)(?:when\s+)?(\w[\w.]*)\s+(?:=|is|equals?|==)\s+([^\s,;]+)`)
	// knownFamilies is the list of recognized source family keywords.
	knownFamilies = []string{"AIBOX", "PVS", "KLYNX", "HIKVISION", "DAHUA"}
	// actionKeywords maps action keywords to action types.
	actionKeywords = map[string]string{
		"webhook": "webhook",
		"line":    "line",
		"discord": "discord",
		"mqtt":    "mqtt",
	}
)

// parseIntent parses a natural-language prompt into a DraftIntent.
// Extracts: sourceFamily hints, field conditions, delivery targets.
// Masks any token/credential patterns before storing in RawPrompt.
func parseIntent(prompt string) DraftIntent {
	intent := DraftIntent{
		RawPrompt: maskCredentials(prompt),
		ParsedAt:  time.Now().UTC(),
	}

	upper := strings.ToUpper(prompt)

	// Detect source family.
	for _, fam := range knownFamilies {
		if strings.Contains(upper, fam) {
			intent.SourceFamily = fam
			break
		}
	}

	// Detect conditions.
	matches := conditionRe.FindAllStringSubmatch(prompt, -1)
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		cond := IntentCondition{
			RawPhrase: m[0],
			FieldHint: m[1],
			Operator:  "eq",
			ValueHint: m[2],
		}
		intent.Conditions = append(intent.Conditions, cond)
	}

	// Detect delivery actions.
	lower := strings.ToLower(prompt)
	for keyword, actionType := range actionKeywords {
		if strings.Contains(lower, keyword) {
			action := IntentAction{
				Type:      actionType,
				RawTarget: extractMaskedTarget(prompt, keyword),
			}
			intent.DeliveryActions = append(intent.DeliveryActions, action)
		}
	}

	return intent
}

// maskCredentials replaces LINE tokens, Discord webhook URLs, and high-entropy
// strings in the prompt with placeholder text before storage.
func maskCredentials(prompt string) string {
	// Mask LINE tokens first (very long base64-like strings).
	masked := lineTokenRe.ReplaceAllStringFunc(prompt, func(s string) string {
		if len(s) >= 170 {
			return "[REDACTED_TOKEN]"
		}
		return s
	})
	// Mask Discord webhook URLs.
	masked = discordWebhookRe.ReplaceAllString(masked, "[REDACTED_DISCORD_WEBHOOK]")
	// Mask remaining high-entropy strings.
	masked = highEntropyRe.ReplaceAllStringFunc(masked, func(s string) string {
		if isHighEntropy(s) && len(s) >= 40 {
			return "[REDACTED_SECRET]"
		}
		return s
	})
	return masked
}

// isHighEntropy returns true when the string has high character-variety typical
// of secrets (does not look like a plain English word or URL path segment).
func isHighEntropy(s string) bool {
	var digits, uppers, lowers, specials int
	for _, r := range s {
		switch {
		case unicode.IsDigit(r):
			digits++
		case unicode.IsUpper(r):
			uppers++
		case unicode.IsLower(r):
			lowers++
		default:
			specials++
		}
	}
	total := len(s)
	if total == 0 {
		return false
	}
	// Consider high-entropy when at least 3 character classes are present and
	// no single class dominates (>90 %).
	classes := 0
	if digits > 0 {
		classes++
	}
	if uppers > 0 {
		classes++
	}
	if lowers > 0 {
		classes++
	}
	if specials > 0 {
		classes++
	}
	return classes >= 3
}

// extractMaskedTarget attempts to pull out the raw target value for an action
// keyword from the prompt. It returns an empty string if none is found.
func extractMaskedTarget(prompt, keyword string) string {
	lower := strings.ToLower(prompt)
	idx := strings.Index(lower, keyword)
	if idx == -1 {
		return ""
	}
	// Take up to 120 characters after the keyword as context and mask it.
	end := idx + len(keyword) + 120
	if end > len(prompt) {
		end = len(prompt)
	}
	snippet := prompt[idx:end]
	return maskCredentials(snippet)
}
