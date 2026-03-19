package testutil

// ValidRoadmapItemJSON is a complete, valid roadmap item with all required fields.
// Use this fixture when testing successful JSON parsing and validation.
// Contains: slug, title, priority (high), status (planned), and context.
const ValidRoadmapItemJSON = `{
  "slug": "test-feature",
  "title": "Test Feature",
  "priority": "high",
  "status": "planned",
  "context": "This is a test feature for validation"
}`

// InvalidJSON is malformed JSON that cannot be parsed.
// Use this fixture to test JSON parsing error handling.
// Missing closing brace causes parse failure.
const InvalidJSON = `{
  "slug": "invalid",
  "title": "Invalid Item",
  "priority": "high"
`

// EmptyResponse represents an empty AI tool response.
// Use this fixture to test handling of empty output from AI tools.
const EmptyResponse = ``

// RoadmapItemMissingFields is valid JSON but missing required roadmap fields.
// Use this fixture to test validation of required fields (missing context, status, priority).
const RoadmapItemMissingFields = `{
  "slug": "incomplete-item",
  "title": "Incomplete Item"
}`

// HighConfidenceMatch represents a fuzzy match result with high confidence (>0.8).
// Use this fixture to test AI-based fuzzy matching with auto-accept behavior.
// High confidence matches should be automatically accepted in --yes mode.
const HighConfidenceMatch = `{
  "matches": [
    {
      "source_slug": "new-feature",
      "target_slug": "old-feature",
      "confidence": 0.95,
      "reasoning": "Strong semantic match based on title and context similarity"
    }
  ]
}`

// LowConfidenceMatch represents a fuzzy match result with low confidence (<0.5).
// Use this fixture to test AI-based fuzzy matching requiring user confirmation.
// Low confidence matches should prompt for confirmation in interactive mode.
const LowConfidenceMatch = `{
  "matches": [
    {
      "source_slug": "feature-a",
      "target_slug": "feature-b",
      "confidence": 0.35,
      "reasoning": "Weak match - only partial keyword overlap"
    }
  ]
}`

// MultipleMatches represents multiple fuzzy match candidates.
// Use this fixture to test handling of multiple AI-suggested matches.
// Tests should verify disambiguation logic and user prompting.
const MultipleMatches = `{
  "matches": [
    {
      "source_slug": "api-feature",
      "target_slug": "api-v1",
      "confidence": 0.82,
      "reasoning": "High match on API-related content"
    },
    {
      "source_slug": "api-feature",
      "target_slug": "api-v2",
      "confidence": 0.78,
      "reasoning": "Moderate match on API context"
    }
  ]
}`

// ToolNotFoundError simulates an AI tool execution error when the tool binary is not found.
// Use this fixture to test error handling when the AI tool is missing from PATH.
const ToolNotFoundError = `AI tool not found in PATH. Please ensure the tool is installed and accessible.`

// TimeoutError simulates an AI tool execution timeout.
// Use this fixture to test handling of long-running AI operations that exceed time limits.
const TimeoutError = `tool execution timed out after 60s`

// ParseError simulates an error message for unparseable AI tool output.
// Use this fixture to test error handling when AI returns non-JSON or invalid format.
const ParseError = `failed to parse AI response: invalid character 'i' looking for beginning of value`
