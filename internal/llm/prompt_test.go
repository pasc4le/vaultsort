package llm

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

// ============================================================
// B4: BuildPrompt tests
// ============================================================

func TestBuildPrompt_WithFileContent(t *testing.T) {
	input := PromptInput{
		Filename:     "photo.jpg",
		FileContents: []byte{0xFF, 0xD8, 0xFF, 0xE0},
		VaultDir:     "/tmp/vault",
		Template:     "Organize {{filename}} from {{vault_dir}}. Contents: {{file_contents}}",
	}

	msgs := BuildPrompt(input)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (system+user), got %d", len(msgs))
	}

	// System message
	if msgs[0].Role != "system" {
		t.Fatalf("expected system role, got %s", msgs[0].Role)
	}

	// User msg should have Parts (multipart)
	if len(msgs[1].Parts) == 0 {
		t.Fatal("expected user message with Parts when FileContents is provided")
	}

	// First part should be text with placeholder replaced
	textPart := msgs[1].Parts[0]
	if textPart.Type != "text" {
		t.Fatalf("expected text type for first part, got %s", textPart.Type)
	}
	if !strings.Contains(textPart.Text, "(file attached below)") {
		t.Fatalf("expected '(file attached below)' placeholder, got: %s", textPart.Text)
	}
	if strings.Contains(textPart.Text, "{{file_contents}}") {
		t.Fatal("{{file_contents}} should be replaced in prompt")
	}

	// Second part should be input_file
	filePart := msgs[1].Parts[1]
	if filePart.Type != "input_file" {
		t.Fatalf("expected input_file type for second part, got %s", filePart.Type)
	}
	if filePart.Filename != "photo.jpg" {
		t.Fatalf("expected filename photo.jpg, got %s", filePart.Filename)
	}
}

func TestBuildPrompt_WithoutFileContent(t *testing.T) {
	input := PromptInput{
		Filename:     "notes.txt",
		FileContents: nil, // no content
		VaultDir:     "/tmp/vault",
		Template:     "Organize {{filename}} from {{vault_dir}}. Contents: {{file_contents}}",
	}

	msgs := BuildPrompt(input)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	// User msg should be plain text (no Parts)
	if len(msgs[1].Parts) > 0 {
		t.Fatal("expected no Parts when FileContents is nil")
	}
	if msgs[1].Content == "" {
		t.Fatal("expected non-empty Content when no Parts")
	}

	// Placeholder should be replaced with "(file content not available)"
	if !strings.Contains(msgs[1].Content, "(file content not available)") {
		t.Fatalf("expected '(file content not available)', got: %s", msgs[1].Content)
	}
	if strings.Contains(msgs[1].Content, "{{file_contents}}") {
		t.Fatal("{{file_contents}} should be replaced")
	}
}

func TestBuildPrompt_EmptyFileContentSlice(t *testing.T) {
	// Empty slice (not nil) should also be treated as no content
	input := PromptInput{
		Filename:     "doc.pdf",
		FileContents: []byte{},
		VaultDir:     "/tmp/vault",
		Template:     "Handle {{filename}}. Content: {{file_contents}}",
	}

	msgs := BuildPrompt(input)
	if len(msgs[1].Parts) > 0 {
		t.Fatal("expected no Parts for empty FileContents")
	}
	if !strings.Contains(msgs[1].Content, "(file content not available)") {
		t.Fatalf("expected '(file content not available)' placeholder, got: %s", msgs[1].Content)
	}
}

func TestBuildPrompt_TemplateSubstitution(t *testing.T) {
	input := PromptInput{
		Filename:     "report.pdf",
		FileContents: nil,
		VaultDir:     "/Users/x/Vault",
		Template:     "File: {{filename}}, Vault: {{vault_dir}}",
	}

	msgs := BuildPrompt(input)
	content := msgs[1].Content

	if !strings.Contains(content, "report.pdf") {
		t.Fatalf("expected filename substitution, got: %s", content)
	}
	if !strings.Contains(content, "/Users/x/Vault") {
		t.Fatalf("expected vault_dir substitution, got: %s", content)
	}
}

func TestBuildPrompt_WithContentOverLimit(t *testing.T) {
	// Create content > 10KB limit
	largeContent := make([]byte, 20*1024)
	for i := range largeContent {
		largeContent[i] = byte('A' + i%26)
	}

	input := PromptInput{
		Filename:     "large.txt",
		FileContents: largeContent,
		VaultDir:     "/tmp/vault",
		Template:     "Process {{filename}}. Content: {{file_contents}}",
	}

	msgs := BuildPrompt(input)
	if len(msgs[1].Parts) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(msgs[1].Parts))
	}

	filePart := msgs[1].Parts[1]
	if filePart.Type != "input_file" {
		t.Fatalf("expected input_file type, got %s", filePart.Type)
	}

	// Decode the base64 to verify it's truncated
	decoded, err := decodeBase64(filePart.RawData)
	if err != nil {
		t.Fatalf("failed to decode base64: %v", err)
	}
	if len(decoded) > 10240 {
		t.Fatalf("content should be truncated to 10KB, got %d bytes", len(decoded))
	}
}

func decodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

func TestBuildPrompt_JSONSerializable(t *testing.T) {
	// Ensure the returned messages can be serialized to JSON
	input := PromptInput{
		Filename:     "test.txt",
		FileContents: []byte("hello"),
		VaultDir:     "/tmp/vault",
		Template:     "Process {{filename}}",
	}

	msgs := BuildPrompt(input)
	raw := toRawMessages(msgs)
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("failed to marshal messages to JSON: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("empty JSON output")
	}
}
