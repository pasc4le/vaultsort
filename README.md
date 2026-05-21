# vaultsort

AI-powered daemon that watches directories and organizes files into a structured vault.

## Install

```bash
go install github.com/pasc4le/vaultsort/cmd/vaultsort@latest
```

## Usage

```bash
# Validate your config
vaultsort check

# Run once (process files and exit)
vaultsort --once

# Dry-run to preview what would happen
vaultsort --dry-run --once

# Run as background service (macOS LaunchAgent)
vaultsort install

# Start/stop the service
launchctl start com.vaultsort
launchctl stop com.vaultsort
```

Licensed under the **EUPL v1.2**.

See [docs/](docs/) for full configuration, rule engine, providers, and service management.
