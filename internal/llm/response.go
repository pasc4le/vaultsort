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
// With structured output, the response is already valid JSON.
// Falls back to extraction from markdown/text for non-structured providers.
func ParseResponse(text string) (*LLMResponse, error) {
	// Fast path: try direct parse (structured output returns clean JSON)
	if strings.HasPrefix(strings.TrimSpace(text), "{") {
		var resp LLMResponse
		if err := json.Unmarshal([]byte(text), &resp); err == nil {
			if err := validateResponse(&resp); err != nil {
				return nil, err
			}
			return &resp, nil
		}
	}

	// Fallback: extract JSON from markdown or text
	jsonStr := extractJSON(text)
	if jsonStr == "" {
		return nil, fmt.Errorf("no JSON found in LLM response")
	}

	var resp LLMResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		return nil, fmt.Errorf("parse JSON: %w", err)
	}

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
					return candidate
				}
			}
		}
	}

	return ""
}

// validateResponse checks that the LLM response meets safety requirements.
func validateResponse(resp *LLMResponse) error {
	if resp.Filename == "" {
		return fmt.Errorf("LLM response has empty filename")
	}
	if resp.Subdir == "" {
		return fmt.Errorf("LLM response has empty subdir")
	}

	resp.Filename = sanitizeFilename(resp.Filename)
	if resp.Filename == "" {
		return fmt.Errorf("filename is empty after sanitization")
	}

	resp.Filename = ensureExtension(resp.Filename)

	resp.Subdir = strings.TrimPrefix(resp.Subdir, "/")
	resp.Subdir = strings.TrimSuffix(resp.Subdir, "/")

	if strings.Contains(resp.Subdir, "..") || strings.Contains(resp.Filename, "..") {
		return fmt.Errorf("path traversal detected: %s / %s", resp.Subdir, resp.Filename)
	}

	if filepath.IsAbs(resp.Subdir) {
		return fmt.Errorf("subdir is absolute path: %s", resp.Subdir)
	}

	return nil
}

// sanitizeFilename removes dangerous characters from a filename.
func sanitizeFilename(name string) string {
	name = strings.ReplaceAll(name, "\x00", "")
	name = strings.ReplaceAll(name, "/", "")
	name = strings.ReplaceAll(name, "\\", "")
	re := regexp.MustCompile(`[[:cntrl:]]`)
	name = re.ReplaceAllString(name, "")
	name = strings.TrimSpace(name)
	name = strings.TrimRight(name, ".")
	return name
}

// ensureExtension makes sure the filename has an extension.
func ensureExtension(name string) string {
	if filepath.Ext(name) != "" {
		return name
	}
	return name
}
