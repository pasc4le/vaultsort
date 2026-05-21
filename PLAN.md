# VaultSort — Intelligent File Organization Daemon

## Overview

**VaultSort** is a macOS-compatible background daemon that watches specified directories and uses LLM intelligence to automatically organize files into a structured vault. It applies configurable rules to filter files, then prompts an LLM to determine optimal file naming and storage location.

---

## Language Choice: **Go**

After researching library support across Go, Rust, and Bun:

| Criteria | Go | Rust | Bun/TS |
|----------|-----|------|--------|
| File Watcher | `fsnotify` (mature, cross-platform) | `notify` crate | `chokidar` v4 |
| Config (TOML) | `koanf` or `viper` | `toml` + `serde` | Built-in / `@iarna/toml` |
| LLM Clients | `goai`, `any-llm-go`, `llmhub` | `async-openai`, `litellm-rust` | Official OpenAI SDK |
| Distribution | Single static binary ✓ | Single binary (larger) | Requires Bun runtime |
| macOS Service | Native plist + launchctl | Native plist + launchctl | Needs wrapper script |
| Build Complexity | Simple | Moderate (compile times) | Simple |

**Decision: Go** — Single binary distribution, zero runtime dependencies, excellent ecosystem for all required features, simplest macOS service integration.

---

## Project Name: `vaultsort`

Config directory: `~/.config/vaultsort/`
Binary name: `vaultsort`

---

## Architecture

```
vaultsort/
├── cmd/
│   └── vaultsort/
│       └── main.go              # Entry point, CLI flags, signal handling
├── internal/
│   ├── config/
│   │   ├── config.go            # TOML config loading & validation
│   │   └── types.go             # Config struct definitions
│   ├── watcher/
│   │   └── watcher.go           # fsnotify wrapper, directory polling
│   ├── rules/
│   │   ├── engine.go            # Rule matching engine
│   │   ├── matcher.go           # Individual matchers (extension, prefix, etc.)
│   │   └── types.go             # Rule struct definitions
│   ├── llm/
│   │   ├── client.go            # Multi-provider LLM client abstraction
│   │   ├── prompt.go            # Prompt construction
│   │   └── response.go          # Response parsing (path extraction)
│   ├── organizer/
│   │   └── organizer.go         # File movement logic, path safety checks
│   └── service/
│       └── launchd.go           # macOS launchd plist generation
├── config.example.toml          # Example configuration
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

---

## Configuration: `~/.config/vaultsort/config.toml`

```toml
# ─── Global Settings ───────────────────────────────────────────────────────────

[settings]
# How often to scan watched directories (seconds)
poll_interval = 30

# Maximum file size to send to LLM (bytes). Files larger than this
# will only have their filename sent.
max_file_size = 1048576  # 1MB

# Log level: "debug", "info", "warn", "error"
log_level = "info"

# Log file path (optional, defaults to stderr)
# log_file = "/Users/you/.local/state/vaultsort/log"

# ─── LLM Provider ──────────────────────────────────────────────────────────────

[llm]
# Provider type: "openai", "anthropic", "ollama", "azure", "custom"
# All providers use OpenAI-compatible chat completion format.
# "custom" lets you specify any base_url.
provider = "openai"

# API key (can also use VAULTSORT_LLM_API_KEY env var)
api_key = "sk-..."

# Model to use
model = "gpt-4o-mini"

# Base URL override (required for "ollama", "azure", "custom")
# OpenAI default:    https://api.openai.com/v1
# Ollama default:    http://localhost:11434/v1
# Anthropic compat:  https://api.anthropic.com/v1  (requires anthropic-compatible proxy)
# base_url = "http://localhost:11434/v1"

# Optional: organization ID (OpenAI)
# organization = ""

# Request timeout in seconds
timeout = 30

# Max retries on transient failures
max_retries = 3

# Temperature for LLM responses (0.0 = deterministic)
temperature = 0.1

# ─── Watch Paths ────────────────────────────────────────────────────────────────

# Each watch path is a directory to monitor.
# Can also be set via WATCH_PATH env var (colon-separated).
[[watch_paths]]
path = "/Users/you/Downloads"

[[watch_paths]]
path = "/Users/you/Desktop/inbox"

# ─── Rules ──────────────────────────────────────────────────────────────────────

# Rules are evaluated in order. First matching rule wins.
# If no rule matches, the file is left untouched.

# Example: Organize PDFs
[[rules]]
name = "pdfs"
description = "Organize PDF documents"
enabled = true

# File filters (all must match for rule to apply)
[rules.match]
extension = ["pdf"]
# startswith = "invoice_"
# endswith = "_final"
# created_after = "2024-01-01T00:00:00Z"
# modified_after = "2024-06-01T00:00:00Z"

# LLM instruction
[rules.action]
# Whether to include file contents in LLM prompt
send_file = true

# Whether to include original filename in LLM prompt
send_filename = true

# The LLM prompt template. Available placeholders:
#   {{filename}} - original filename
#   {{file_contents}} - file contents (if send_file = true and file is text)
#   {{vault_dir}} - the vault directory path
prompt = """
You are a file organization assistant. Analyze this file and determine:
1. A clean, descriptive filename (with extension)
2. A subdirectory path under {{vault_dir}} where it should be stored.

File: {{filename}}
Contents (excerpt):
{{file_contents}}

Respond in JSON format:
{"filename": "descriptive_name.pdf", "subdir": "documents/invoices/2024"}
"""

# Constrain LLM response to start with this subdirectory
# end_dir = "documents"

# Only apply this rule to watch_paths whose basename matches
# start_dir = "Downloads"

# If LLM fails, use this fallback subdirectory
fallback_subdir = "unsorted"

# Example: Organize images
[[rules]]
name = "images"
description = "Organize image files"
enabled = true

[rules.match]
extension = ["jpg", "jpeg", "png", "gif", "webp", "heic"]

[rules.action]
send_file = false
send_filename = true
prompt = """
You are a file organization assistant. Based on the filename, determine:
1. A clean, descriptive filename (with extension)
2. A subdirectory path under {{vault_dir}} for categorization.

Filename: {{filename}}

Respond in JSON format:
{"filename": "clean_name.jpg", "subdir": "photos/2024/misc"}
"""

# Example: Only for a specific watch directory
[[rules]]
name = "screenshots"
description = "Organize screenshots from Desktop"
enabled = true

[rules.match]
startswith = "Screenshot"
extension = ["png", "jpg"]

[rules.action]
send_file = false
send_filename = true
start_dir = "Desktop"
prompt = """
Organize this screenshot. Filename: {{filename}}
Respond in JSON: {"filename": "name.png", "subdir": "screenshots/2024"}
"""

# Example: Temp files — move old ones
[[rules]]
name = "old_temp_files"
description = "Archive old temporary files"
enabled = true

[rules.match]
extension = ["tmp", "temp", "bak"]
modified_after = "2024-01-01T00:00:00Z"

[rules.action]
send_file = false
send_filename = true
prompt = """
File: {{filename}}
Respond in JSON: {"filename": "{{filename}}", "subdir": "archive/temp"}
"""
```

---

## Core Components

### 1. Config Loader (`internal/config/`)

- Uses `github.com/knadh/koanf/v2` with TOML parser
- Environment variable overrides:
  - `WATCH_PATH` — colon-separated list of directories (appended to config)
  - `VAULTSORT_VAULT_DIR` — override vault directory
  - `VAULTSORT_LLM_API_KEY` — override LLM API key
  - `VAULTSORT_LLM_BASE_URL` — override LLM base URL
  - `VAULTSORT_LLM_PROVIDER` — override provider
  - `VAULTSORT_LLM_MODEL` — override model
- Validates all paths exist on startup
- Creates config directory if missing

### 2. File Watcher (`internal/watcher/`)

- Uses `github.com/fsnotify/fsnotify` for real-time notifications
- Additionally polls every `poll_interval` seconds (catches edge cases)
- Debounces rapid events (500ms default)
- Tracks processed files to avoid re-processing (in-memory map + optional state file)
- Ignores: dotfiles, `.DS_Store`, temp files (`.crdownload`, `.part`, `.download`)

### 3. Rule Engine (`internal/rules/`)

**Rule struct:**
```go
type Rule struct {
    Name        string
    Description string
    Enabled     bool
    Match       MatchCriteria
    Action      ActionConfig
}

type MatchCriteria struct {
    Extension      []string  // e.g., ["pdf", "doc"]
    StartsWith     string    // filename prefix
    EndsWith       string    // filename suffix
    CreatedAfter   *time.Time
    ModifiedAfter  *time.Time
}

type ActionConfig struct {
    SendFile       bool
    SendFilename   bool
    Prompt         string
    EndDir         string    // constrain LLM response path prefix
    StartDir       string    // only match files from watch_paths with this basename
    FallbackSubdir string    // on LLM failure
}
```

**Matching logic:**
1. Filter by `StartDir` (if set, only process files from matching watch_path)
2. Check `Extension` (case-insensitive, without dot)
3. Check `StartsWith` (case-sensitive)
4. Check `EndsWith` (case-sensitive, before extension)
5. Check `CreatedAfter` (file birthtime on macOS)
6. Check `ModifiedAfter` (file mtime)
7. All criteria must match (AND logic)
8. First matching rule wins

### 4. LLM Client (`internal/llm/`)

**Provider abstraction:**
```go
type Provider string

const (
    ProviderOpenAI   Provider = "openai"
    ProviderAnthropic Provider = "anthropic"
    ProviderOllama   Provider = "ollama"
    ProviderAzure    Provider = "azure"
    ProviderCustom   Provider = "custom"
)

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
```

**Compatibility approach:**
- All providers exposed via OpenAI-compatible `/v1/chat/completions` endpoint
- For Anthropic: use their Messages API directly (different format) with conversion layer
- For Ollama: native OpenAI-compatible mode at `/v1/chat/completions`
- For Azure: different URL pattern (`/openai/deployments/{model}/chat/completions?api-version=...`)

**Prompt construction:**
```go
type PromptInput struct {
    Filename     string
    FileContents []byte  // may be nil
    VaultDir     string
    Template     string
}

func (c *Client) BuildPrompt(input PromptInput) []Message {
    // Replace {{filename}}, {{file_contents}}, {{vault_dir}}
    // Return system + user messages
}
```

**Response parsing:**
```go
type LLMResponse struct {
    Filename string `json:"filename"`
    Subdir   string `json:"subdir"`
}

// Extract JSON from LLM response (handle markdown code blocks, etc.)
// Validate: no path traversal, subdir is relative, filename is safe
```

**File content handling:**
- Text files: send first N bytes (configurable, default 10KB excerpt)
- Binary files (images, etc.): send only filename if `send_file=true` but file is binary
- Large files: send only filename if exceeds `max_file_size`
- MIME detection via `net/http.DetectContentType`

### 5. Organizer (`internal/organizer/`)

```go
type Organizer struct {
    vaultDir    string
    ruleEngine  *rules.Engine
    llmClient   *llm.Client
    stateFile   string
    mu          sync.Mutex
}

func (o *Organizer) ProcessFile(watchPath string, filePath string) error {
    // 1. Find matching rule
    // 2. Read file (if send_file)
    // 3. Build prompt
    // 4. Call LLM
    // 5. Parse response
    // 6. Validate path safety
    // 7. Create destination directory
    // 8. Move file (with conflict resolution: append timestamp)
    // 9. Log result
    // 10. Update state
}
```

**Path safety:**
- Resolve all symlinks
- Ensure final path is under `VAULT_DIR`
- Reject any `..` components
- Sanitize filename (remove `/`, `\`, null bytes, control chars)
- Max path length: 1024 chars

**Conflict resolution:**
- If destination exists: append `_1`, `_2`, etc. before extension
- Max 99 attempts

### 6. macOS Service (`internal/service/`)

**Install command:**
```
vaultsort install
```

Generates and installs `~/Library/LaunchAgents/com.vaultsort.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.vaultsort</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/vaultsort</string>
        <string>run</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/Users/USER/.local/state/vaultsort/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>/Users/USER/.local/state/vaultsort/stderr.log</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin</string>
    </dict>
</dict>
</plist>
```

**Uninstall command:**
```
vaultsort uninstall
```

---

## CLI Interface

```
vaultsort [command] [flags]

Commands:
  run           Start the daemon (default if no command)
  install       Install as macOS LaunchAgent
  uninstall     Remove macOS LaunchAgent
  check         Validate config file and exit
  version       Print version

Flags:
  --config, -c    Path to config file (default: ~/.config/vaultsort/config.toml)
  --once          Process current files and exit (don't watch)
  --dry-run       Log what would happen, don't move files
  --verbose, -v   Enable debug logging
```

---

## Dependencies

```go
// go.mod
module github.com/user/vaultsort

go 1.22

require (
    github.com/fsnotify/fsnotify v1.7.0      // File watching
    github.com/knadh/koanf/v2 v2.1.0         // Config loading
    github.com/knadh/koanf/parsers/toml v0.1.0 // TOML parser
    github.com/pelletier/go-toml/v2 v2.2.0   // TOML (alternative)
    go.uber.org/zap v1.27.0                   // Structured logging
    github.com/google/uuid v1.6.0             // Temp file naming
)
```

---

## Build & Install

### Makefile

```makefile
BINARY_NAME=vaultsort
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

.PHONY: build install uninstall clean

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) ./cmd/vaultsort

install: build
	cp bin/$(BINARY_NAME) /usr/local/bin/$(BINARY_NAME)
	@echo "Binary installed to /usr/local/bin/$(BINARY_NAME)"
	@echo "Run 'vaultsort install' to set up as a LaunchAgent"

uninstall:
	vaultsort uninstall 2>/dev/null || true
	rm -f /usr/local/bin/$(BINARY_NAME)

clean:
	rm -rf bin/
```

### Installation Steps

```bash
# Clone and build
git clone https://github.com/user/vaultsort.git
cd vaultsort
make build

# Install binary
sudo cp bin/vaultsort /usr/local/bin/

# Create config
mkdir -p ~/.config/vaultsort
cp config.example.toml ~/.config/vaultsort/config.toml
# Edit config.toml with your settings

# Validate config
vaultsort check

# Install as LaunchAgent
vaultsort install

# Manage service
launchctl start com.vaultsort
launchctl stop com.vaultsort
launchctl list | grep vaultsort
```

---

## State Management

State file: `~/.local/state/vaultsort/state.json`

```json
{
  "processed_files": {
    "/path/to/file.pdf": {
      "processed_at": "2024-01-15T10:30:00Z",
      "rule": "pdfs",
      "destination": "/vault/documents/invoices/report.pdf"
    }
  },
  "last_scan": "2024-01-15T10:30:00Z"
}
```

- Tracks processed files by path + mtime
- Prevents re-processing on daemon restart
- Periodic cleanup of entries older than 30 days

---

## Error Handling & Resilience

1. **LLM failures**: Use `fallback_subdir` from matching rule
2. **Network errors**: Retry with exponential backoff (configurable)
3. **Permission errors**: Log warning, skip file
4. **Disk full**: Log error, pause processing, retry in 60s
5. **Config changes**: Reload on SIGHUP
6. **Graceful shutdown**: On SIGINT/SIGTERM, finish current operations, save state

---

## Testing Strategy

1. **Unit tests**: Rule matching, prompt building, path safety, config parsing
2. **Integration tests**: File watching + rule application (temp directories)
3. **LLM tests**: Mock HTTP server for deterministic responses
4. **Manual testing**: `--dry-run` mode for safe validation

---

## Implementation Phases

### Phase 1: Core (Week 1)
- [ ] Project scaffolding, go.mod, Makefile
- [ ] Config loading with TOML parsing
- [ ] Rule engine with all matchers
- [ ] File watcher with fsnotify + polling

### Phase 2: LLM Integration (Week 2)
- [ ] Multi-provider LLM client (OpenAI-compatible base)
- [ ] Prompt construction with placeholders
- [ ] Response parsing and validation
- [ ] Path safety checks

### Phase 3: Organization & Service (Week 3)
- [ ] File movement logic with conflict resolution
- [ ] State tracking
- [ ] LaunchAgent installer/uninstaller
- [ ] CLI interface (cobra or standard flag)

### Phase 4: Polish (Week 4)
- [ ] Error handling and retries
- [ ] SIGHUP config reload
- [ ] Logging improvements
- [ ] Documentation and README
- [ ] Release builds for arm64/amd64

---

## Future Enhancements

- [ ] Watch for file content changes (not just new files)
- [ ] Custom prompt templates per file type
- [ ] Dry-run diff preview before moving
- [ ] Web UI for rule management
- [ ] File hash deduplication
- [ ] Undo last N operations
- [ ] Notification on file move (macOS notifications)
- [ ] Multiple vault directories
- [ ] Rules import/export
