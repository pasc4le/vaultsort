package rules

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/pasc4le/vaultsort/internal/config"
)

// Engine evaluates file paths against configured rules.
type Engine struct {
	rules []*Rule
}

// NewEngine creates a rule engine from config rule entries.
func NewEngine(cfgRules []config.RuleConfig) (*Engine, error) {
	var rules []*Rule
	for i, rc := range cfgRules {
		if !rc.Enabled {
			continue
		}
		rule, err := NewRule(rc)
		if err != nil {
			return nil, fmt.Errorf("parsing rule[%d] %q: %w", i, rc.Name, err)
		}
		rules = append(rules, rule)
	}
	return &Engine{rules: rules}, nil
}

// FindMatch finds the first matching rule for a file.
// watchDir is the watch path that triggered the event (for StartDir filtering).
// Returns the matching rule and whether a match was found.
func (e *Engine) FindMatch(filePath string, watchDir string) (*Rule, bool, error) {
	for _, rule := range e.rules {
		// Check StartDir filter first (cheap check)
		if rule.Action.StartDir != "" {
			watchBase := filepath.Base(watchDir)
			if !strings.EqualFold(watchBase, rule.Action.StartDir) {
				continue
			}
		}

		matched, err := rule.Matches(filePath)
		if err != nil {
			return nil, false, fmt.Errorf("matching rule %q against %s: %w", rule.Name, filePath, err)
		}
		if matched {
			return rule, true, nil
		}
	}
	return nil, false, nil
}

// Rules returns the list of active rules.
func (e *Engine) Rules() []*Rule {
	return e.rules
}
