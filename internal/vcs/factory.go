package vcs

import (
	"fmt"

	"github.com/abenz1267/muster/internal/config"
)

// New creates a VCS implementation based on the merge strategy.
// It returns an error for the "direct" strategy (VCS not needed) and
// unknown strategies.
func New(strategy, dir string) (VCS, error) {
	switch strategy {
	case config.MergeStrategyGitHubPR:
		return NewGitHub(dir), nil
	case config.MergeStrategyGitLabMR:
		return NewGitLab(dir), nil
	case config.MergeStrategyDirect:
		return nil, fmt.Errorf("VCS not needed for merge strategy %q", strategy)
	default:
		return nil, fmt.Errorf("unknown merge_strategy %q", strategy)
	}
}
