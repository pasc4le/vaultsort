package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/pasc4le/vaultsort/internal/config"
	"github.com/pasc4le/vaultsort/internal/llm"
	"github.com/pasc4le/vaultsort/internal/organizer"
	"github.com/pasc4le/vaultsort/internal/rules"
	"github.com/pasc4le/vaultsort/internal/service"
	"github.com/pasc4le/vaultsort/internal/watcher"
)

var version = "dev"

func main() {
	// Pre-process os.Args to find --config/-c anywhere (flag.Parse stops at first non-flag)
	configPath := findConfigFlag(os.Args[1:])

	// Rebuild args without --config for flag.Parse (it would duplicate)
	cleanArgs := stripConfigFlag(os.Args[1:])

	showVersion := flag.Bool("version", false, "Print version")
	once := flag.Bool("once", false, "Process current files and exit (don't watch)")
	dryRun := flag.Bool("dry-run", false, "Log what would happen, don't move files")
	verbose := flag.Bool("verbose", false, "Enable debug logging")

	// Register config flag (will be parsed if before subcommand)
	cfgPath := flag.String("config", "", "Path to config file (default: ~/.config/vaultsort/config.toml)")
	cfgPathShort := flag.String("c", "", "Path to config file (shorthand)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, `vaultsort - Intelligent File Organization Daemon

Usage:
  vaultsort [flags] <command> [args]

Commands:
  run           Start the daemon (default)
  install       Install as macOS LaunchAgent
  uninstall     Remove macOS LaunchAgent
  check         Validate config file and exit
  version       Print version

Flags:
  -c, --config   Path to config file (default: ~/.config/vaultsort/config.toml)
  --once         Process current files and exit (don't watch)
  --dry-run      Log what would happen, don't move files
  -v, --verbose  Enable debug logging
  --version      Print version
`)
	}

	flag.CommandLine.Parse(cleanArgs)

	if *showVersion {
		fmt.Printf("vaultsort version %s\n", version)
		os.Exit(0)
	}

	// Use manually found config path, or flag value, or default
	resolvedConfig := configPath
	if resolvedConfig == "" {
		resolvedConfig = *cfgPath
	}
	if resolvedConfig == "" {
		resolvedConfig = *cfgPathShort
	}

	// Determine command from remaining args
	args := flag.Args()
	cmd := "run"
	if len(args) > 0 {
		cmd = args[0]
	}

	switch cmd {
	case "version":
		fmt.Printf("vaultsort version %s\n", version)
		os.Exit(0)

	case "install":
		if err := service.Install(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)

	case "uninstall":
		if err := service.Uninstall(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		os.Exit(0)

	case "check":
		if err := runCheck(resolvedConfig); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Configuration is valid.")
		os.Exit(0)

	case "run":
		// fall through to daemon start

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		flag.Usage()
		os.Exit(1)
	}

	// ─── Daemon mode ─────────────────────────────────────────────────────

	if err := config.EnsureDirs(); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating directories: %v\n", err)
		os.Exit(1)
	}

	cfg, err := config.Load(resolvedConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	logLevel := slog.LevelInfo
	if *verbose || cfg.Settings.LogLevel == "debug" {
		logLevel = slog.LevelDebug
	}

	var logFile *os.File
	if cfg.Settings.LogFile != "" {
		var err error
		logFile, err = os.OpenFile(cfg.Settings.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening log file: %v\n", err)
			os.Exit(1)
		}
		defer logFile.Close()
	}

	loggerOpts := &slog.HandlerOptions{Level: logLevel}
	var logger *slog.Logger
	if logFile != nil {
		logger = slog.New(slog.NewTextHandler(logFile, loggerOpts))
	} else {
		logger = slog.New(slog.NewTextHandler(os.Stderr, loggerOpts))
	}

	vaultDir := cfg.Settings.VaultDir
	if vaultDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			logger.Error("cannot determine home directory", "error", err)
			os.Exit(1)
		}
		vaultDir = filepath.Join(home, "Vault")
	}

	ruleEngine, err := rules.NewEngine(cfg.Rules)
	if err != nil {
		logger.Error("rule engine init", "error", err)
		os.Exit(1)
	}

	llmClient, err := llm.NewClient(llm.Config{
		Provider:    cfg.LLM.Provider,
		APIKey:      cfg.LLM.APIKey,
		BaseURL:     cfg.LLM.BaseURL,
		Model:       cfg.LLM.Model,
		Timeout:     cfg.LLM.Timeout,
		MaxRetries:  cfg.LLM.MaxRetries,
		Temperature: cfg.LLM.Temperature,
	})
	if err != nil {
		logger.Error("LLM client init", "error", err)
		os.Exit(1)
	}

	org, err := organizer.New(
		vaultDir,
		ruleEngine,
		llmClient,
		config.StateFile(),
		*dryRun,
		logger,
	)
	if err != nil {
		logger.Error("organizer init", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	if *once {
		for _, wp := range cfg.WatchPaths {
			entries, err := os.ReadDir(wp.Path)
			if err != nil {
				logger.Error("read directory", "path", wp.Path, "error", err)
				continue
			}
			for _, entry := range entries {
				if entry.IsDir() {
					continue
				}
				fullPath := filepath.Join(wp.Path, entry.Name())
				if err := org.ProcessFile(ctx, wp.Path, fullPath); err != nil {
					logger.Error("process file", "path", fullPath, "error", err)
				}
			}
		}
		if err := org.SaveState(); err != nil {
			logger.Error("save state", "error", err)
		}
		return
	}

	w, err := watcher.New(cfg.Settings.PollInterval, logger)
	if err != nil {
		logger.Error("watcher init", "error", err)
		os.Exit(1)
	}
	defer w.Stop()

	for _, wp := range cfg.WatchPaths {
		if err := w.AddDir(wp.Path); err != nil {
			logger.Error("add watch directory", "path", wp.Path, "error", err)
			os.Exit(1)
		}
	}

	events, err := w.Start(ctx)
	if err != nil {
		logger.Error("watcher start", "error", err)
		os.Exit(1)
	}

	logger.Info("vaultsort daemon started",
		"vault_dir", vaultDir,
		"watch_paths", len(cfg.WatchPaths),
		"rules", len(cfg.Rules),
	)

	for {
		select {
		case <-sigCh:
			logger.Info("shutting down gracefully...")
			cancel()
			if err := org.SaveState(); err != nil {
				logger.Error("save state on shutdown", "error", err)
			}
			return

		case evt, ok := <-events:
			if !ok {
				return
			}
			if err := org.ProcessFile(ctx, evt.WatchRoot, evt.Path); err != nil {
				logger.Error("process file", "path", evt.Path, "error", err)
			}
			if err := org.SaveState(); err != nil {
				logger.Error("save state", "error", err)
			}
		}
	}
}

// findConfigFlag scans args for --config <path> or -c <path> anywhere.
func findConfigFlag(args []string) string {
	for i, a := range args {
		if a == "--config" || a == "-c" {
			if i+1 < len(args) {
				return args[i+1]
			}
		}
		if strings.HasPrefix(a, "--config=") {
			return strings.TrimPrefix(a, "--config=")
		}
		if strings.HasPrefix(a, "-c=") {
			return strings.TrimPrefix(a, "-c=")
		}
	}
	return ""
}

// stripConfigFlag removes --config/-c and their value from args.
func stripConfigFlag(args []string) []string {
	var result []string
	skipNext := false
	for i, a := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if a == "--config" || a == "-c" {
			// Check if next exists and is not a flag
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				skipNext = true
			}
			continue
		}
		if strings.HasPrefix(a, "--config=") || strings.HasPrefix(a, "-c=") {
			continue
		}
		result = append(result, a)
	}
	return result
}

func runCheck(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}

	fmt.Printf("Settings:\n")
	fmt.Printf("  Poll interval: %ds\n", cfg.Settings.PollInterval)
	fmt.Printf("  Max file size: %d bytes\n", cfg.Settings.MaxFileSize)
	fmt.Printf("  Log level: %s\n", cfg.Settings.LogLevel)

	if cfg.Settings.VaultDir != "" {
		fmt.Printf("  Vault dir: %s\n", cfg.Settings.VaultDir)
	}
	if cfg.Settings.LogFile != "" {
		fmt.Printf("  Log file: %s\n", cfg.Settings.LogFile)
	}

	fmt.Printf("\nLLM Provider:\n")
	fmt.Printf("  Provider: %s\n", cfg.LLM.Provider)
	fmt.Printf("  Model: %s\n", cfg.LLM.Model)
	if cfg.LLM.BaseURL != "" {
		fmt.Printf("  Base URL: %s\n", cfg.LLM.BaseURL)
	}

	fmt.Printf("\nWatch Paths:\n")
	for _, wp := range cfg.WatchPaths {
		fmt.Printf("  - %s\n", wp.Path)
	}

	fmt.Printf("\nRules (%d total):\n", len(cfg.Rules))
	for i, r := range cfg.Rules {
		status := "enabled"
		if !r.Enabled {
			status = "disabled"
		}
		fmt.Printf("  %d. %s (%s): %s\n", i+1, r.Name, status, r.Description)
	}

	return nil
}
