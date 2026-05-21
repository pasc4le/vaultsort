package llm

import (
	"bytes"
	"context"
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

// Message represents a chat message in the request.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the OpenAI-compatible chat completion request body.
type ChatRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature float64   `json:"temperature,omitempty"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

// ChatResponse is the OpenAI-compatible chat completion response.
type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
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

// chatOpenAICompatible sends a request to any OpenAI-compatible endpoint.
func (c *Client) chatOpenAICompatible(ctx context.Context, messages []Message) (string, error) {
	url := c.buildURL()

	reqBody := ChatRequest{
		Model:       c.model,
		Messages:    messages,
		Temperature: c.temperature,
		MaxTokens:   1024,
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

		return chatResp.Choices[0].Message.Content, nil
	}

	return "", fmt.Errorf("after %d retries: %w", c.maxRetries, lastErr)
}

// chatAnthropic handles the Anthropic Messages API format.
func (c *Client) chatAnthropic(ctx context.Context, messages []Message) (string, error) {
	url := c.baseURL + "/messages"

	// Convert to Anthropic format
	type anthropicContent struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type anthropicMessage struct {
		Role    string             `json:"role"`
		Content []anthropicContent `json:"content"`
	}

	var anthropicMessages []anthropicMessage
	for _, m := range messages {
		anthropicMessages = append(anthropicMessages, anthropicMessage{
			Role:    m.Role,
			Content: []anthropicContent{{Type: "text", Text: m.Content}},
		})
	}

	reqBody := map[string]interface{}{
		"model":       c.model,
		"messages":    anthropicMessages,
		"temperature": c.temperature,
		"max_tokens":  1024,
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

		// Parse Anthropic response format
		var anonResp struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
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

		// Return concatenated text content
		var text string
		for _, c := range anonResp.Content {
			if c.Type == "text" {
				text += c.Text
			}
		}
		if text == "" {
			lastErr = fmt.Errorf("anthropic response has no text content")
			continue
		}
		return text, nil
	}

	return "", fmt.Errorf("after %d retries: %w", c.maxRetries, lastErr)
}

// EndpointURL returns the full API endpoint URL this client will call.
func (c *Client) EndpointURL() string {
	return c.buildURL()
}

// buildURL constructs the API endpoint URL based on provider.
func (c *Client) buildURL() string {
	switch c.provider {
	case ProviderAzure:
		// Azure: /openai/deployments/{model}/chat/completions?api-version=2024-02-15-preview
		return fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=2024-02-15-preview",
			c.baseURL, c.model)
	default:
		return c.baseURL + "/chat/completions"
	}
}
