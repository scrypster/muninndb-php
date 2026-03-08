package enrich

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/scrypster/muninndb/internal/plugin"
)

// extractJSON finds and returns the first valid JSON structure in a string.
// Handles markdown code fences and trailing text.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)

	// Remove markdown code fences if present
	if strings.Contains(s, "```json") {
		start := strings.Index(s, "```json")
		end := strings.Index(s[start+7:], "```")
		if end != -1 {
			s = s[start+7 : start+7+end]
			s = strings.TrimSpace(s)
		}
	} else if strings.Contains(s, "```") {
		start := strings.Index(s, "```")
		end := strings.Index(s[start+3:], "```")
		if end != -1 {
			s = s[start+3 : start+3+end]
			s = strings.TrimSpace(s)
		}
	}

	// Find first [ or {
	start := strings.IndexAny(s, "[{")
	if start < 0 {
		return s
	}

	// Find matching end from the end of string backwards
	for i := len(s) - 1; i >= start; i-- {
		if s[i] == ']' || s[i] == '}' {
			return strings.TrimSpace(s[start : i+1])
		}
	}

	return s[start:]
}

// ParseEntityResponse parses the JSON response from the entity extraction call.
func ParseEntityResponse(raw string) ([]plugin.ExtractedEntity, error) {
	raw = strings.TrimSpace(raw)
	jsonStr := extractJSON(raw)

	// Try to parse as wrapper object with "entities" key
	var wrapper struct {
		Entities []plugin.ExtractedEntity `json:"entities"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &wrapper); err == nil {
		if wrapper.Entities == nil && strings.Contains(jsonStr, `"entities"`) {
			return nil, nil
		}
		if wrapper.Entities != nil {
			// Validate and deduplicate
			return validateAndDedupeEntities(wrapper.Entities), nil
		}
	} else {
		// Fall through to direct-array parsing before surfacing the error.
	}

	// Try to parse as direct array
	var entities []plugin.ExtractedEntity
	if err := json.Unmarshal([]byte(jsonStr), &entities); err == nil {
		return validateAndDedupeEntities(entities), nil
	}

	return nil, fmt.Errorf("invalid entity response JSON: %s", truncateForError(jsonStr))
}

// ParseRelationshipResponse parses the JSON response from the relationship extraction call.
func ParseRelationshipResponse(raw string) ([]plugin.ExtractedRelation, error) {
	raw = strings.TrimSpace(raw)
	jsonStr := extractJSON(raw)

	// Try to parse as wrapper object with "relationships" key
	// Use intermediate struct with "type" field for JSON parsing
	var wrapper struct {
		Relationships []struct {
			From   string  `json:"from"`
			To     string  `json:"to"`
			Type   string  `json:"type"`
			Weight float32 `json:"weight"`
		} `json:"relationships"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &wrapper); err == nil {
		if wrapper.Relationships == nil && strings.Contains(jsonStr, `"relationships"`) {
			return nil, nil
		}
		if wrapper.Relationships != nil {
			var result []plugin.ExtractedRelation
			for _, rel := range wrapper.Relationships {
				result = append(result, plugin.ExtractedRelation{
					FromEntity: rel.From,
					ToEntity:   rel.To,
					RelType:    rel.Type,
					Weight:     rel.Weight,
				})
			}
			return validateRelationships(result), nil
		}
	}

	// Try to parse as direct array
	var rawRels []struct {
		From   string  `json:"from"`
		To     string  `json:"to"`
		Type   string  `json:"type"`
		Weight float32 `json:"weight"`
	}
	if err := json.Unmarshal([]byte(jsonStr), &rawRels); err == nil {
		var result []plugin.ExtractedRelation
		for _, rel := range rawRels {
			result = append(result, plugin.ExtractedRelation{
				FromEntity: rel.From,
				ToEntity:   rel.To,
				RelType:    rel.Type,
				Weight:     rel.Weight,
			})
		}
		return validateRelationships(result), nil
	}

	return nil, fmt.Errorf("invalid relationship response JSON: %s", truncateForError(jsonStr))
}

// ParseClassificationResponse parses the JSON response from the classification call.
func ParseClassificationResponse(raw string) (memType, typeLabel, category, subcategory string, tags []string, err error) {
	raw = strings.TrimSpace(raw)
	jsonStr := extractJSON(raw)

	var result struct {
		MemoryType  string   `json:"memory_type"`
		TypeLabel   string   `json:"type_label"`
		Category    string   `json:"category"`
		Subcategory string   `json:"subcategory"`
		Tags        []string `json:"tags"`
	}

	err = json.Unmarshal([]byte(jsonStr), &result)
	if err != nil {
		return "", "", "", "", nil, fmt.Errorf("invalid classification response JSON: %s", truncateForError(jsonStr))
	}
	if result.MemoryType == "" && result.TypeLabel == "" && result.Category == "" && result.Subcategory == "" && len(result.Tags) == 0 {
		return "", "", "", "", nil, fmt.Errorf("classification response was empty")
	}

	return result.MemoryType, result.TypeLabel, result.Category, result.Subcategory, result.Tags, nil
}

// ParseSummarizeResponse parses the JSON response from the summarization call.
func ParseSummarizeResponse(raw string) (summary string, keyPoints []string, err error) {
	raw = strings.TrimSpace(raw)
	jsonStr := extractJSON(raw)

	var result struct {
		Summary   string   `json:"summary"`
		KeyPoints []string `json:"key_points"`
	}

	err = json.Unmarshal([]byte(jsonStr), &result)
	if err != nil {
		return "", nil, fmt.Errorf("invalid summarize response JSON: %s", truncateForError(jsonStr))
	}
	if result.Summary == "" && len(result.KeyPoints) == 0 {
		return "", nil, fmt.Errorf("summarize response was empty")
	}

	return result.Summary, result.KeyPoints, nil
}

func truncateForError(s string) string {
	const maxLen = 160
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// validateAndDedupeEntities validates entity fields and removes duplicates (keeping highest confidence).
func validateAndDedupeEntities(entities []plugin.ExtractedEntity) []plugin.ExtractedEntity {
	seen := make(map[string]plugin.ExtractedEntity)

	for _, e := range entities {
		e.Name = strings.TrimSpace(e.Name)
		e.Type = strings.TrimSpace(e.Type)

		// Skip empty names
		if e.Name == "" {
			continue
		}

		// Validate and normalize type
		e.Type = normalizeEntityType(e.Type)

		// Clamp confidence to [0.0, 1.0]
		if e.Confidence < 0.0 {
			e.Confidence = 0.0
		} else if e.Confidence > 1.0 {
			e.Confidence = 1.0
		}

		// Keep highest confidence for duplicates
		if existing, ok := seen[e.Name]; ok {
			if e.Confidence > existing.Confidence {
				seen[e.Name] = e
			}
		} else {
			seen[e.Name] = e
		}
	}

	result := make([]plugin.ExtractedEntity, 0, len(seen))
	for _, e := range seen {
		result = append(result, e)
	}

	return result
}

// normalizeEntityType validates and normalizes entity type strings.
func normalizeEntityType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))

	validTypes := map[string]bool{
		"person":       true,
		"organization": true,
		"project":      true,
		"tool":         true,
		"framework":    true,
		"language":     true,
		"database":     true,
		"service":      true,
	}

	if validTypes[t] {
		return t
	}

	// Default to "service" for unknown types
	return "service"
}

// validateRelationships validates relationship fields.
func validateRelationships(rels []plugin.ExtractedRelation) []plugin.ExtractedRelation {
	result := make([]plugin.ExtractedRelation, 0, len(rels))

	for _, r := range rels {
		r.FromEntity = strings.TrimSpace(r.FromEntity)
		r.ToEntity = strings.TrimSpace(r.ToEntity)
		r.RelType = strings.TrimSpace(r.RelType)

		// Skip if from or to is empty
		if r.FromEntity == "" || r.ToEntity == "" {
			continue
		}

		// Clamp weight to [0.0, 1.0]
		if r.Weight < 0.0 {
			r.Weight = 0.0
		} else if r.Weight > 1.0 {
			r.Weight = 1.0
		}

		result = append(result, r)
	}

	return result
}
