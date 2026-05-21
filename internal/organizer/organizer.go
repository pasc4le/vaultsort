package organizer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/pasc4le/vaultsort/internal/llm"
	"github.com/pasc4le/vaultsort/internal/rules"
)

const (
	maxPathLen   = 1024
	maxConflict  = 99
	maxFileRead  = 10 * 1024 // 10KB excerpt for LLM
)

// State tracks processed files to avoid re-processing.
type State struct {
	ProcessedFiles map[string]ProcessedEntry `json:"processed_files"`
	LastScan       time.Time                  `json:"last_scan"`
}

// ProcessedEntry records details of a processed file.
type ProcessedEntry struct {
	ProcessedAt time.Time `json:"processed_at"`
	Rule        string    `json:"rule"`
	Destination string    `json:"destination"`
}

// Organizer orchestrates rule matching, LLM analysis, and file movement.
type Organizer struct {
	vaultDir    string
	ruleEngine  *rules.Engine
	llmClient   *llm.Client
	stateFile   string
	dryRun      bool
	maxFileSize int64
	logger      *slog.Logger
	state       *State
	mu          sync.Mutex
}

// New creates a new Organizer.
func New(
	vaultDir string,
	ruleEngine *rules.Engine,
	llmClient *llm.Client,
	stateFile string,
	dryRun bool,
	maxFileSize int64,
	logger *slog.Logger,
) (*Organizer, error) {
	absVault, err := filepath.Abs(vaultDir)
	if err != nil {
		return nil, fmt.Errorf("resolve vault dir: %w", err)
	}

	// Ensure vault directory exists
	if err := os.MkdirAll(absVault, 0755); err != nil {
		return nil, fmt.Errorf("create vault dir: %w", err)
	}

	org := &Organizer{
		vaultDir:    absVault,
		ruleEngine:  ruleEngine,
		llmClient:   llmClient,
		stateFile:   stateFile,
		dryRun:      dryRun,
		maxFileSize: maxFileSize,
		logger:      logger,
		state:       &State{ProcessedFiles: make(map[string]ProcessedEntry)},
	}

	// Load existing state
	if err := org.loadState(); err != nil {
		logger.Warn("could not load state file, starting fresh", "error", err)
	}

	return org, nil
}

// ProcessFile handles a single file: match rule, call LLM, move file.
func (o *Organizer) ProcessFile(ctx context.Context, watchPath string, filePath string) error {
	// Check if already processed (by path) — atomic check-and-mark
	if o.tryMarkProcessing(filePath) {
		o.logger.Debug("file already processed or in-progress, skipping", "path", filePath)
		return nil
	}

	// 1. Find matching rule
	rule, matched, err := o.ruleEngine.FindMatch(filePath, watchPath)
	if err != nil {
		return fmt.Errorf("rule matching: %w", err)
	}
	if !matched {
		o.logger.Debug("no matching rule for file", "path", filePath)
		return nil
	}

	o.logger.Info("file matched rule",
		"path", filePath,
		"rule", rule.Name,
	)

	// 2. Read file contents if needed
	var fileContents []byte
	if rule.Action.SendFile {
		fileContents, err = o.readFileContent(filePath)
		if err != nil {
			o.logger.Warn("could not read file content", "path", filePath, "error", err)
			// Continue without content
		}
	}

	// 3. Build prompt
	messages := llm.BuildPrompt(llm.PromptInput{
		Filename:     filepath.Base(filePath),
		FileContents: fileContents,
		VaultDir:     o.vaultDir,
		Template:     rule.Action.Prompt,
	})

	// 4. Call LLM
	o.logger.Debug("sending to LLM",
		"rule", rule.Name,
		"endpoint", o.llmClient.EndpointURL(),
		"messages", len(messages),
	)
	result, err := o.callLLM(ctx, messages, rule)
	if err != nil {
		// Use fallback
		fallback := rule.Action.FallbackSubdir
		if fallback == "" {
			fallback = "unsorted"
		}
		o.logger.Warn("LLM call failed, using fallback",
			"path", filePath,
			"rule", rule.Name,
			"error", err,
			"fallback_subdir", fallback,
		)
		result = &llm.LLMResponse{
			Filename: filepath.Base(filePath),
			Subdir:   fallback,
		}
	}

	// 5. Apply EndDir constraint if set
	if rule.Action.EndDir != "" {
		result.Subdir = filepath.Join(rule.Action.EndDir, result.Subdir)
	}

	// 6. Build destination path
	destPath := filepath.Join(o.vaultDir, result.Subdir, result.Filename)

	// 7. Validate path safety
	if err := o.validatePath(destPath); err != nil {
		return fmt.Errorf("path safety violation: %w", err)
	}

	// 8. Move file (or log in dry-run mode)
	if o.dryRun {
		o.logger.Info("[DRY RUN] would move file",
			"from", filePath,
			"to", destPath,
			"rule", rule.Name,
		)
		return nil
	}

	movedPath, err := o.moveFile(filePath, destPath)
	if err != nil {
		return fmt.Errorf("move file: %w", err)
	}

	// 9. Log result
	o.logger.Info("file organized",
		"from", filePath,
		"to", movedPath,
		"rule", rule.Name,
	)

	// 10. Update state (note: tryMarkProcessing already marked it at start)
	o.state.ProcessedFiles[filePath] = ProcessedEntry{
		ProcessedAt: time.Now(),
		Rule:        rule.Name,
		Destination: movedPath,
	}

	return nil
}

// SaveState persists the state to disk.
func (o *Organizer) SaveState() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.state.LastScan = time.Now()

	// Clean up entries older than 30 days
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	for path, entry := range o.state.ProcessedFiles {
		if entry.ProcessedAt.Before(cutoff) {
			delete(o.state.ProcessedFiles, path)
		}
	}

	data, err := json.MarshalIndent(o.state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	dir := filepath.Dir(o.stateFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	if err := os.WriteFile(o.stateFile, data, 0644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}

	return nil
}

// loadState reads state from disk.
func (o *Organizer) loadState() error {
	data, err := os.ReadFile(o.stateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // fresh start
		}
		return err
	}

	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	if state.ProcessedFiles == nil {
		state.ProcessedFiles = make(map[string]ProcessedEntry)
	}

	o.mu.Lock()
	o.state = &state
	o.mu.Unlock()
	return nil
}

// tryMarkProcessing atomically checks if a file has been processed and marks it.
// Returns true if the file was already marked (should be skipped).
// Returns false if this call successfully claimed the file for processing.
func (o *Organizer) tryMarkProcessing(path string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if _, ok := o.state.ProcessedFiles[path]; ok {
		return true
	}
	o.state.ProcessedFiles[path] = ProcessedEntry{
		ProcessedAt: time.Now(),
	}
	return false
}

// callLLM sends messages to the LLM and parses the response.
func (o *Organizer) callLLM(ctx context.Context, messages []llm.Message, rule *rules.Rule) (*llm.LLMResponse, error) {
	o.logger.Debug("llm request sent, waiting for response",
		"rule", rule.Name,
	)

	text, err := o.llmClient.Chat(ctx, messages)
	if err != nil {
		o.logger.Debug("llm request failed",
			"rule", rule.Name,
			"error", err,
		)
		return nil, fmt.Errorf("LLM chat: %w", err)
	}

	o.logger.Debug("llm response received, parsing",
		"rule", rule.Name,
		"response_len", len(text),
	)

	resp, err := llm.ParseResponse(text)
	if err != nil {
		o.logger.Debug("llm response parse failed",
			"rule", rule.Name,
			"error", err,
		)
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}

	o.logger.Debug("llm response parsed",
		"rule", rule.Name,
		"filename", resp.Filename,
		"subdir", resp.Subdir,
	)

	return resp, nil
}

// readFileContent reads file contents for sending to the LLM.
// Images are sent in full (truncating makes them invalid).
// Text files are excerpted to maxFileRead bytes.
// Other binary files are skipped.
func (o *Organizer) readFileContent(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	// Detect MIME type first, before deciding how to handle the file
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sniff := make([]byte, 512)
	n, _ := f.Read(sniff)
	mimeType := httpDetectContentType(sniff[:n])

	maxSize := o.maxFileSize
	if maxSize <= 0 {
		maxSize = 1048576 // 1MB fallback
	}

	switch {
	case strings.HasPrefix(mimeType, "image/"), strings.HasPrefix(mimeType, "video/"):
		// Images and videos must be sent in full — truncating makes them invalid.
		// Respect maxFileSize as the upper limit.
		if info.Size() > maxSize {
			o.logger.Warn("media file exceeds max_file_size, skipping content",
				"path", path, "size", info.Size(), "max", maxSize)
			return nil, nil
		}
		if _, err := f.Seek(0, 0); err != nil {
			return nil, fmt.Errorf("seek: %w", err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		return data, nil

	case isTextMIME(mimeType):
		// For text files: send a 10KB excerpt if file is large
		if info.Size() <= int64(maxFileRead) {
			if _, err := f.Seek(0, 0); err != nil {
				return nil, fmt.Errorf("seek: %w", err)
			}
			return os.ReadFile(path)
		}
		if _, err := f.Seek(0, 0); err != nil {
			return nil, fmt.Errorf("seek: %w", err)
		}
		buf := make([]byte, maxFileRead)
		n, err := f.Read(buf)
		if err != nil {
			return nil, err
		}
		return buf[:n], nil

	default:
		// Other binary files (PDFs, archives, etc.) — don't send as content.
		// They can't be meaningfully interpreted as text and would need
		// provider-specific handling that isn't implemented yet.
		return nil, nil
	}
}

// validatePath ensures the destination is safe and under vaultDir.
func (o *Organizer) validatePath(dest string) error {
	// Check length
	if len(dest) > maxPathLen {
		return fmt.Errorf("destination path too long (%d chars)", len(dest))
	}

	// Clean and resolve to absolute
	cleaned := filepath.Clean(dest)
	abs, err := filepath.Abs(cleaned)
	if err != nil {
		return fmt.Errorf("resolve absolute path: %w", err)
	}

	// Ensure destination is under vaultDir (not just a prefix match)
	vaultAbs, err := filepath.Abs(o.vaultDir)
	if err != nil {
		return fmt.Errorf("resolve vault path: %w", err)
	}
	rel, err := filepath.Rel(vaultAbs, abs)
	if err != nil {
		return fmt.Errorf("cannot determine relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("destination %s escapes vault directory %s", abs, vaultAbs)
	}

	return nil
}

// moveFile moves a file to the destination, handling conflicts.
func (o *Organizer) moveFile(src, dest string) (string, error) {
	destDir := filepath.Dir(dest)

	// Create destination directory
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return "", fmt.Errorf("create destination directory: %w", err)
	}

	// Check for conflicts and resolve
	resolvedDest := o.resolveConflict(dest)

	// Move the file
	if err := os.Rename(src, resolvedDest); err != nil {
		// If rename fails (e.g., cross-device), fall back to copy+delete
		if copyErr := copyFile(src, resolvedDest); copyErr != nil {
			return "", fmt.Errorf("copy file: %w", copyErr)
		}
		if err := os.Remove(src); err != nil {
			// File was copied but source couldn't be removed — log but don't fail
			o.logger.Warn("copied file but could not remove source",
				"src", src, "dest", resolvedDest, "error", err)
		}
	}

	return resolvedDest, nil
}

// resolveConflict finds a non-conflicting filename.
func (o *Organizer) resolveConflict(dest string) string {
	if _, err := os.Stat(dest); os.IsNotExist(err) {
		return dest
	}

	dir := filepath.Dir(dest)
	filename := filepath.Base(dest)
	ext := filepath.Ext(filename)
	base := strings.TrimSuffix(filename, ext)

	for i := 1; i <= maxConflict; i++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, i, ext))
		if _, err := os.Stat(candidate); os.IsNotExist(err) {
			return candidate
		}
	}

	// Last resort: timestamp
	ts := time.Now().UnixNano()
	return filepath.Join(dir, fmt.Sprintf("%s_%d%s", base, ts, ext))
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// httpDetectContentType detects the MIME type using stdlib + fallbacks for HEIC/AVIF.
func httpDetectContentType(data []byte) string {
	if len(data) == 0 {
		return "application/octet-stream"
	}
	// Use stdlib detection first (handles JPEG, PNG, GIF, WebP, etc.)
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "application/octet-stream") {
		return mime
	}
	// Custom fallbacks for types stdlib doesn't know
	if isHEIC(data) {
		return "image/heic"
	}
	if isAVIF(data) {
		return "image/avif"
	}
	return mime
}

// isHEIC detects HEIC/HEIF images via "ftyp" box.
func isHEIC(data []byte) bool {
	if len(data) < 12 {
		return false
	}
	return string(data[4:8]) == "ftyp" &&
		(string(data[8:12]) == "heic" || string(data[8:12]) == "heix" || string(data[8:12]) == "mif1")
}

// isAVIF detects AVIF images via "ftyp" box.
func isAVIF(data []byte) bool {
	if len(data) < 12 {
		return false
	}
	return string(data[4:8]) == "ftyp" &&
		(string(data[8:12]) == "avif" || string(data[8:12]) == "avis")
}

// isTextMIME returns true for text-like MIME types.
func isTextMIME(mime string) bool {
	textTypes := []string{
		"text/", "application/json", "application/xml",
		"application/javascript", "application/x-yaml",
		"application/toml", "application/x-sh",
	}
	for _, t := range textTypes {
		if strings.HasPrefix(mime, t) {
			return true
		}
	}
	return false
}
