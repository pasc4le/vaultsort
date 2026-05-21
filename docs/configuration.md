# Configuration Reference

VaultSort configuration uses TOML format and lives at `~/.config/vaultsort/config.toml` by default.

## File Location

| Path | Purpose |
|------|---------|
| `~/.config/vaultsort/config.toml` | Main configuration file |
| `~/.config/vaultsort/` | Config directory (auto-created) |
| `~/.local/state/vaultsort/state.json` | Processed files state |
| `~/.local/state/vaultsort/stdout.log` | LaunchAgent stdout |
| `~/.local/state/vaultsort/stderr.log` | LaunchAgent stderr |

## Environment Variable Overrides

| Variable | Overrides | Example |
|----------|-----------|---------|
| `WATCH_PATH` | Colon-separated list of directories to watch | `~/Downloads:~/Desktop/inbox` |
| `VAULTSORT_VAULT_DIR` | Vault storage directory | `~/Vault` |
| `VAULTSORT_LLM_API_KEY` | LLM API key | `sk-...` |
| `VAULTSORT_LLM_BASE_URL` | LLM base URL | `http://localhost:11434/v1` |
| `VAULTSORT_LLM_PROVIDER` | LLM provider | `ollama` |
| `VAULTSORT_LLM_MODEL` | LLM model name | `gpt-4o-mini` |

## Full Configuration

### `[settings]`

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `poll_interval` | int | `30` | Seconds between directory scans (catches events fsnotify might miss) |
| `max_file_size` | int | `1048576` | Max file size in bytes for LLM content analysis. Larger files send filename only |
| `log_level` | string | `"info"` | One of: `debug`, `info`, `warn`, `error` |
| `log_file` | string | `""` | Log file path. If empty, logs to stderr |

### `[llm]`

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `provider` | string | `"openai"` | One of: `openai`, `anthropic`, `ollama`, `azure`, `custom` |
| `api_key` | string | `""` | API key. Can use `VAULTSORT_LLM_API_KEY` env var instead |
| `model` | string | `"gpt-4o-mini"` | Model name to use |
| `base_url` | string | `""` | Base URL override. Provider defaults apply if empty |
| `timeout` | int | `30` | Request timeout in seconds |
| `max_retries` | int | `3` | Number of retries on transient failures |
| `temperature` | float | `0.1` | LLM temperature (0.0 = deterministic, 1.0 = creative) |

### `[[watch_paths]]`

Each entry specifies a directory to monitor:

```toml
[[watch_paths]]
path = "/Users/you/Downloads"

[[watch_paths]]
path = "/Users/you/Desktop/inbox"
```

### `[[rules]]`

Each rule has three sections:

#### Rule Header

| Key | Type | Description |
|-----|------|-------------|
| `name` | string | Unique rule identifier |
| `description` | string | Human-readable description |
| `enabled` | bool | Set to `false` to disable a rule |

#### `[rules.match]` — Matching Criteria

All criteria must match (AND logic). Empty/unset criteria are ignored.

| Key | Type | Description |
|-----|------|-------------|
| `extension` | string[] | File extensions (case-insensitive, without dot): `["pdf", "doc"]` |
| `startswith` | string | Filename must start with this prefix: `"invoice_"` |
| `endswith` | string | Filename (before extension) must end with this suffix: `"_final"` |
| `created_after` | string | File birthtime after this timestamp (RFC3339): `"2024-01-01T00:00:00Z"` |
| `modified_after` | string | File mtime after this timestamp (RFC3339) |

#### `[rules.action]` — Action Configuration

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `send_file` | bool | `false` | Include file contents in LLM prompt (text files only, limited to 10KB) |
| `send_filename` | bool | `true` | Include original filename in LLM prompt |
| `prompt` | string | `""` | LLM prompt template with `{{filename}}`, `{{file_contents}}`, `{{vault_dir}}` placeholders |
| `end_dir` | string | `""` | Constrain LLM response to start with this subdirectory prefix |
| `start_dir` | string | `""` | Only apply this rule to watch paths whose basename matches |
| `fallback_subdir` | string | `"unsorted"` | Subdirectory to use if LLM call fails |
