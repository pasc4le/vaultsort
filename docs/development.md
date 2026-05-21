# Development Guide

## Architecture Overview

```
cmd/vaultsort/main.go     → Entry point, CLI parsing, signal handling
internal/config/          → TOML config loading & validation
internal/rules/           → Rule matching engine
internal/watcher/         → fsnotify + polling file watcher
internal/llm/             → Multi-provider LLM client
internal/organizer/       → Orchestration: rule → LLM → file move
internal/service/         → macOS LaunchAgent installer
```

### Data Flow

```
File System Event
    ↓
watcher.Watcher ──→ Event{Path, WatchRoot}
    ↓
organizer.Organizer.ProcessFile()
    ├─ 1. rules.Engine.FindMatch() → matching Rule
    ├─ 2. Read file contents (if send_file)
    ├─ 3. Build prompt with placeholders
    ├─ 4. llm.Client.Chat() → LLM response
    ├─ 5. Parse JSON response → {filename, subdir}
    ├─ 6. Validate path safety
    ├─ 7. Create destination directory
    ├─ 8. Move file (conflict resolution)
    └─ 9. Update state.json
```

## Prerequisites

- Go 1.22+
- Git
- Make

## Building

```bash
# Build binary
make build

# Install to /usr/local/bin
make install

# Run tests
make test

# Format & vet
make fmt
make vet
```

## Project Structure

```
vaultsort/
├── cmd/vaultsort/main.go        # Entry point
├── internal/
│   ├── config/
│   │   ├── config.go           # Config loading
│   │   └── types.go            # Struct definitions
│   ├── watcher/
│   │   └── watcher.go          # File system watcher
│   ├── rules/
│   │   ├── engine.go           # Rule matching engine
│   │   ├── matcher.go          # Individual matchers
│   │   └── types.go            # Rule structs
│   ├── llm/
│   │   ├── client.go           # Multi-provider client
│   │   ├── prompt.go           # Prompt construction
│   │   └── response.go         # Response parsing
│   ├── organizer/
│   │   └── organizer.go        # File movement logic
│   └── service/
│       └── launchd.go          # Plist generator
├── docs/
│   ├── configuration.md
│   ├── rules.md
│   ├── providers.md
│   ├── service.md
│   └── development.md
├── config.example.toml
├── README.md
├── Makefile
└── LICENSE
```

## Key Design Decisions

### Why `log/slog` instead of `zap`?
The plan mentions both. We use `log/slog` from the standard library (Go 1.21+) to minimize external dependencies. It provides structured logging with levels out of the box.

### Why custom flag parsing instead of cobra?
The CLI is simple enough that standard library `flag` with subcommand handling keeps dependencies minimal. Cobra would add unnecessary complexity.

### Why koanf instead of viper?
koanf is lighter, has first-class TOML support, and doesn't pull in many transitive dependencies like viper does.

### Why own MIME detection instead of net/http?
To avoid importing `net/http` just for `DetectContentType`. Our implementation handles the common cases and is sufficient for deciding whether to send file contents to the LLM.

## Concurrency Model

- **Main goroutine**: CLI parsing, setup, signal handling
- **Watcher goroutine**: fsnotify event loop + polling ticker
- **Event processing**: Sequential in main goroutine (simplicity over parallelism)
- **State access**: Protected by `sync.Mutex` in Organizer

For future optimization, event processing could be parallelized with a worker pool, but the current sequential model is simpler and sufficient for typical usage.

## Testing

```bash
# All tests
go test ./...

# Verbose
go test -v ./...

# Specific package
go test -v ./internal/rules/...

# Coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Test Strategy

1. **Unit tests**: Rule matching, prompt building, path safety, config parsing
2. **Integration tests**: File watching + rule application (temp directories)
3. **LLM tests**: Mock HTTP server for deterministic responses
4. **Manual testing**: `--dry-run` mode for safe validation

## Adding a New LLM Provider

1. Add provider constant in `internal/llm/client.go`
2. Add default base URL in `NewClient()`
3. If the provider uses a non-OpenAI format, add a dedicated method (like `chatAnthropic`)
4. Add provider to validation in `internal/config/config.go`
5. Document in `docs/providers.md`

## Adding a New Matcher

1. Add the field to `MatchCriteria` in `internal/rules/types.go`
2. Add the config field to `MatchConfig` in `internal/config/types.go`
3. Implement the matcher function in `internal/rules/matcher.go`
4. Add the check to `Rule.Matches()` in `internal/rules/matcher.go`
5. Document in `docs/rules.md`

## Release Process

```bash
# Version bump
git tag v0.1.0
git push --tags

# Build for all platforms
GOOS=darwin GOARCH=arm64 go build -o vaultsort-darwin-arm64 ./cmd/vaultsort
GOOS=darwin GOARCH=amd64 go build -o vaultsort-darwin-amd64 ./cmd/vaultsort

# Create GitHub release with binaries
```
