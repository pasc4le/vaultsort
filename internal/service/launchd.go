package service

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"text/template"
)

const (
	label       = "com.vaultsort"
	plistName   = "com.vaultsort.plist"
)

// plistTemplate is the macOS LaunchAgent plist.
var plistTemplate = template.Must(template.New("plist").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.vaultsort</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.BinaryPath}}</string>
        <string>run</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>{{.StateDir}}/stdout.log</string>
    <key>StandardErrorPath</key>
    <string>{{.StateDir}}/stderr.log</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/usr/bin:/bin</string>
    </dict>
</dict>
</plist>
`))

// PlistData holds template data for the plist file.
type PlistData struct {
	BinaryPath string
	StateDir   string
}

// Install installs vaultsort as a LaunchAgent.
func Install() error {
	usr, err := user.Current()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}

	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("get executable path: %w", err)
	}

	stateDir := filepath.Join(usr.HomeDir, ".local", "state", "vaultsort")

	// Ensure state directory exists
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		return fmt.Errorf("create state directory: %w", err)
	}

	// Generate plist content
	var buf bytes.Buffer
	err = plistTemplate.Execute(&buf, PlistData{
		BinaryPath: binaryPath,
		StateDir:   stateDir,
	})
	if err != nil {
		return fmt.Errorf("generate plist: %w", err)
	}

	// Write plist to LaunchAgents directory
	launchAgentsDir := filepath.Join(usr.HomeDir, "Library", "LaunchAgents")
	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return fmt.Errorf("create LaunchAgents directory: %w", err)
	}

	plistPath := filepath.Join(launchAgentsDir, plistName)
	if err := os.WriteFile(plistPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	fmt.Printf("Plist written to %s\n", plistPath)

	// Load with launchctl
	cmd := exec.Command("launchctl", "load", plistPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("launchctl load: %w", err)
	}

	fmt.Println("vaultsort LaunchAgent installed and loaded.")
	return nil
}

// Uninstall removes the vaultsort LaunchAgent.
func Uninstall() error {
	usr, err := user.Current()
	if err != nil {
		return fmt.Errorf("get current user: %w", err)
	}

	launchAgentsDir := filepath.Join(usr.HomeDir, "Library", "LaunchAgents")
	plistPath := filepath.Join(launchAgentsDir, plistName)

	// Unload with launchctl (ignore error if not loaded)
	cmd := exec.Command("launchctl", "unload", plistPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Run()

	// Remove plist file
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove plist: %w", err)
	}

	fmt.Println("vaultsort LaunchAgent removed.")
	return nil
}

// IsInstalled checks if vaultsort is installed as a LaunchAgent.
func IsInstalled() bool {
	usr, err := user.Current()
	if err != nil {
		return false
	}
	plistPath := filepath.Join(usr.HomeDir, "Library", "LaunchAgents", plistName)
	_, err = os.Stat(plistPath)
	return err == nil
}
