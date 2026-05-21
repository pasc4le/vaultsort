package watcher

import (
	"log/slog"
	"os"
	"testing"
	"time"
)

// ============================================================
// B8: cleanupProcessed tests
// ============================================================

func newTestWatcher(t *testing.T) *Watcher {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	w, err := New(30, logger)
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}
	return w
}

func TestCleanupProcessed_RemovesOldEntries(t *testing.T) {
	w := newTestWatcher(t)

	// Add old entries (older than processedTTL)
	w.mu.Lock()
	w.processed["/old/file1.txt"] = time.Now().Add(-48 * time.Hour) // 2 days ago
	w.processed["/old/file2.txt"] = time.Now().Add(-36 * time.Hour) // 36h ago
	w.processed["/recent/file.txt"] = time.Now().Add(-1 * time.Hour) // 1h ago (recent)
	w.mu.Unlock()

	w.cleanupProcessed()

	w.mu.Lock()
	defer w.mu.Unlock()

	// Old entries should be removed
	if _, ok := w.processed["/old/file1.txt"]; ok {
		t.Fatal("old entry /old/file1.txt should have been cleaned up")
	}
	if _, ok := w.processed["/old/file2.txt"]; ok {
		t.Fatal("old entry /old/file2.txt should have been cleaned up")
	}
	// Recent entry should remain
	if _, ok := w.processed["/recent/file.txt"]; !ok {
		t.Fatal("recent entry /recent/file.txt should still be present")
	}
}

func TestCleanupProcessed_KeepsAllRecent(t *testing.T) {
	w := newTestWatcher(t)

	// Add entries all within TTL
	w.mu.Lock()
	w.processed["/a.txt"] = time.Now().Add(-1 * time.Hour)
	w.processed["/b.txt"] = time.Now().Add(-2 * time.Hour)
	w.processed["/c.txt"] = time.Now()
	w.mu.Unlock()

	w.cleanupProcessed()

	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.processed) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(w.processed))
	}
}

func TestCleanupProcessed_EmptyMap(t *testing.T) {
	w := newTestWatcher(t)

	w.cleanupProcessed()

	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.processed) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(w.processed))
	}
}

func TestCleanupProcessed_MixedOldAndRecent(t *testing.T) {
	w := newTestWatcher(t)

	w.mu.Lock()
	w.processed["/old/a.txt"] = time.Now().Add(-48 * time.Hour)
	w.processed["/recent/b.txt"] = time.Now().Add(-10 * time.Minute)
	w.processed["/old/c.txt"] = time.Now().Add(-72 * time.Hour)
	w.processed["/recent/d.txt"] = time.Now()
	w.mu.Unlock()

	w.cleanupProcessed()

	w.mu.Lock()
	defer w.mu.Unlock()

	if _, ok := w.processed["/old/a.txt"]; ok {
		t.Fatal("old entry should be removed")
	}
	if _, ok := w.processed["/old/c.txt"]; ok {
		t.Fatal("old entry should be removed")
	}
	if _, ok := w.processed["/recent/b.txt"]; !ok {
		t.Fatal("recent entry should be kept")
	}
	if _, ok := w.processed["/recent/d.txt"]; !ok {
		t.Fatal("recent entry should be kept")
	}
}

func TestCleanupProcessed_DoesNotRunTooOften(t *testing.T) {
	w := newTestWatcher(t)

	w.mu.Lock()
	w.processed["/old/file.txt"] = time.Now().Add(-48 * time.Hour)
	w.lastCleanup = time.Now() // just cleaned
	w.mu.Unlock()

	w.cleanupProcessed()

	w.mu.Lock()
	defer w.mu.Unlock()

	// Should NOT have cleaned because lastCleanup is recent
	if _, ok := w.processed["/old/file.txt"]; !ok {
		t.Fatal("cleanup should not run due to cleanupInterval guard")
	}
}

func TestReset_ClearsProcessed(t *testing.T) {
	w := newTestWatcher(t)

	w.mu.Lock()
	w.processed["/file.txt"] = time.Now()
	w.mu.Unlock()

	w.Reset()

	w.mu.Lock()
	defer w.mu.Unlock()

	if len(w.processed) != 0 {
		t.Fatalf("expected empty processed map after Reset, got %d entries", len(w.processed))
	}
}

func TestWasProcessed_And_MarkProcessed(t *testing.T) {
	w := newTestWatcher(t)

	if w.wasProcessed("/new/file.txt") {
		t.Fatal("file should not be marked as processed yet")
	}

	w.markProcessed("/new/file.txt")

	if !w.wasProcessed("/new/file.txt") {
		t.Fatal("file should be marked as processed now")
	}
}
