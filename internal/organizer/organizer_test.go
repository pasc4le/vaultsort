package organizer

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// newTestOrganizer creates an Organizer with a temp vault dir for testing.
func newTestOrganizer(t *testing.T) (*Organizer, string) {
	t.Helper()

	vaultDir := t.TempDir()
	logger := NewTestLogger(t)

	org := &Organizer{
		vaultDir:    vaultDir,
		state:       &State{ProcessedFiles: make(map[string]ProcessedEntry)},
		logger:      logger,
		maxFileSize: 1 * 1024 * 1024, // 1MB
	}
	return org, vaultDir
}

// NewTestLogger returns a discard logger for testing.
func NewTestLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// ============================================================
// B1: readFileContent image handling tests
// ============================================================

func TestReadFileContent_PNG_FullContent(t *testing.T) {
	org, _ := newTestOrganizer(t)

	// Create a PNG file (≥10KB) with valid PNG magic bytes
	f, err := os.CreateTemp(t.TempDir(), "test*.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	// Write PNG magic + enough data to exceed 10KB
	pngHeader := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}
	payload := make([]byte, 15*1024) // 15KB
	copy(payload, pngHeader)
	content := append(pngHeader, payload...)
	if err := os.WriteFile(f.Name(), content, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := org.readFileContent(f.Name())
	if err != nil {
		t.Fatalf("readFileContent returned error: %v", err)
	}
	if result == nil {
		t.Fatal("readFileContent returned nil for valid PNG")
	}
	if len(result) < 15*1024 {
		t.Fatalf("PNG content truncated: got %d bytes, expected >= %d", len(result), 15*1024)
	}
}

func TestReadFileContent_PNG_TooLarge(t *testing.T) {
	org, _ := newTestOrganizer(t)
	org.maxFileSize = 1024 // 1KB max

	f, err := os.CreateTemp(t.TempDir(), "test*.png")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Write a file larger than maxFileSize
	content := make([]byte, 2048)
	copy(content, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'})
	if err := os.WriteFile(f.Name(), content, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := org.readFileContent(f.Name())
	if err != nil {
		t.Fatalf("readFileContent returned error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil for oversized PNG, got %d bytes", len(result))
	}
}

func TestReadFileContent_Text_Large(t *testing.T) {
	org, _ := newTestOrganizer(t)

	f, err := os.CreateTemp(t.TempDir(), "test*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Write text file larger than maxFileRead (10KB)
	content := []byte(strings.Repeat("hello world\n", 2000)) // ~24KB
	if err := os.WriteFile(f.Name(), content, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := org.readFileContent(f.Name())
	if err != nil {
		t.Fatalf("readFileContent returned error: %v", err)
	}
	if result == nil {
		t.Fatal("readFileContent returned nil for text file")
	}
	if len(result) > maxFileRead {
		t.Fatalf("text content exceeds maxFileRead: got %d bytes, max %d", len(result), maxFileRead)
	}
	if len(result) == 0 {
		t.Fatal("readFileContent returned empty slice for text file")
	}
}

func TestReadFileContent_Text_Small(t *testing.T) {
	org, _ := newTestOrganizer(t)

	f, err := os.CreateTemp(t.TempDir(), "test*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	content := []byte("hello world, this is a small file")
	if err := os.WriteFile(f.Name(), content, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := org.readFileContent(f.Name())
	if err != nil {
		t.Fatalf("readFileContent returned error: %v", err)
	}
	if result == nil {
		t.Fatal("readFileContent returned nil for small text file")
	}
	if string(result) != string(content) {
		t.Fatalf("content mismatch: got %q, expected %q", string(result), string(content))
	}
}

func TestReadFileContent_UnknownBinary(t *testing.T) {
	org, _ := newTestOrganizer(t)

	f, err := os.CreateTemp(t.TempDir(), "test*.zip")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Write ZIP magic bytes
	content := []byte{0x50, 0x4B, 0x03, 0x04, 0x00, 0x00, 0x00, 0x00}
	if err := os.WriteFile(f.Name(), content, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := org.readFileContent(f.Name())
	if err != nil {
		t.Fatalf("readFileContent returned error: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil for unknown binary, got %d bytes", len(result))
	}
}

// ============================================================
// B2: validatePath tests
// ============================================================

func TestValidatePath_InsideVault(t *testing.T) {
	org, vaultDir := newTestOrganizer(t)

	dest := filepath.Join(vaultDir, "subdir", "file.txt")
	if err := org.validatePath(dest); err != nil {
		t.Fatalf("expected no error for path inside vault, got: %v", err)
	}
}

func TestValidatePath_OutsideVault(t *testing.T) {
	org, _ := newTestOrganizer(t)

	// /tmp is outside the temp vault dir
	dest := filepath.Join(os.TempDir(), "escaped.txt")
	if err := org.validatePath(dest); err == nil {
		t.Fatal("expected error for path outside vault, got nil")
	}
}

func TestValidatePath_PathTraversal(t *testing.T) {
	org, vaultDir := newTestOrganizer(t)

	// ../etc/passwd traversal
	dest := filepath.Join(vaultDir, "..", "etc", "passwd")
	if err := org.validatePath(dest); err == nil {
		t.Fatal("expected error for path traversal, got nil")
	}
}

func TestValidatePath_VaultSubdirPrefix(t *testing.T) {
	org, vaultDir := newTestOrganizer(t)

	// Create a path that shares a prefix with vaultDir
	// e.g., vaultDir = "/tmp/Vault", path = "/tmp/Vault2/file"
	// This was the original HasPrefix bug
	parentDir := filepath.Dir(vaultDir)
	maliciousPath := filepath.Join(parentDir, filepath.Base(vaultDir)+"2", "file.txt")
	if err := org.validatePath(maliciousPath); err == nil {
		t.Fatal("expected error for subpath prefix attack, got nil")
	}
}

func TestValidatePath_VaultRoot(t *testing.T) {
	org, vaultDir := newTestOrganizer(t)

	// Path at vault root itself (no subdir)
	dest := filepath.Join(vaultDir, "file.txt")
	if err := org.validatePath(dest); err != nil {
		t.Fatalf("expected no error for path at vault root, got: %v", err)
	}
}

func TestValidatePath_DeeplyNested(t *testing.T) {
	org, vaultDir := newTestOrganizer(t)

	dest := filepath.Join(vaultDir, "a", "b", "c", "d", "file.txt")
	if err := org.validatePath(dest); err != nil {
		t.Fatalf("expected no error for nested path, got: %v", err)
	}
}

func TestValidatePath_TooLong(t *testing.T) {
	org, vaultDir := newTestOrganizer(t)

	// Create a path exceeding maxPathLen
	longPart := strings.Repeat("a", maxPathLen)
	dest := filepath.Join(vaultDir, longPart)
	if err := org.validatePath(dest); err == nil {
		t.Fatal("expected error for too-long path, got nil")
	}
}

// ============================================================
// B9: tryMarkProcessing tests
// ============================================================

func TestTryMarkProcessing_FirstCall(t *testing.T) {
	org, _ := newTestOrganizer(t)

	already := org.tryMarkProcessing("/path/to/file.txt")
	if already {
		t.Fatal("expected tryMarkProcessing to return false on first call")
	}
}

func TestTryMarkProcessing_SecondCall(t *testing.T) {
	org, _ := newTestOrganizer(t)

	org.tryMarkProcessing("/path/to/file.txt")
	already := org.tryMarkProcessing("/path/to/file.txt")
	if !already {
		t.Fatal("expected tryMarkProcessing to return true on second call")
	}
}

func TestTryMarkProcessing_Concurrent(t *testing.T) {
	org, _ := newTestOrganizer(t)

	const goroutines = 10
	results := make([]bool, goroutines)
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			results[idx] = org.tryMarkProcessing("/path/to/unique.txt")
		}(i)
	}
	wg.Wait()

	// Exactly one goroutine should have gotten false (first claim)
	claimed := 0
	for _, r := range results {
		if !r {
			claimed++
		}
	}
	if claimed != 1 {
		t.Fatalf("expected exactly 1 claim, got %d", claimed)
	}
}

func TestTryMarkProcessing_DifferentPaths(t *testing.T) {
	org, _ := newTestOrganizer(t)

	org.tryMarkProcessing("/path/file1.txt")
	org.tryMarkProcessing("/path/file2.txt")

	// Both should be marked now; different paths don't conflict
	if !org.tryMarkProcessing("/path/file1.txt") {
		t.Fatal("file1 should be marked as processed")
	}
	if !org.tryMarkProcessing("/path/file2.txt") {
		t.Fatal("file2 should be marked as processed")
	}
}

// ============================================================
// Mock-based ProcessFile test (B3, B4, B9 integration)
// ============================================================

func TestProcessFile_DuplicateSkip(t *testing.T) {
	org, _ := newTestOrganizer(t)

	// First call marks the file
	// (We skip the full pipeline; just test that the second call is rejected)
	org.state.ProcessedFiles["/test/file.txt"] = ProcessedEntry{
		ProcessedAt: time.Now(),
	}

	err := org.ProcessFile(context.Background(), "/watch", "/test/file.txt")
	if err != nil {
		t.Fatalf("expected no error for duplicate skip, got: %v", err)
	}
	// The file should still be in processed state (not removed)
	if _, ok := org.state.ProcessedFiles["/test/file.txt"]; !ok {
		t.Fatal("file should remain in processed state after duplicate skip")
	}
}

// ============================================================
// MIME detection tests (B5, B11)
// ============================================================

func TestDetectMIME_JPEG(t *testing.T) {
	mime := httpDetectContentType([]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46})
	if !strings.HasPrefix(mime, "image/jpeg") {
		t.Fatalf("expected image/jpeg, got %s", mime)
	}
}

func TestDetectMIME_PNG(t *testing.T) {
	mime := httpDetectContentType([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'})
	if !strings.HasPrefix(mime, "image/png") {
		t.Fatalf("expected image/png, got %s", mime)
	}
}

func TestDetectMIME_GIF(t *testing.T) {
	mime := httpDetectContentType([]byte{'G', 'I', 'F', '8', '9', 'a'})
	if !strings.HasPrefix(mime, "image/gif") {
		t.Fatalf("expected image/gif, got %s", mime)
	}
}

func TestDetectMIME_HEIC(t *testing.T) {
	// HEIC "ftyp" box structure
	data := []byte{
		0x00, 0x00, 0x00, 0x1C, // box size
		'f', 't', 'y', 'p', // box type
		'h', 'e', 'i', 'c', // major brand
	}
	mime := httpDetectContentType(data)
	if mime != "image/heic" {
		t.Fatalf("expected image/heic, got %s", mime)
	}
}

func TestDetectMIME_AVIF(t *testing.T) {
	// AVIF "ftyp" box structure
	data := []byte{
		0x00, 0x00, 0x00, 0x1C, // box size
		'f', 't', 'y', 'p', // box type
		'a', 'v', 'i', 'f', // major brand
	}
	mime := httpDetectContentType(data)
	if mime != "image/avif" {
		t.Fatalf("expected image/avif, got %s", mime)
	}
}

func TestDetectMIME_Text(t *testing.T) {
	mime := httpDetectContentType([]byte("hello world, this is plain text"))
	if !strings.HasPrefix(mime, "text/plain") && mime != "text/plain; charset=utf-8" {
		t.Fatalf("expected text/plain, got %s", mime)
	}
}

func TestDetectMIME_Binary(t *testing.T) {
	mime := httpDetectContentType([]byte{0x00, 0x01, 0x02, 0x03, 0x04})
	if !strings.HasPrefix(mime, "application/octet-stream") {
		t.Fatalf("expected application/octet-stream, got %s", mime)
	}
}

func TestDetectMIME_Empty(t *testing.T) {
	mime := httpDetectContentType([]byte{})
	if mime != "application/octet-stream" {
		t.Fatalf("expected application/octet-stream, got %s", mime)
	}
}
