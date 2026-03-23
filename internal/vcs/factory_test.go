package vcs

import (
	"testing"

	"github.com/abenz1267/muster/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_GitHubPR(t *testing.T) {
	vcs, err := New(config.MergeStrategyGitHubPR, "/test")
	require.NoError(t, err)
	require.NotNil(t, vcs)
	_, ok := vcs.(*GitHubVCS)
	assert.True(t, ok, "expected GitHubVCS implementation")
}

func TestNew_GitLabMR(t *testing.T) {
	vcs, err := New(config.MergeStrategyGitLabMR, "/test")
	require.NoError(t, err)
	require.NotNil(t, vcs)
	_, ok := vcs.(*GitLabVCS)
	assert.True(t, ok, "expected GitLabVCS implementation")
}

func TestNew_Direct(t *testing.T) {
	vcs, err := New(config.MergeStrategyDirect, "/test")
	require.Error(t, err)
	assert.Nil(t, vcs)
	assert.Contains(t, err.Error(), "VCS not needed")
}

func TestNew_UnknownStrategy(t *testing.T) {
	vcs, err := New("unknown-strategy", "/test")
	require.Error(t, err)
	assert.Nil(t, vcs)
	assert.Contains(t, err.Error(), "unknown merge_strategy")
}
