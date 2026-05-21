# macOS Service Management

VaultSort can run as a background service (LaunchAgent) on macOS, managed by `launchd`.

## What is a LaunchAgent?

A LaunchAgent is a user-level background service that starts when you log in and runs until you log out. Unlike LaunchDaemons (system-level), LaunchAgents run with your user account and have access to your home directory and GUI.

## Installation

```bash
# Install as LaunchAgent
vaultsort install
```

This does the following:

1. **Generates** `~/Library/LaunchAgents/com.vaultsort.plist`
2. **Creates** `~/.local/state/vaultsort/` for logs
3. **Loads** the service with `launchctl load ~/Library/LaunchAgents/com.vaultsort.plist`

After installation, the service:
- Starts automatically when you log in
- Restarts automatically if it crashes
- Logs stdout to `~/.local/state/vaultsort/stdout.log`
- Logs stderr to `~/.local/state/vaultsort/stderr.log`

## Uninstallation

```bash
vaultsort uninstall
```

This:
1. Unloads the service with `launchctl unload`
2. Removes the plist file

## Manual Service Management

```bash
# Check if running
launchctl list | grep vaultsort

# Start manually
launchctl start com.vaultsort

# Stop manually
launchctl stop com.vaultsort

# Check logs
cat ~/.local/state/vaultsort/stdout.log
cat ~/.local/state/vaultsort/stderr.log

# Follow logs in real-time
tail -f ~/.local/state/vaultsort/stdout.log
```

## The Plist File

The generated plist is at `~/Library/LaunchAgents/com.vaultsort.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.vaultsort</string>
    <key>ProgramArguments</key>
    <array>
        <string>/path/to/vaultsort</string>
        <string>run</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>...</string>
    <key>StandardErrorPath</key>
    <string>...</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin</string>
    </dict>
</dict>
</plist>
```

### Key Plist Settings

| Key | Value | Purpose |
|-----|-------|---------|
| `RunAtLoad` | `true` | Start when user logs in |
| `KeepAlive` | `true` | Restart if crashes |
| `StandardOutPath` | log path | Capture stdout |
| `StandardErrorPath` | log path | Capture stderr |

## Troubleshooting

### Service not starting
```bash
# Check if plist is loaded
launchctl list | grep vaultsort

# Check for errors
cat ~/.local/state/vaultsort/stderr.log

# Try manual start
launchctl start com.vaultsort
```

### "Operation not permitted" errors
On macOS 10.15+ (Catalina), VaultSort needs permissions to access watched directories:
- **Downloads folder**: Give VaultSort "Full Disk Access" in System Settings → Privacy & Security
- **Desktop folder**: Same as above
- **Other folders**: Standard file access

### Service stops immediately
Check logs for config errors:
```bash
# Validate config
vaultsort check

# Run once to see errors
vaultsort --once
```

### Binary not found after update
If you update the binary, unload and reload:
```bash
vaultsort uninstall
vaultsort install
```

### Multiple instances
Only one instance of vaultsort should run. Check with:
```bash
ps aux | grep vaultsort
```
If multiple are running, kill all and reload the service.
