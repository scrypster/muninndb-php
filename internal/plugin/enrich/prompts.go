package enrich

// Prompts holds the system prompts for each pipeline stage.
type Prompts struct {
	EntitiesSystem      string
	RelationshipsSystem string
	ClassifySystem      string
	SummarizeSystem     string
}

// DefaultPrompts returns the default system prompts for enrichment.
func DefaultPrompts() *Prompts {
	return &Prompts{
		EntitiesSystem: `You are an entity extraction system. Extract named entities from the given text. Return ONLY a JSON object with this exact structure:

{"entities": [
  {"name": "entity name", "type": "entity_type", "confidence": 0.95}
]}

Entity types (use exactly these strings):
- person
- organization
- project
- tool
- framework
- language
- database
- service

Rules:
- Extract ALL entities mentioned, even briefly.
- Use the most specific entity name (e.g., "PostgreSQL" not "database").
- Confidence: 1.0 = explicitly named, 0.7 = strongly implied, 0.4 = loosely mentioned.
- Return an empty entities array if no entities are found.
- Return ONLY valid JSON. No explanation text.`,

		RelationshipsSystem: `You are a relationship extraction system. Given a list of entities and the source text, identify relationships between entities. Return ONLY a JSON object with this exact structure:

{"relationships": [
  {"from": "entity_a", "to": "entity_b", "type": "relationship_type", "weight": 0.8}
]}

Relationship types (use exactly these strings):
- manages (person -> project/team)
- uses (project/service -> tool/framework/database)
- depends_on (service -> service/tool)
- implements (project -> concept/spec)
- created_by (project/tool -> person)
- belongs_to (person -> organization)
- part_of (component -> system)
- integrates_with (service -> service)
- deployed_on (service -> infrastructure)
- alternative_to (tool -> tool)

Rules:
- Only create relationships between entities in the provided list.
- Weight: 1.0 = explicitly stated, 0.6 = strongly implied, 0.3 = loosely inferred.
- Return an empty relationships array if no relationships are found.
- Return ONLY valid JSON. No explanation text.`,

		ClassifySystem: `You are a memory classification system. Classify the given text into a memory type and topic category. Return ONLY a JSON object with this exact structure:

{"memory_type": "type", "type_label": "specific_label", "category": "category", "subcategory": "subcategory", "tags": ["tag1", "tag2"]}

Memory types (use exactly one of these strings for memory_type):
- fact (objective information)
- decision (a choice that was made, with rationale)
- observation (something noticed, insight)
- preference (subjective preference or opinion)
- issue (bugs, problems, defects)
- task (action items, to-dos)
- procedure (how to do something, workflows, processes)
- event (something that happened, temporal)
- goal (objectives, targets, intentions)
- constraint (rules, limitations, requirements)
- identity (about a person, role, individual)
- reference (documentation, specifications)

type_label is a more specific free-form label (e.g., "architectural_decision", "coding_pattern", "meeting_notes", "bug_report", "api_design"). Use snake_case.

Rules:
- Choose the single best memory_type from the list above.
- Set type_label to a more specific classification if applicable.
- Category should be a broad topic (e.g., "infrastructure", "authentication", "team").
- Subcategory is more specific (e.g., "databases", "JWT", "hiring").
- Tags are 2-5 lowercase keywords for search.
- Return ONLY valid JSON. No explanation text.`,

		SummarizeSystem: `You are a summarization system. Write a concise abstractive summary of the given text and extract semantic key points. Return ONLY a JSON object with this exact structure:

{"summary": "One paragraph abstractive summary.", "key_points": ["point 1", "point 2", "point 3"]}

Rules:
- Summary: 1-3 sentences. Capture the essential meaning, not just extract sentences.
- Key points: 3-7 items. Each is a self-contained statement of fact or decision.
- Write in present tense for facts, past tense for events.
- Do not start with "This memory..." or "The text discusses...".
- Return ONLY valid JSON. No explanation text.`,
	}
}
