// internal/services/aimappingsvc/prompt.go
package aimappingsvc

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

const aiSuggestPromptVersion = "v1.0"

const (
	maxEnumContextFields    = 20
	maxEnumContextBytesPerField = 2048
)

// EnumContextBundle is field path → valueCodes map.
type EnumContextBundle struct {
	FieldDictionaries map[string]map[string]string
}

// resolveEnumContext resolves enum codes for observed paths from existing system suggestions.
// Returns bundle with max 20 fields, max 2KB per field dictionary.
func resolveEnumContext(suggestions []*ingestmod.MappingSuggestion, observedPaths []string, sourceFamily string) EnumContextBundle {
	bundle := EnumContextBundle{
		FieldDictionaries: make(map[string]map[string]string),
	}

	pathSet := make(map[string]bool, len(observedPaths))
	for _, p := range observedPaths {
		pathSet[p] = true
	}

	for _, sug := range suggestions {
		if sug == nil {
			continue
		}
		for _, fm := range sug.FieldMappings {
			if len(fm.ValueCodes) == 0 {
				continue
			}
			// Only include if the source field is among observed paths.
			if !pathSet[fm.SourceField] {
				continue
			}
			if len(bundle.FieldDictionaries) >= maxEnumContextFields {
				break
			}
			// Enforce per-field byte budget.
			encoded, err := json.Marshal(fm.ValueCodes)
			if err != nil {
				continue
			}
			if len(encoded) > maxEnumContextBytesPerField {
				// Trim the map to fit.
				trimmed := trimValueCodes(fm.ValueCodes, maxEnumContextBytesPerField)
				bundle.FieldDictionaries[fm.SourceField] = trimmed
			} else {
				bundle.FieldDictionaries[fm.SourceField] = fm.ValueCodes
			}
		}
	}

	return bundle
}

// trimValueCodes removes entries from a valueCodes map until it fits within maxBytes.
func trimValueCodes(vc map[string]string, maxBytes int) map[string]string {
	out := make(map[string]string, len(vc))
	for k, v := range vc {
		out[k] = v
	}
	for len(out) > 0 {
		encoded, err := json.Marshal(out)
		if err != nil || len(encoded) <= maxBytes {
			break
		}
		for k := range out {
			delete(out, k)
			break
		}
	}
	return out
}

// buildPrompt creates the full prompt string to send to the AI provider.
// It includes: task instructions, payload structure, observed paths,
// enum context (if any), existing system suggestions, and required JSON schema.
func buildPrompt(
	sourceFamily string,
	reduced ReduceResult,
	enumBundle EnumContextBundle,
	systemMappings []SuggestionFieldMap,
) string {
	var sb strings.Builder

	sb.WriteString("You are an expert IoT data integration assistant.\n\n")
	sb.WriteString("## Task\n")
	sb.WriteString("Analyze the payload structure below and return a JSON mapping suggestion.\n\n")

	// Source family context.
	fmt.Fprintf(&sb, "## Source Family\n%s\n\n", sourceFamily)

	// Reduced payload.
	payloadJSON, _ := json.MarshalIndent(reduced.ReducedPayload, "", "  ")
	sb.WriteString("## Payload Structure\n```json\n")
	sb.Write(payloadJSON)
	sb.WriteString("\n```\n\n")

	// Observed paths.
	sb.WriteString("## Observed Field Paths\n")
	for _, p := range reduced.ObservedPaths {
		fmt.Fprintf(&sb, "- %s\n", p)
	}
	sb.WriteString("\n")

	// Enum context (if any).
	if len(enumBundle.FieldDictionaries) > 0 {
		sb.WriteString("## Known Value Code Dictionaries\n")
		sb.WriteString("These are known value-code → label mappings for specific fields:\n")
		for field, codes := range enumBundle.FieldDictionaries {
			codesJSON, _ := json.Marshal(codes)
			fmt.Fprintf(&sb, "- %s: %s\n", field, string(codesJSON))
		}
		sb.WriteString("\n")
	}

	// Existing system suggestions for reference.
	if len(systemMappings) > 0 {
		sb.WriteString("## Existing System Field Mappings (for reference)\n")
		for _, fm := range systemMappings {
			fmt.Fprintf(&sb, "- sourceField=%q → targetField=%q", fm.SourceField, fm.TargetField)
			if fm.Source != "" {
				fmt.Fprintf(&sb, " (source=%s)", fm.Source)
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	// Instructions.
	sb.WriteString("## Instructions\n")
	sb.WriteString("1. Suggest canonical field mappings: sourceField (from observed paths) → targetField (snake_case canonical name).\n")
	sb.WriteString("2. Suggest match rules to uniquely identify this payload type (fieldPath, operator, value).\n")
	sb.WriteString("   - Operators allowed: \"eq\", \"exists\", \"contains\"\n")
	sb.WriteString("   - Prefer specific field+value rules over generic \"exists\" alone.\n")
	sb.WriteString("3. Suggest a suggestedEventType in UPPER_SNAKE_CASE (e.g. DEVICE_HEARTBEAT).\n")
	sb.WriteString("4. For fields with numeric/string codes, include valueCodes map where you can infer the meaning.\n")
	sb.WriteString("5. Return ONLY valid JSON — no markdown, no explanation, no extra keys.\n\n")

	// Required JSON schema.
	sb.WriteString("## Required JSON Output Schema\n```json\n")
	sb.WriteString(`{
  "suggestedEventType": "UPPER_SNAKE_CASE string",
  "fieldMappings": [
    {
      "sourceField": "observed.path",
      "targetField": "snake_case_target",
      "valueCodes": { "0": "label0", "1": "label1" }
    }
  ],
  "matchRules": [
    {
      "fieldPath": "observed.path",
      "operator": "eq | exists | contains",
      "value": "optional match value",
      "required": true,
      "reason": "why this rule helps identify the payload"
    }
  ]
}`)
	sb.WriteString("\n```\n\n")
	sb.WriteString("Return only the JSON object. Do not wrap it in markdown code blocks.\n")

	return sb.String()
}
