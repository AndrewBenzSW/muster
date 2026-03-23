package vcs

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface check
var _ VCS = &GitLabVCS{}

func TestGitLabVCS_CheckAuth_Success(t *testing.T) {
	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"glab auth status": {stdout: "Logged in to gitlab.com", stderr: "", err: nil},
		},
	}

	gl := &GitLabVCS{Dir: "/test", Run: mock.run}
	err := gl.CheckAuth()
	assert.NoError(t, err)
}

func TestGitLabVCS_CheckAuth_NotLoggedIn(t *testing.T) {
	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"glab auth status": {stdout: "", stderr: "not logged in", err: &exec.ExitError{}},
		},
	}

	gl := &GitLabVCS{Dir: "/test", Run: mock.run}
	err := gl.CheckAuth()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not authenticated")
}

func TestGitLabVCS_CheckAuth_CommandNotFound(t *testing.T) {
	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"glab auth status": {stdout: "", stderr: "", err: exec.ErrNotFound},
		},
	}

	gl := &GitLabVCS{Dir: "/test", Run: mock.run}
	err := gl.CheckAuth()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGitLabVCS_ViewPR_Success(t *testing.T) {
	mrJSON := `{
		"state": "opened",
		"iid": 456,
		"web_url": "https://gitlab.com/owner/repo/-/merge_requests/456",
		"source_branch": "feature-branch",
		"draft": false
	}`

	ciStatusJSON := `[
		{"name": "test", "status": "success", "web_url": "https://gitlab.com/owner/repo/-/pipelines/789"},
		{"name": "lint", "status": "success", "web_url": "https://gitlab.com/owner/repo/-/pipelines/789"}
	]`

	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"glab mr view 456 --output json": {
				stdout: mrJSON,
				stderr: "",
				err:    nil,
			},
			"glab ci status -b feature-branch -F json": {
				stdout: ciStatusJSON,
				stderr: "",
				err:    nil,
			},
		},
	}

	gl := &GitLabVCS{Dir: "/test", Run: mock.run}
	status, err := gl.ViewPR("456")
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, PRStateOpen, status.State)
	assert.Equal(t, 456, status.Number)
	assert.Equal(t, "https://gitlab.com/owner/repo/-/merge_requests/456", status.URL)
	assert.Equal(t, "feature-branch", status.Branch)
	assert.False(t, status.IsDraft)
	assert.Equal(t, ReviewStatusNone, status.ReviewStatus) // GitLab doesn't expose this simply
	assert.Equal(t, CIStatusPassing, status.CIStatus)
}

func TestGitLabVCS_ViewPR_NotFound(t *testing.T) {
	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"glab mr view 999 --output json": {
				stdout: "",
				stderr: "no merge request found",
				err:    &exec.ExitError{},
			},
		},
	}

	gl := &GitLabVCS{Dir: "/test", Run: mock.run}
	status, err := gl.ViewPR("999")
	require.Error(t, err)
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "not found")
}

func TestGitLabVCS_ListChecks_AllPassing(t *testing.T) {
	mrJSON := `{
		"state": "opened",
		"iid": 456,
		"web_url": "https://gitlab.com/owner/repo/-/merge_requests/456",
		"source_branch": "feature-branch",
		"draft": false
	}`

	ciStatusJSON := `[
		{"name": "test", "status": "success", "web_url": "https://gitlab.com/owner/repo/-/pipelines/789"},
		{"name": "lint", "status": "success", "web_url": "https://gitlab.com/owner/repo/-/pipelines/789"}
	]`

	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"glab mr view 456 --output json": {
				stdout: mrJSON,
				stderr: "",
				err:    nil,
			},
			"glab ci status -b feature-branch -F json": {
				stdout: ciStatusJSON,
				stderr: "",
				err:    nil,
			},
		},
	}

	gl := &GitLabVCS{Dir: "/test", Run: mock.run}
	checks, err := gl.ListChecks("456")
	require.NoError(t, err)
	require.Len(t, checks, 2)
	assert.Equal(t, "test", checks[0].Name)
	assert.Equal(t, CIStatusPassing, checks[0].Status)
	assert.Equal(t, "lint", checks[1].Name)
	assert.Equal(t, CIStatusPassing, checks[1].Status)
}

func TestGitLabVCS_ListChecks_SomeFailing(t *testing.T) {
	mrJSON := `{
		"state": "opened",
		"iid": 456,
		"web_url": "https://gitlab.com/owner/repo/-/merge_requests/456",
		"source_branch": "feature-branch",
		"draft": false
	}`

	ciStatusJSON := `[
		{"name": "test", "status": "success", "web_url": "https://gitlab.com/owner/repo/-/pipelines/789"},
		{"name": "lint", "status": "failed", "web_url": "https://gitlab.com/owner/repo/-/pipelines/789"}
	]`

	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"glab mr view 456 --output json": {
				stdout: mrJSON,
				stderr: "",
				err:    nil,
			},
			"glab ci status -b feature-branch -F json": {
				stdout: ciStatusJSON,
				stderr: "",
				err:    nil,
			},
		},
	}

	gl := &GitLabVCS{Dir: "/test", Run: mock.run}
	checks, err := gl.ListChecks("456")
	require.NoError(t, err)
	require.Len(t, checks, 2)
	assert.Equal(t, CIStatusPassing, checks[0].Status)
	assert.Equal(t, CIStatusFailing, checks[1].Status)
}

func TestGitLabVCS_MergePR_Success(t *testing.T) {
	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"glab mr merge 456 --remove-source-branch --yes": {
				stdout: "Merged !456",
				stderr: "",
				err:    nil,
			},
		},
	}

	gl := &GitLabVCS{Dir: "/test", Run: mock.run}
	err := gl.MergePR("456")
	assert.NoError(t, err)
}

func TestGitLabVCS_MergePR_PipelineBlocked(t *testing.T) {
	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"glab mr merge 456 --remove-source-branch --yes": {
				stdout: "",
				stderr: "pipeline failed",
				err:    &exec.ExitError{},
			},
		},
	}

	gl := &GitLabVCS{Dir: "/test", Run: mock.run}
	err := gl.MergePR("456")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline failed")
}

func TestGitLabVCS_MergePR_NotFound(t *testing.T) {
	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"glab mr merge 999 --remove-source-branch --yes": {
				stdout: "",
				stderr: "not found",
				err:    &exec.ExitError{},
			},
		},
	}

	gl := &GitLabVCS{Dir: "/test", Run: mock.run}
	err := gl.MergePR("999")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGitLabVCS_GetFailedLogs_BestEffort(t *testing.T) {
	mrJSON := `{
		"state": "opened",
		"iid": 456,
		"web_url": "https://gitlab.com/owner/repo/-/merge_requests/456",
		"source_branch": "feature-branch",
		"draft": false
	}`

	ciStatusJSON := `[
		{"name": "test", "status": "failed", "web_url": "https://gitlab.com/owner/repo/-/pipelines/789"}
	]`

	ciListJSON := `[
		{"id": 100, "status": "failed"}
	]`

	ciViewJSON := `{
		"id": 100,
		"name": "test",
		"trace": "Error: test failed on line 10"
	}`

	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"glab mr view 456 --output json": {
				stdout: mrJSON,
				stderr: "",
				err:    nil,
			},
			"glab ci status -b feature-branch -F json": {
				stdout: ciStatusJSON,
				stderr: "",
				err:    nil,
			},
			"glab ci list -b feature-branch --output json": {
				stdout: ciListJSON,
				stderr: "",
				err:    nil,
			},
			"glab ci view 100 --output json": {
				stdout: ciViewJSON,
				stderr: "",
				err:    nil,
			},
		},
	}

	gl := &GitLabVCS{Dir: "/test", Run: mock.run}
	logs, err := gl.GetFailedLogs("456")
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "test", logs[0].CheckName)
	assert.Equal(t, "100", logs[0].JobID)
	assert.Contains(t, logs[0].Logs, "test failed on line 10")
}

func TestGitLabVCS_GetFailedLogs_NoLogs(t *testing.T) {
	mrJSON := `{
		"state": "opened",
		"iid": 456,
		"web_url": "https://gitlab.com/owner/repo/-/merge_requests/456",
		"source_branch": "feature-branch",
		"draft": false
	}`

	ciStatusJSON := `[
		{"name": "test", "status": "failed", "web_url": "https://gitlab.com/owner/repo/-/pipelines/789"}
	]`

	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"glab mr view 456 --output json": {
				stdout: mrJSON,
				stderr: "",
				err:    nil,
			},
			"glab ci status -b feature-branch -F json": {
				stdout: ciStatusJSON,
				stderr: "",
				err:    nil,
			},
			"glab ci list -b feature-branch --output json": {
				stdout: "",
				stderr: "no pipelines",
				err:    &exec.ExitError{},
			},
		},
	}

	gl := &GitLabVCS{Dir: "/test", Run: mock.run}
	logs, err := gl.GetFailedLogs("456")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "best-effort")
	assert.Empty(t, logs)
}
