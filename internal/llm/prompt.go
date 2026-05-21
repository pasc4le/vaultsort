package llm

import (
	"fmt"
	"net/http"
	"strings"
)

// PromptInput holds data for constructing a prompt from a template.
type PromptInput struct {
	Filename     string
	FileContents []byte // raw file bytes — sent as actual file part
	VaultDir     string
	Template     string
}

// detectMIME detects the MIME type of raw bytes.
func detectMIME(data []byte) string {
	if len(data) == 0 {
		return "text/plain"
	}
	// Use net/http's DetectContentType for the first 512 bytes
	sniff := data
	if len(sniff) > 512 {
		sniff = data[:512]
	}
	mime := http.DetectContentType(sniff)
	// Trim parameters (e.g. "text/plain; charset=utf-8" → "text/plain")
	if idx := strings.Index(mime, ";"); idx >= 0 {
		mime = strings.TrimSpace(mime[:idx])
	}
	return mime
}

// BuildPrompt constructs system and user messages.
// If FileContents is provided, it is sent as an actual file part (base64)
// rather than string interpolation into the prompt text.
func BuildPrompt(input PromptInput) []Message {
	systemMsg := Message{
		Role:    "system",
		Content: "You are a file organization assistant. Respond only with valid JSON.",
	}

	// Build user prompt text from template
	prompt := input.Template
	prompt = strings.ReplaceAll(prompt, "{{filename}}", input.Filename)
	prompt = strings.ReplaceAll(prompt, "{{vault_dir}}", input.VaultDir)

	if len(input.FileContents) > 0 {
		// Replace placeholder with accurate description
		prompt = strings.ReplaceAll(prompt, "{{file_contents}}", "(file attached below)")

		mimeType := detectMIME(input.FileContents)
		// Limit to 10KB for LLM context (most providers have context limits)
		limit := 10240
		fileData := input.FileContents
		if len(fileData) > limit {
			fileData = fileData[:limit]
		}

		userMsg := WithParts("user", []MessagePart{
			TextPart(prompt),
			FilePart(input.Filename, fileData, mimeType),
		})
		return []Message{systemMsg, userMsg}
	}

	// No file content — remove misleading placeholder
	prompt = strings.ReplaceAll(prompt, "{{file_contents}}", "(file content not available)")

	userMsg := Message{
		Role:    "user",
		Content: prompt,
	}
	return []Message{systemMsg, userMsg}
}

// PromptForRule generates a human-readable summary of a rule for logging.
func PromptForRule(name, filename, vaultDir string, sendFile bool) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Rule: %s\n", name))
	b.WriteString(fmt.Sprintf("File: %s\n", filename))
	b.WriteString(fmt.Sprintf("Vault: %s\n", vaultDir))
	b.WriteString(fmt.Sprintf("Send file content: %v\n", sendFile))
	return b.String()
}
