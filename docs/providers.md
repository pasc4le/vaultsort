# LLM Provider Setup

VaultSort supports multiple LLM providers for file organization decisions.

## Supported Providers

| Provider | Config Value | Protocol | Default Base URL |
|----------|-------------|----------|------------------|
| OpenAI | `openai` | OpenAI Chat Completions | `https://api.openai.com/v1` |
| Anthropic | `anthropic` | Anthropic Messages API | `https://api.anthropic.com/v1` |
| Ollama | `ollama` | OpenAI-compatible | `http://localhost:11434/v1` |
| Azure OpenAI | `azure` | Azure OpenAI | (see below) |
| Custom | `custom` | OpenAI-compatible | (user-specified) |

## OpenAI

```toml
[llm]
provider = "openai"
api_key = "sk-your-api-key"
model = "gpt-4o-mini"
```

Recommended models: `gpt-4o-mini` (fast & cheap), `gpt-4o` (more capable)

Get an API key at [platform.openai.com/api-keys](https://platform.openai.com/api-keys).

## Anthropic

```toml
[llm]
provider = "anthropic"
api_key = "sk-ant-your-api-key"
model = "claude-3-haiku-20240307"
```

Recommended models: `claude-3-haiku-20240307` (fast), `claude-3-5-sonnet-20241022` (powerful)

VaultSort uses the Anthropic Messages API natively (different format from OpenAI).

Get an API key at [console.anthropic.com](https://console.anthropic.com/).

## Ollama (Local)

```toml
[llm]
provider = "ollama"
model = "llama3.2"
# base_url defaults to http://localhost:11434/v1
```

### Setup

1. Install Ollama: `brew install ollama`
2. Start: `ollama serve`
3. Pull a model: `ollama pull llama3.2`
4. VaultSort connects to Ollama's OpenAI-compatible endpoint at `http://localhost:11434/v1`

No API key needed for local Ollama.

## Azure OpenAI

```toml
[llm]
provider = "azure"
api_key = "your-azure-api-key"
model = "your-deployment-name"
base_url = "https://your-resource.openai.azure.com"
```

Azure uses deployment names instead of model names. The `base_url` is your resource endpoint.

The API version defaults to `2024-02-15-preview`.

## Custom (OpenAI-compatible)

```toml
[llm]
provider = "custom"
api_key = "optional-key"
model = "your-model"
base_url = "https://your-api-endpoint/v1"
```

Any provider that implements the OpenAI chat completions format can be used. Examples:

- **Groq**: `base_url = "https://api.groq.com/openai/v1"`
- **Together AI**: `base_url = "https://api.together.xyz/v1"`
- **OpenRouter**: `base_url = "https://openrouter.ai/api/v1"`
- **LocalAI**: `base_url = "http://localhost:8080/v1"`
- **vLLM**: `base_url = "http://localhost:8000/v1"`

## API Key Security

- API keys can be set in config file OR via `VAULTSORT_LLM_API_KEY` environment variable
- Environment variable takes precedence over config file
- Avoid committing API keys to version control

## Troubleshooting

### Connection Refused
```
Error: LLM chat: HTTP request: dial tcp: connect: connection refused
```
→ Check that your provider endpoint is running and accessible

### Authentication Error
```
Error: LLM API error (HTTP 401): ...
```
→ Check your API key is correct and has access to the specified model

### Model Not Found
```
Error: LLM API error: model not found
```
→ Check the model name is correct and available in your provider

### Timeout
```
Error: LLM chat: HTTP request: context deadline exceeded
```
→ Increase the `timeout` value in `[llm]` config

### Rate Limited
VaultSort retries automatically up to `max_retries` times with exponential backoff.
