package watcher

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Event represents a file system event of interest.
type Event struct {
	Path      string
	WatchRoot string // the watch directory that contains this file
}

const (
	processedTTL     = 24 * time.Hour
	cleanupInterval  = 1 * time.Hour
)

// Watcher monitors directories for new files.
type Watcher struct {
	fsWatcher      *fsnotify.Watcher
	events         chan Event
	done           chan struct{}
	pollTicker     *time.Ticker
	pollInterval   time.Duration
	debounceDur    time.Duration
	processed      map[string]time.Time
	mu             sync.Mutex
	logger         *slog.Logger
	watchDirs      map[string]string // path -> watch root
	lastCleanup    time.Time
}

// ignoredFiles lists file patterns to skip.
var ignoredFiles = map[string]bool{
	".ds_store": true,
	".crdownload": true,
	".part":      true,
	".download":  true,
}

// New creates a new Watcher.
func New(pollInterval int, logger *slog.Logger) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		fsWatcher:    fsw,
		events:       make(chan Event, 100),
		done:         make(chan struct{}),
		pollInterval: time.Duration(pollInterval) * time.Second,
		debounceDur:  500 * time.Millisecond,
		processed:    make(map[string]time.Time),
		logger:       logger,
		watchDirs:    make(map[string]string),
	}, nil
}

// AddDir adds a directory to watch.
func (w *Watcher) AddDir(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return &os.PathError{Op: "watch", Path: abs, Err: os.ErrInvalid}
	}

	if err := w.fsWatcher.Add(abs); err != nil {
		return err
	}
	w.watchDirs[abs] = abs
	w.logger.Info("watching directory", "path", abs)
	return nil
}

// Start begins watching directories. Events are sent to the Events channel.
// Blocks until context is cancelled.
func (w *Watcher) Start(ctx context.Context) (<-chan Event, error) {
	w.pollTicker = time.NewTicker(w.pollInterval)

	go w.loop(ctx)

	return w.events, nil
}

// Stop stops the watcher.
func (w *Watcher) Stop() {
	if w.pollTicker != nil {
		w.pollTicker.Stop()
	}
	w.fsWatcher.Close()
	close(w.done)
}

func (w *Watcher) loop(ctx context.Context) {
	defer close(w.events)

	// debounce state
	var (
		debounceTimer *time.Timer
		debounceCh    <-chan time.Time
		pending       = make(map[string]struct{})
	)

	for {
		select {
		case <-ctx.Done():
			return

		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}
			if !w.isFileOfInterest(event.Name) {
				continue
			}

			// Debounce: accumulate events, fire after debounce duration
			pending[event.Name] = struct{}{}
			if debounceTimer == nil {
				debounceTimer = time.NewTimer(w.debounceDur)
				debounceCh = debounceTimer.C
			} else {
				debounceTimer.Reset(w.debounceDur)
			}

		case <-debounceCh:
			debounceTimer = nil
			debounceCh = nil
			w.flushPending(pending)
			pending = make(map[string]struct{})

		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			w.logger.Error("fsnotify error", "error", err)

		case <-w.pollTicker.C:
			w.pollDirectories()
		}
	}
}

func (w *Watcher) flushPending(pending map[string]struct{}) {
	w.cleanupProcessed()
	for p := range pending {
		if w.wasProcessed(p) {
			continue
		}
		w.markProcessed(p)

		root := w.findWatchRoot(p)
		w.events <- Event{Path: p, WatchRoot: root}
	}
}

func (w *Watcher) pollDirectories() {
	w.cleanupProcessed()
	for watchRoot := range w.watchDirs {
		entries, err := os.ReadDir(watchRoot)
		if err != nil {
			w.logger.Error("poll error", "path", watchRoot, "error", err)
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			fullPath := filepath.Join(watchRoot, entry.Name())
			if !w.isFileOfInterest(fullPath) {
				continue
			}
			if w.wasProcessed(fullPath) {
				continue
			}
			w.markProcessed(fullPath)

			w.logger.Debug("poll found file", "path", fullPath)
			w.events <- Event{Path: fullPath, WatchRoot: watchRoot}
		}
	}
}

func (w *Watcher) isFileOfInterest(path string) bool {
	base := strings.ToLower(filepath.Base(path))

	// Skip directories
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if info.IsDir() {
		return false
	}

	// Skip dotfiles
	if strings.HasPrefix(base, ".") {
		return false
	}

	// Skip known temp/partial files
	ext := strings.ToLower(filepath.Ext(base))
	if ignoredFiles[ext] {
		return false
	}
	if strings.HasSuffix(base, ".crdownload") ||
		strings.HasSuffix(base, ".part") ||
		strings.HasSuffix(base, ".download") {
		return false
	}

	return true
}

func (w *Watcher) findWatchRoot(path string) string {
	abs, _ := filepath.Abs(path)
	dir := filepath.Dir(abs)
	for {
		if root, ok := w.watchDirs[dir]; ok {
			return root
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return abs
		}
		dir = parent
	}
}

func (w *Watcher) wasProcessed(path string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, ok := w.processed[path]
	return ok
}

func (w *Watcher) markProcessed(path string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.processed[path] = time.Now()
}

// cleanupProcessed removes entries older than processedTTL.
func (w *Watcher) cleanupProcessed() {
	w.mu.Lock()
	defer w.mu.Unlock()

	if time.Since(w.lastCleanup) < cleanupInterval {
		return
	}
	w.lastCleanup = time.Now()

	cutoff := time.Now().Add(-processedTTL)
	for path, t := range w.processed {
		if t.Before(cutoff) {
			delete(w.processed, path)
		}
	}
}

// Reset clears the processed files map (for state recovery).
func (w *Watcher) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.processed = make(map[string]time.Time)
	w.lastCleanup = time.Now()
}
