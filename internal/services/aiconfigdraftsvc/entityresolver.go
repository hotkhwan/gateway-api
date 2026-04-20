// internal/services/aiconfigdraftsvc/entityresolver.go
package aiconfigdraftsvc

import (
	"strings"

	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// resolveEntities maps raw field hints and value hints in the intent to canonical
// field paths and resolved values using the provided mapping suggestions.
func resolveEntities(intent *DraftIntent, suggestions []*ingestmod.MappingSuggestion) {
	if intent == nil || len(suggestions) == 0 {
		return
	}

	// Build a lookup: sourceField (lowercased) → SuggestionFieldMap.
	type fieldEntry struct {
		sourcePath string
		targetPath string
		valueCodes map[string]string
	}
	fieldIndex := make(map[string]fieldEntry)
	for _, sug := range suggestions {
		if sug == nil {
			continue
		}
		for _, fm := range sug.FieldMappings {
			key := strings.ToLower(fm.SourceField)
			fieldIndex[key] = fieldEntry{
				sourcePath: fm.SourceField,
				targetPath: fm.TargetField,
				valueCodes: fm.ValueCodes,
			}
		}
	}

	for i := range intent.Conditions {
		cond := &intent.Conditions[i]
		hint := strings.ToLower(cond.FieldHint)

		// Direct match.
		if entry, ok := fieldIndex[hint]; ok {
			cond.Resolved = true
			cond.ResolvedPath = entry.sourcePath

			// Attempt to resolve the value code.
			if entry.valueCodes != nil {
				vHint := strings.ToLower(cond.ValueHint)
				for code, label := range entry.valueCodes {
					if strings.EqualFold(label, vHint) || strings.EqualFold(code, vHint) {
						cond.ResolvedValue = code
						break
					}
				}
			}
			if cond.ResolvedValue == nil {
				cond.ResolvedValue = cond.ValueHint
			}
			continue
		}

		// Partial / suffix match: "alarmType" hint → "data.alarmType".
		for key, entry := range fieldIndex {
			if strings.HasSuffix(key, hint) || strings.HasSuffix(hint, key) {
				cond.Resolved = true
				cond.ResolvedPath = entry.sourcePath
				if cond.ResolvedValue == nil {
					cond.ResolvedValue = cond.ValueHint
				}
				break
			}
		}
	}
}
