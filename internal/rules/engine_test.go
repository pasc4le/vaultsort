package rules

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pasc4le/vaultsort/internal/config"
)

// ============================================================
// Rule matching tests
// ============================================================

func TestEngine_FindMatch_ByExtension(t *testing.T) {
	cfgRules := []config.RuleConfig{
		{
			Name:    "images",
			Enabled: true,
			Match: config.MatchConfig{
				Extension: []string{"jpg", "png"},
			},
			Action: config.ActionConfig{
				SendFile:       true,
				FallbackSubdir: "images",
			},
		},
	}

	engine, err := NewEngine(cfgRules)
	if err != nil {
		t.Fatalf("NewEngine error: %v", err)
	}

	// Create temp file with .jpg extension
	f, err := os.CreateTemp(t.TempDir(), "test*.jpg")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	rule, matched, err := engine.FindMatch(f.Name(), "")
	if err != nil {
		t.Fatalf("FindMatch error: %v", err)
	}
	if !matched {
		t.Fatal("expected match for .jpg file")
	}
	if rule.Name != "images" {
		t.Fatalf("expected 'images' rule, got %s", rule.Name)
	}
}

func TestEngine_FindMatch_NoMatch(t *testing.T) {
	cfgRules := []config.RuleConfig{
		{
			Name:    "images",
			Enabled: true,
			Match: config.MatchConfig{
				Extension: []string{"jpg"},
			},
		},
	}

	engine, err := NewEngine(cfgRules)
	if err != nil {
		t.Fatalf("NewEngine error: %v", err)
	}

	f, err := os.CreateTemp(t.TempDir(), "test*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	_, matched, err := engine.FindMatch(f.Name(), "")
	if err != nil {
		t.Fatalf("FindMatch error: %v", err)
	}
	if matched {
		t.Fatal("expected no match for .pdf file against jpg-only rule")
	}
}

func TestEngine_FindMatch_DisabledRule(t *testing.T) {
	cfgRules := []config.RuleConfig{
		{
			Name:    "disabled",
			Enabled: false,
			Match: config.MatchConfig{
				Extension: []string{"txt"},
			},
		},
		{
			Name:    "enabled",
			Enabled: true,
			Match: config.MatchConfig{
				Extension: []string{"txt"},
			},
		},
	}

	engine, err := NewEngine(cfgRules)
	if err != nil {
		t.Fatalf("NewEngine error: %v", err)
	}

	f, err := os.CreateTemp(t.TempDir(), "test*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	rule, matched, err := engine.FindMatch(f.Name(), "")
	if err != nil {
		t.Fatalf("FindMatch error: %v", err)
	}
	if !matched {
		t.Fatal("expected match for .txt file")
	}
	if rule.Name != "enabled" {
		t.Fatalf("expected 'enabled' rule, got %s", rule.Name)
	}
}

func TestEngine_FindMatch_StartsWith(t *testing.T) {
	cfgRules := []config.RuleConfig{
		{
			Name:    "screenshots",
			Enabled: true,
			Match: config.MatchConfig{
				StartsWith: "Screen",
			},
		},
	}

	engine, err := NewEngine(cfgRules)
	if err != nil {
		t.Fatalf("NewEngine error: %v", err)
	}

	f, err := os.CreateTemp(t.TempDir(), "Screenshot*.png")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	base := filepath.Base(f.Name())
	if len(base) < 10 || base[:10] != "Screenshot" {
		// os.CreateTemp might not preserve prefix exactly; adjust
		// Actually CreateTemp uses a pattern like "Screenshot*.png" and creates "Screenshot1234567890.png"
	}

	rule, matched, err := engine.FindMatch(f.Name(), "")
	if err != nil {
		t.Fatalf("FindMatch error: %v", err)
	}
	if !matched {
		t.Fatalf("expected match for file starting with 'Screen', got name: %s", filepath.Base(f.Name()))
	}
	if rule.Name != "screenshots" {
		t.Fatalf("expected 'screenshots' rule, got %s", rule.Name)
	}
}

func TestEngine_FindMatch_StartDir(t *testing.T) {
	cfgRules := []config.RuleConfig{
		{
			Name:    "downloads",
			Enabled: true,
			Match: config.MatchConfig{
				Extension: []string{"pdf"},
			},
			Action: config.ActionConfig{
				StartDir: "Downloads",
			},
		},
	}

	engine, err := NewEngine(cfgRules)
	if err != nil {
		t.Fatalf("NewEngine error: %v", err)
	}

	f, err := os.CreateTemp(t.TempDir(), "test*.pdf")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// Should match when watchDir is "Downloads"
	rule, matched, err := engine.FindMatch(f.Name(), "/Users/x/Downloads")
	if err != nil {
		t.Fatalf("FindMatch error: %v", err)
	}
	if !matched {
		t.Fatal("expected match for PDF in Downloads watch dir")
	}
	if rule.Name != "downloads" {
		t.Fatalf("expected 'downloads' rule, got %s", rule.Name)
	}

	// Should NOT match when watchDir is different
	_, matched, err = engine.FindMatch(f.Name(), "/Users/x/Desktop")
	if err != nil {
		t.Fatalf("FindMatch error: %v", err)
	}
	if matched {
		t.Fatal("expected no match for PDF in Desktop watch dir")
	}
}

func TestEngine_FindMatch_ModifiedAfter(t *testing.T) {
	now := time.Now()
	past := now.Add(-7 * 24 * time.Hour) // 7 days ago

	cfgRules := []config.RuleConfig{
		{
			Name:    "recent",
			Enabled: true,
			Match: config.MatchConfig{
				Extension:     []string{"txt"},
				ModifiedAfter: past.Format(time.RFC3339),
			},
		},
	}

	engine, err := NewEngine(cfgRules)
	if err != nil {
		t.Fatalf("NewEngine error: %v", err)
	}

	f, err := os.CreateTemp(t.TempDir(), "test*.txt")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	// File is just created, so it's modified after past
	rule, matched, err := engine.FindMatch(f.Name(), "")
	if err != nil {
		t.Fatalf("FindMatch error: %v", err)
	}
	if !matched {
		t.Fatal("expected match for recently modified file")
	}
	if rule.Name != "recent" {
		t.Fatalf("expected 'recent' rule, got %s", rule.Name)
	}
}

func TestEngine_EmptyRules(t *testing.T) {
	engine, err := NewEngine(nil)
	if err != nil {
		t.Fatalf("NewEngine error: %v", err)
	}

	_, matched, err := engine.FindMatch("/some/file.txt", "")
	if err != nil {
		t.Fatalf("FindMatch error: %v", err)
	}
	if matched {
		t.Fatal("expected no match with empty rules")
	}
}
