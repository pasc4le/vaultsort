package llm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Provider types supported.
type Provider string

const (
	ProviderOpenAI   Provider = "openai"
	ProviderAnthropic Provider = "anthropic"
	ProviderOllama   Provider = "ollama"
	ProviderAzure    Provider = "azure"
	ProviderCustom   Provider = "custom"
)

// MessagePart is a single part of a multi-part message content.
type MessagePart struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	Filename string `json:"filename,omitempty"`
	FileData string `json:"file_data,omitempty"` // OpenAI: "base64:mime;base64data"
	// Anthropic-specific fields (only used when serializing for Anthropic)
	MediaType string          `json:"-"`
	RawData   string          `json:"-"` // raw base64 without prefix
	Source    json.RawMessage `json:"source,omitempty"`
}

// Message represents a chat message in the request.
// Content is used for simple text-only messages.
// Parts is used for multi-part messages (text + files).
type Message struct {
	Role    string         `json:"role"`
	Content string         `json:"content,omitempty"`
	Parts   []MessagePart  `json:"-"` // used internally for file uploads
}

// ChatRequest is the OpenAI-compatible chat completion request body.
type ChatRequest struct {
	Model          string          `json:"model"`
	Messages       []RawMessage    `json:"messages"`
	Temperature    float64         `json:"temperature,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`
}

// RawMessage is used for JSON marshaling of messages with multipart content.
type RawMessage struct {
	Role    string        `json:"role"`
	Content any           `json:"content"` // string or []MessagePart
}

// ResponseFormat configures structured output.
type ResponseFormat struct {
	Type       string      `json:"type"`
	JSONSchema *JSONSchema `json:"json_schema,omitempty"`
}

// JSONSchema defines the schema for structured output.
type JSONSchema struct {
	Name   string         `json:"name"`
	Schema map[string]any `json:"schema"`
	Strict bool           `json:"strict"`
}

// ChatResponse is the OpenAI-compatible chat completion response.
type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
			Parsed  string `json:"parsed"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Client is a multi-provider LLM client.
type Client struct {
	provider    Provider
	apiKey      string
	baseURL     string
	model       string
	timeout     time.Duration
	maxRetries  int
	temperature float64
	httpClient  *http.Client
}

// Config holds LLM client configuration.
type Config struct {
	Provider    string
	APIKey      string
	BaseURL     string
	Model       string
	Timeout     int
	MaxRetries  int
	Temperature float64
}

// NewClient creates a new LLM client from config.
func NewClient(cfg Config) (*Client, error) {
	provider := Provider(cfg.Provider)
	baseURL := cfg.BaseURL

	// Set default base URLs per provider
	if baseURL == "" {
		switch provider {
		case ProviderOpenAI:
			baseURL = "https://api.openai.com/v1"
		case ProviderOllama:
			baseURL = "http://localhost:11434/v1"
		case ProviderAnthropic:
			baseURL = "https://api.anthropic.com/v1"
		default:
			baseURL = "https://api.openai.com/v1"
		}
	}

	return &Client{
		provider:    provider,
		apiKey:      cfg.APIKey,
		baseURL:     baseURL,
		model:       cfg.Model,
		timeout:     time.Duration(cfg.Timeout) * time.Second,
		maxRetries:  cfg.MaxRetries,
		temperature: cfg.Temperature,
		httpClient: &http.Client{
			Timeout: time.Duration(cfg.Timeout) * time.Second,
		},
	}, nil
}

// Chat sends a chat completion request and returns the response content.
func (c *Client) Chat(ctx context.Context, messages []Message) (string, error) {
	if c.provider == ProviderAnthropic {
		return c.chatAnthropic(ctx, messages)
	}
	return c.chatOpenAICompatible(ctx, messages)
}

// toRawMessages converts internal Message slice to serializable RawMessage slice
// for OpenAI-compatible providers.
func toRawMessages(messages []Message) []RawMessage {
	raw := make([]RawMessage, len(messages))
	for i, m := range messages {
		if len(m.Parts) > 0 {
			// Convert parts to OpenAI format
			parts := make([]map[string]any, len(m.Parts))
			for j, p := range m.Parts {
				switch p.Type {
				case "text":
					parts[j] = map[string]any{"type": "text", "text": p.Text}
				case "input_file":
					parts[j] = map[string]any{
						"type":     "input_file",
						"filename": p.Filename,
						"file_data": p.FileData,
					}
				default:
					parts[j] = map[string]any{"type": p.Type}
				}
			}
			raw[i] = RawMessage{Role: m.Role, Content: parts}
		} else {
			raw[i] = RawMessage{Role: m.Role, Content: m.Content}
		}
	}
	return raw
}

// chatOpenAICompatible sends a request to any OpenAI-compatible endpoint.
func (c *Client) chatOpenAICompatible(ctx context.Context, messages []Message) (string, error) {
	url := c.buildURL()

	rawMsgs := toRawMessages(messages)
	reqBody := ChatRequest{
		Model:          c.model,
		Messages:       rawMsgs,
		Temperature:    c.temperature,
		MaxTokens:      1024,
		ResponseFormat: jsonResponseFormat(),
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if c.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+c.apiKey)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("HTTP request: %w", err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read response: %w", err)
			continue
		}

		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, string(respBody))
			continue
		}

		var chatResp ChatResponse
		if err := json.Unmarshal(respBody, &chatResp); err != nil {
			lastErr = fmt.Errorf("parse response: %w", err)
			continue
		}

		if chatResp.Error != nil {
			lastErr = fmt.Errorf("API error: %s", chatResp.Error.Message)
			continue
		}

		if len(chatResp.Choices) == 0 {
			lastErr = fmt.Errorf("empty response: no choices")
			continue
		}

		content := chatResp.Choices[0].Message.Content
		if content == "" {
			content = chatResp.Choices[0].Message.Parsed
		}
		return content, nil
	}

	return "", fmt.Errorf("after %d retries: %w", c.maxRetries, lastErr)
}

// chatAnthropic handles the Anthropic Messages API format.
func (c *Client) chatAnthropic(ctx context.Context, messages []Message) (string, error) {
	url := c.baseURL + "/messages"

	// Convert to Anthropic format with multipart content support
	var anthropicMessages []map[string]any
	for _, m := range messages {
		msg := map[string]any{"role": m.Role}

		if len(m.Parts) > 0 {
			// Build multipart content array
			var parts []map[string]any
			for _, p := range m.Parts {
				switch p.Type {
				case "text":
					parts = append(parts, map[string]any{"type": "text", "text": p.Text})
				case "document", "input_file":
					// Anthropic document format
					parts = append(parts, map[string]any{
						"type": "document",
						"source": map[string]any{
							"type":       "base64",
							"media_type": p.MediaType,
							"data":       p.RawData,
						},
					})
				default:
					parts = append(parts, map[string]any{"type": p.Type})
				}
			}
			msg["content"] = parts
		} else {
			msg["content"] = []map[string]any{{"type": "text", "text": m.Content}}
		}

		anthropicMessages = append(anthropicMessages, msg)
	}

	reqBody := map[string]interface{}{
		"model":         c.model,
		"messages":      anthropicMessages,
		"temperature":   c.temperature,
		"max_tokens":    1024,
		"tool_choice":   map[string]string{"type": "tool", "name": "file_organization"},
		"tools":         anthropicTools(),
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal anthropic request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}

		req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("create anthropic request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("x-api-key", c.apiKey)
		req.Header.Set("anthropic-version", "2023-06-01")

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("anthropic HTTP request: %w", err)
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read anthropic response: %w", err)
			continue
		}

		if resp.StatusCode != 200 {
			lastErr = fmt.Errorf("anthropic API error (HTTP %d): %s", resp.StatusCode, string(respBody))
			continue
		}

		// Parse Anthropic response: extract from tool_use content block
		var anonResp struct {
			Content []struct {
				Type  string          `json:"type"`
				Text  string          `json:"text"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			} `json:"content"`
		}
		if err := json.Unmarshal(respBody, &anonResp); err != nil {
			lastErr = fmt.Errorf("parse anthropic response: %w", err)
			continue
		}

		if len(anonResp.Content) == 0 {
			lastErr = fmt.Errorf("anthropic empty response")
			continue
		}

		// Try to extract from tool_use block first
		for _, c := range anonResp.Content {
			if c.Type == "tool_use" && c.Name == "file_organization" {
				var input struct {
					Filename string `json:"filename"`
					Subdir   string `json:"subdir"`
				}
				if err := json.Unmarshal(c.Input, &input); err == nil {
					out, _ := json.Marshal(input)
					return string(out), nil
				}
			}
		}

		// Fallback: return concatenated text content
		var text string
		for _, c := range anonResp.Content {
			if c.Type == "text" {
				text += c.Text
			}
		}
		if text == "" {
			lastErr = fmt.Errorf("anthropic response has no content")
			continue
		}
		return text, nil
	}

	return "", fmt.Errorf("after %d retries: %w", c.maxRetries, lastErr)
}

// jsonResponseFormat returns the OpenAI structured output response_format.
func jsonResponseFormat() *ResponseFormat {
	return &ResponseFormat{
		Type: "json_schema",
		JSONSchema: &JSONSchema{
			Name: "file_organization",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"filename": map[string]any{"type": "string"},
					"subdir":   map[string]any{"type": "string"},
				},
				"required":             []string{"filename", "subdir"},
				"additionalProperties": false,
			},
			Strict: true,
		},
	}
}

// anthropicTools returns Anthropic tool definitions for structured output.
func anthropicTools() []map[string]any {
	return []map[string]any{
		{
			"name":        "file_organization",
			"description": "Return the organized filename and subdirectory for a file.",
			"input_schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"filename": map[string]any{"type": "string", "description": "Clean filename with extension"},
					"subdir":   map[string]any{"type": "string", "description": "Relative subdirectory path"},
				},
				"required": []string{"filename", "subdir"},
			},
		},
	}
}

// EndpointURL returns the full API endpoint URL this client will call.
func (c *Client) EndpointURL() string {
	return c.buildURL()
}

// buildURL constructs the API endpoint URL based on provider.
func (c *Client) buildURL() string {
	switch c.provider {
	case ProviderAzure:
		return fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=2024-02-15-preview",
			c.baseURL, c.model)
	default:
		return c.baseURL + "/chat/completions"
	}
}

// FilePart creates a MessagePart for a file to be sent as part of the message.
// data is the raw file bytes, mimeType is the MIME type (e.g. "application/pdf").
func FilePart(filename string, data []byte, mimeType string) MessagePart {
	encoded := base64.StdEncoding.EncodeToString(data)
	return MessagePart{
		Type:      "input_file",
		Filename:  filename,
		FileData:  fmt.Sprintf("base64:%s;%s", mimeType, encoded),
		MediaType: mimeType,
		RawData:   encoded,
	}
}

// TextPart creates a MessagePart for plain text content.
func TextPart(text string) MessagePart {
	return MessagePart{Type: "text", Text: text}
}

// WithParts creates a Message with multipart content.
func WithParts(role string, parts []MessagePart) Message {
	return Message{Role: role, Parts: parts}
}
