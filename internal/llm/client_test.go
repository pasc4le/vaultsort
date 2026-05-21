package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ============================================================
// B3: supportsStructuredOutput tests
// ============================================================

func TestSupportsStructuredOutput_OpenAI(t *testing.T) {
	c := &Client{provider: ProviderOpenAI}
	if !c.supportsStructuredOutput() {
		t.Fatal("expected OpenAI to support structured output")
	}
}

func TestSupportsStructuredOutput_Azure(t *testing.T) {
	c := &Client{provider: ProviderAzure}
	if !c.supportsStructuredOutput() {
		t.Fatal("expected Azure to support structured output")
	}
}

func TestSupportsStructuredOutput_Custom(t *testing.T) {
	c := &Client{provider: ProviderCustom}
	if c.supportsStructuredOutput() {
		t.Fatal("expected Custom provider to NOT support structured output")
	}
}

func TestSupportsStructuredOutput_Ollama(t *testing.T) {
	c := &Client{provider: ProviderOllama}
	if c.supportsStructuredOutput() {
		t.Fatal("expected Ollama to NOT support structured output")
	}
}

func TestSupportsStructuredOutput_Anthropic(t *testing.T) {
	c := &Client{provider: ProviderAnthropic}
	if c.supportsStructuredOutput() {
		t.Fatal("expected Anthropic (non-OpenAI route) to NOT support structured output")
	}
}

// ============================================================
// B3: response_format conditionality in chatOpenAICompatible
// ============================================================

func TestChatOpenAICompatible_ResponseFormat_Included(t *testing.T) {
	// For OpenAI provider, response_format should be included
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Decode the request to check response_format
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.ResponseFormat == nil {
			t.Fatal("expected response_format to be set for OpenAI provider")
		}
		if req.ResponseFormat.Type != "json_schema" {
			t.Fatalf("expected json_schema, got %s", req.ResponseFormat.Type)
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"filename\":\"test.txt\",\"subdir\":\"docs\"}"}}]}`))
	}))
	defer server.Close()

	c := &Client{
		provider:   ProviderOpenAI,
		baseURL:    server.URL,
		model:      "gpt-4o-mini",
		httpClient: http.DefaultClient,
	}
	msg := []Message{{Role: "user", Content: "test"}}
	_, err := c.chatOpenAICompatible(context.Background(), msg)
	if err != nil {
		t.Fatalf("chatOpenAICompatible error: %v", err)
	}
}

func TestChatOpenAICompatible_ResponseFormat_Omitted(t *testing.T) {
	// For Custom provider, response_format should be omitted
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.ResponseFormat != nil {
			t.Fatal("expected response_format to be nil for Custom provider")
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"filename\":\"test.txt\",\"subdir\":\"docs\"}"}}]}`))
	}))
	defer server.Close()

	c := &Client{
		provider:   ProviderCustom,
		baseURL:    server.URL,
		model:      "custom-model",
		httpClient: http.DefaultClient,
	}
	msg := []Message{{Role: "user", Content: "test"}}
	_, err := c.chatOpenAICompatible(context.Background(), msg)
	if err != nil {
		t.Fatalf("chatOpenAICompatible error: %v", err)
	}
}

func TestChatOpenAICompatible_ResponseFormat_Omitted_Ollama(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.ResponseFormat != nil {
			t.Fatal("expected response_format to be nil for Ollama provider")
		}
		w.Write([]byte(`{"choices":[{"message":{"content":"{\"filename\":\"test.txt\",\"subdir\":\"docs\"}"}}]}`))
	}))
	defer server.Close()

	c := &Client{
		provider:   ProviderOllama,
		baseURL:    server.URL,
		model:      "llama3",
		httpClient: http.DefaultClient,
	}
	msg := []Message{{Role: "user", Content: "test"}}
	_, err := c.chatOpenAICompatible(context.Background(), msg)
	if err != nil {
		t.Fatalf("chatOpenAICompatible error: %v", err)
	}
}

// ============================================================
// B6: toRawMessages — no video_url
// ============================================================

func TestToRawMessages_ImagePart(t *testing.T) {
	msgs := []Message{
		{
			Role: "user",
			Parts: []MessagePart{
				TextPart("analyze this image"),
				FilePart("photo.jpg", []byte{0xFF, 0xD8, 0xFF}, "image/jpeg"),
			},
		},
	}

	raw := toRawMessages(msgs)
	if len(raw) != 1 {
		t.Fatalf("expected 1 raw message, got %d", len(raw))
	}

	content, ok := raw[0].Content.([]map[string]any)
	if !ok {
		t.Fatalf("expected Content to be []map[string]any, got %T", raw[0].Content)
	}
	if len(content) != 2 {
		t.Fatalf("expected 2 parts, got %d", len(content))
	}

	// Second part should be image_url
	imgPart := content[1]
	if imgPart["type"] != "image_url" {
		t.Fatalf("expected image_url type, got %s", imgPart["type"])
	}
	if _, ok := imgPart["image_url"]; !ok {
		t.Fatal("expected image_url key in part")
	}
	// Ensure no video_url key
	if _, ok := imgPart["video_url"]; ok {
		t.Fatal("unexpected video_url key in image part")
	}
}

func TestToRawMessages_NonImageFile(t *testing.T) {
	msgs := []Message{
		{
			Role: "user",
			Parts: []MessagePart{
				TextPart("analyze this file"),
				FilePart("document.pdf", []byte{0x25, 0x50, 0x44, 0x46}, "application/pdf"),
			},
		},
	}

	raw := toRawMessages(msgs)
	content, ok := raw[0].Content.([]map[string]any)
	if !ok {
		t.Fatalf("expected Content to be []map[string]any")
	}

	// Non-image file should be text type with decoded content
	filePart := content[1]
	if filePart["type"] != "text" {
		t.Fatalf("expected text type for non-image file, got %s", filePart["type"])
	}
	if filePart["text"] != "%PDF" {
		t.Fatalf("expected decoded text '%%PDF', got %v", filePart["text"])
	}
}

func TestToRawMessages_NoVideoURL(t *testing.T) {
	msgs := []Message{
		{
			Role: "user",
			Parts: []MessagePart{
				TextPart("test"),
			},
		},
	}

	raw := toRawMessages(msgs)
	rawJSON, _ := json.Marshal(raw)
	if containsStr(string(rawJSON), "video_url") {
		t.Fatal("video_url should not appear in any serialized message")
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && searchStr(s, substr)
}

func searchStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================
// B12: isRetryableHTTP tests
// ============================================================

func TestIsRetryableHTTP_429(t *testing.T) {
	if !isRetryableHTTP(429) {
		t.Fatal("429 should be retryable")
	}
}

func TestIsRetryableHTTP_500(t *testing.T) {
	if !isRetryableHTTP(500) {
		t.Fatal("500 should be retryable")
	}
}

func TestIsRetryableHTTP_502(t *testing.T) {
	if !isRetryableHTTP(502) {
		t.Fatal("502 should be retryable")
	}
}

func TestIsRetryableHTTP_503(t *testing.T) {
	if !isRetryableHTTP(503) {
		t.Fatal("503 should be retryable")
	}
}

func TestIsRetryableHTTP_400(t *testing.T) {
	if isRetryableHTTP(400) {
		t.Fatal("400 should NOT be retryable")
	}
}

func TestIsRetryableHTTP_200(t *testing.T) {
	if isRetryableHTTP(200) {
		t.Fatal("200 should NOT be retryable")
	}
}

func TestIsRetryableHTTP_404(t *testing.T) {
	if isRetryableHTTP(404) {
		t.Fatal("404 should NOT be retryable")
	}
}

// ============================================================
// B12: Retry behavior tests
// ============================================================

func TestChatOpenAICompatible_400_NoRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"bad request"}}`))
	}))
	defer server.Close()

	c := &Client{
		provider:   ProviderOpenAI,
		baseURL:    server.URL,
		model:      "gpt-4o-mini",
		maxRetries: 3,
		httpClient: http.DefaultClient,
	}
	_, err := c.chatOpenAICompatible(context.Background(), []Message{{Role: "user", Content: "test"}})
	if err == nil {
		t.Fatal("expected error for HTTP 400")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt (no retry on 400), got %d", attempts)
	}
}

func TestChatOpenAICompatible_500_Retries(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"server error"}}`))
	}))
	defer server.Close()

	c := &Client{
		provider:   ProviderOpenAI,
		baseURL:    server.URL,
		model:      "gpt-4o-mini",
		maxRetries: 2,
		httpClient: http.DefaultClient,
	}
	_, err := c.chatOpenAICompatible(context.Background(), []Message{{Role: "user", Content: "test"}})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if attempts != 3 { // initial + 2 retries
		t.Fatalf("expected 3 attempts (1 initial + 2 retries), got %d", attempts)
	}
}
