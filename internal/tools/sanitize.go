package tools

import (
	"regexp"
	"strings"
)

// redactedPlaceholder replaces sensitive values in error messages exposed at
// HTTP or MCP boundaries.
const redactedPlaceholder = "[REDACTED]"

// bearerPattern matches "Bearer <token>" (case-insensitive).
var bearerPattern = regexp.MustCompile(`(?i)Bearer\s+\S+`)

// skPattern matches sk-... API key literals (OpenAI-style and similar).
var skPattern = regexp.MustCompile(`sk-[a-zA-Z0-9]+`)

// apiKeyPattern matches generic API key query/header literals, e.g.
// "api_key=<value>" or "apikey=<value>".
var apiKeyPattern = regexp.MustCompile(`(?i)api[_-]?key\s*=\s*\S+`)

// SanitizeError returns a scrubbed string representation of err suitable for
// exposure at HTTP response bodies and MCP tool outputs. It removes:
//   - Bearer tokens
//   - sk-... API key literals
//   - api_key=... / apikey=... literals
//
// Returns an empty string when err is nil.
func SanitizeError(err error) string {
	if err == nil {
		return ""
	}
	return scrubSensitive(err.Error())
}

// scrubSensitive applies all redaction patterns to msg.
func scrubSensitive(msg string) string {
	result := msg

	// Scrub "Bearer <token>" patterns.
	result = bearerPattern.ReplaceAllStringFunc(result, func(match string) string {
		// Preserve the "Bearer " prefix for readability.
		idx := strings.IndexFunc(match, func(r rune) bool { return r == ' ' || r == '\t' })
		if idx < 0 {
			return "Bearer " + redactedPlaceholder
		}
		return match[:idx+1] + redactedPlaceholder
	})

	// Scrub sk-... API key literals.
	result = skPattern.ReplaceAllString(result, redactedPlaceholder)

	// Scrub api_key=... / apikey=... literals.
	result = apiKeyPattern.ReplaceAllStringFunc(result, func(match string) string {
		eqIdx := strings.IndexByte(match, '=')
		if eqIdx < 0 {
			return redactedPlaceholder
		}
		return match[:eqIdx+1] + redactedPlaceholder
	})

	return result
}
