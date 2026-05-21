package llm

import (
	"fmt"
	"strings"
)

// PromptInput holds data for constructing a prompt from a template.
type PromptInput struct {
	Filename     string
	FileContents []byte // may be nil
	VaultDir     string
	Template     string
}

// BuildPrompt constructs system and user messages from a template.
// It replaces {{filename}}, {{file_contents}}, and {{vault_dir}} placeholders.
func BuildPrompt(input PromptInput) []Message {
	// Determine content excerpt
	var contentStr string
	if len(input.FileContents) > 0 {
		// Limit to first 10KB for LLM context
		limit := 10240
		if len(input.FileContents) < limit {
			limit = len(input.FileContents)
		}
		contentStr = string(input.FileContents[:limit])
	}

	// Replace placeholders in the user prompt template
	prompt := input.Template
	prompt = strings.ReplaceAll(prompt, "{{filename}}", input.Filename)
	prompt = strings.ReplaceAll(prompt, "{{vault_dir}}", input.VaultDir)
	if contentStr != "" {
		prompt = strings.ReplaceAll(prompt, "{{file_contents}}", contentStr)
	} else {
		prompt = strings.ReplaceAll(prompt, "{{file_contents}}", "(file contents not available)")
	}

	// Build system message
	systemMsg := Message{
		Role:    "system",
		Content: "You are a file organization assistant. Respond only with valid JSON.",
	}

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
