# VaultSort

**Intelligent File Organization Daemon** — watches directories and uses AI to automatically organize files into a structured vault.

## Features

- **AI-Powered Organization**: Uses LLMs (OpenAI, Anthropic, Ollama, Azure, custom) to intelligently name and categorize files
- **Rule Engine**: Configurable rules with extension, prefix, suffix, and time-based matchers
- **Real-Time Watching**: Uses fsnotify + polling to catch every file change
- **macOS Native**: Installs as a LaunchAgent for background operation
- **Safe**: Path traversal protection, conflict resolution, dry-run mode
- **Zero Dependencies**: Single static binary, no runtime requirements

## Quick Install

```bash
brew install vaultsort
# OR
curl -sfL https://github.com/pasc4le/vaultsort/releases/latest/download/install.sh | sh
```

## Quick Start

```bash
# Create config
mkdir -p ~/.config/vaultsort
cp config.example.toml ~/.config/vaultsort/config.toml
# Edit ~/.config/vaultsort/config.toml with your API keys

# Validate
vaultsort check

# Test with dry-run
vaultsort --dry-run --once

# Install as background service
vaultsort install
```

## Documentation

Full documentation is available in the [`docs/`](docs/) directory:

| Document | Description |
|----------|-------------|
| [Configuration](docs/configuration.md) | Complete config.toml reference |
| [Rules](docs/rules.md) | Rule engine: matchers, actions, prompts |
| [Providers](docs/providers.md) | LLM provider setup guides |
| [Service](docs/service.md) | macOS LaunchAgent management |
| [Development](docs/development.md) | Architecture & building from source |

## Commands

```
vaultsort [command] [flags]

Commands:
  run           Start the daemon (default)
  install       Install as macOS LaunchAgent
  uninstall     Remove macOS LaunchAgent
  check         Validate config file
  version       Print version

Flags:
  -c, --config  Config file path
  --once        Process files and exit (no watch)
  --dry-run     Log actions without moving files
  -v, --verbose Debug logging
```

## License

EUPL v1.2 — See [LICENSE](LICENSE)
