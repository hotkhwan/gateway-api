// internal/services/mappingsuggestionsvc/service.go
package mappingsuggestionsvc

import (
	"encoding/json"
	"io/fs"
	"strings"

	mappingsuggestions "github.com/hotkhwan/gateway-api/configs/ingest/mappingsuggestions"
	"github.com/hotkhwan/gateway-api/models/ingestmod"
)

// MappingSuggestionService loads suggestions from embedded JSON files.
// In-memory, read-only — no DB dependency.
type MappingSuggestionService struct {
	all    []*ingestmod.MappingSuggestion
	byFamily map[string][]*ingestmod.MappingSuggestion
}

func NewMappingSuggestionService() *MappingSuggestionService {
	svc := &MappingSuggestionService{
		byFamily: make(map[string][]*ingestmod.MappingSuggestion),
	}
	svc.load()
	return svc
}

// load walks the embedded FS and parses every *.json file.
func (s *MappingSuggestionService) load() {
	_ = fs.WalkDir(mappingsuggestions.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}
		data, readErr := mappingsuggestions.FS.ReadFile(path)
		if readErr != nil {
			return nil
		}
		var suggestion ingestmod.MappingSuggestion
		if jsonErr := json.Unmarshal(data, &suggestion); jsonErr != nil {
			return nil
		}
		s.all = append(s.all, &suggestion)
		s.byFamily[suggestion.SourceFamily] = append(s.byFamily[suggestion.SourceFamily], &suggestion)
		return nil
	})
}

// List returns all suggestions across all source families.
func (s *MappingSuggestionService) List() []*ingestmod.MappingSuggestion {
	return s.all
}

// GetByFamily returns suggestions for a specific sourceFamily.
func (s *MappingSuggestionService) GetByFamily(sourceFamily string) []*ingestmod.MappingSuggestion {
	return s.byFamily[sourceFamily]
}

// GetByID returns a single suggestion by ID, or nil if not found.
func (s *MappingSuggestionService) GetByID(id string) *ingestmod.MappingSuggestion {
	for _, s := range s.all {
		if s.ID == id {
			return s
		}
	}
	return nil
}
