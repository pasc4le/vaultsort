package llm

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// LLMResponse is the expected JSON structure from the LLM.
type LLMResponse struct {
	Filename string `json:"filename"`
	Subdir   string `json:"subdir"`
}

// ParseResponse extracts a valid LLMResponse from LLM output text.
// Handles JSON in markdown code blocks, plain JSON, and various edge cases.
func ParseResponse(text string) (*LLMResponse, error) {
	jsonStr := extractJSON(text)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in LLM response")
	}

	var resp LLMResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

	// Validate
	if err := validateResponse(&resp); err != nil {
		return nil, err
	}

	return &resp, nil
}

// extractJSON extracts the first JSON object from text.
// Handles markdown code blocks (```json ... ```), nested braces.
func extractJSON(text string) string {
	// Try to find JSON in markdown code blocks first
	re := regexp.MustCompile("```(?:json)?\\s*\n?([\\s\\S]*?)```")
	matches := re.FindStringSubmatch(text)
	if len(matches) >= 2 {
		jsonStr := strings.TrimSpace(matches[1])
		if json.Valid([]byte(jsonStr)) {
			return jsonStr
		}
	}

	// Fallback: find first { and last }
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end > start {
		jsonStr := text[start : end+1]
		if json.Valid([]byte(jsonStr)) {
			return jsonStr
		}
		// Try to fix truncated JSON by finding proper closing
		depth := 0
		for i := start; i < len(text); i++ {
			switch text[i] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					candidate := text[start : i+1]
					if json.Valid([]byte(candidate)) {
						return candidate
					}
					// Still try partial parse
					return candidate
				}
			}
		}
	}

	return ""
}

// validateResponse checks that the LLM response meets safety requirements.
func validateResponse(resp *LLMResponse) error {
	// Must have filename
	if resp.Filename == "" {
		return fmt.Errorf("LLM response has empty filename")
	}

	// Must have subdir
	if resp.Subdir == "" {
		return fmt.Errorf("LLM response has empty subdir")
	}

	// Sanitize filename: remove path separators, control chars
	resp.Filename = sanitizeFilename(resp.Filename)
	if resp.Filename == "" {
		return fmt.Errorf("filename is empty after sanitization")
	}

	// Ensure extension is preserved
	resp.Filename = ensureExtension(resp.Filename)

	// Subdir must be relative (no leading /)
	resp.Subdir = strings.TrimPrefix(resp.Subdir, "/")
	resp.Subdir = strings.TrimSuffix(resp.Subdir, "/")

	// Reject path traversal
	if strings.Contains(resp.Subdir, "..") || strings.Contains(resp.Filename, "..") {
		return fmt.Errorf("path traversal detected: %s / %s", resp.Subdir, resp.Filename)
	}

	// Subdir must not be absolute
	if filepath.IsAbs(resp.Subdir) {
		return fmt.Errorf("subdir is absolute path: %s", resp.Subdir)
	}

	return nil
}

// sanitizeFilename removes dangerous characters from a filename.
func sanitizeFilename(name string) string {
	// Remove null bytes
	name = strings.ReplaceAll(name, "\x00", "")
	// Remove path separators
	name = strings.ReplaceAll(name, "/", "")
	name = strings.ReplaceAll(name, "\\", "")
	// Remove control characters (except printable)
	re := regexp.MustCompile(`[[:cntrl:]]`)
	name = re.ReplaceAllString(name, "")
	// Trim whitespace and dots
	name = strings.TrimSpace(name)
	name = strings.TrimRight(name, ".")
	return name
}

// ensureExtension makes sure the filename has an extension.
// If not, returns the original name (don't guess).
func ensureExtension(name string) string {
	if filepath.Ext(name) != "" {
		return name
	}
	return name
}
