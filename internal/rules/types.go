package rules

import (
	"time"

	"github.com/pasc4le/vaultsort/internal/config"
)

// Rule is the runtime representation of a configured rule.
type Rule struct {
	Name        string
	Description string
	Enabled     bool
	Match       MatchCriteria
	Action      ActionConfig
}

// MatchCriteria holds pre-parsed matching criteria.
type MatchCriteria struct {
	Extension     []string
	StartsWith    string
	EndsWith      string
	CreatedAfter  *time.Time
	ModifiedAfter *time.Time
}

// ActionConfig holds the action to take when a rule matches.
type ActionConfig struct {
	SendFile       bool
	SendFilename   bool
	Prompt         string
	EndDir         string
	StartDir       string
	FallbackSubdir string
}

// MatchResult describes the outcome of matching a file against rules.
type MatchResult struct {
	Rule    *Rule
	Matched bool
}

// NewRule creates a Rule from a config RuleConfig.
func NewRule(cfg config.RuleConfig) (*Rule, error) {
	createdAfter, modifiedAfter, err := cfg.Match.ParseDurations()
	if err != nil {
		return nil, err
	}

	return &Rule{
		Name:        cfg.Name,
		Description: cfg.Description,
		Enabled:     cfg.Enabled,
		Match: MatchCriteria{
			Extension:     cfg.Match.Extension,
			StartsWith:    cfg.Match.StartsWith,
			EndsWith:      cfg.Match.EndsWith,
			CreatedAfter:  createdAfter,
			ModifiedAfter: modifiedAfter,
		},
		Action: ActionConfig{
			SendFile:       cfg.Action.SendFile,
			SendFilename:   cfg.Action.SendFilename,
			Prompt:         cfg.Action.Prompt,
			EndDir:         cfg.Action.EndDir,
			StartDir:       cfg.Action.StartDir,
			FallbackSubdir: cfg.Action.FallbackSubdir,
		},
	}, nil
}
