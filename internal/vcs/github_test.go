package vcs

import (
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Compile-time interface check
var _ VCS = &GitHubVCS{}

func TestGitHubVCS_CheckAuth_Success(t *testing.T) {
	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"gh auth status": {stdout: "Logged in to github.com", stderr: "", err: nil},
		},
	}

	gh := &GitHubVCS{Dir: "/test", Run: mock.run}
	err := gh.CheckAuth()
	assert.NoError(t, err)
}

func TestGitHubVCS_CheckAuth_NotLoggedIn(t *testing.T) {
	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"gh auth status": {stdout: "", stderr: "not logged in", err: &exec.ExitError{}},
		},
	}

	gh := &GitHubVCS{Dir: "/test", Run: mock.run}
	err := gh.CheckAuth()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not authenticated")
}

func TestGitHubVCS_CheckAuth_CommandNotFound(t *testing.T) {
	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"gh auth status": {stdout: "", stderr: "", err: exec.ErrNotFound},
		},
	}

	gh := &GitHubVCS{Dir: "/test", Run: mock.run}
	err := gh.CheckAuth()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestGitHubVCS_ViewPR_Success(t *testing.T) {
	prJSON := `{
		"state": "OPEN",
		"number": 123,
		"url": "https://github.com/owner/repo/pull/123",
		"headRefName": "feature-branch",
		"isDraft": false,
		"reviewDecision": "APPROVED"
	}`

	checksJSON := `[
		{"bucket": "pass", "name": "test", "state": "success", "link": "https://github.com/owner/repo/actions/runs/1234"},
		{"bucket": "pass", "name": "lint", "state": "success", "link": "https://github.com/owner/repo/actions/runs/1235"}
	]`

	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"gh pr view 123 --json state,number,url,headRefName,isDraft,reviewDecision": {
				stdout: prJSON,
				stderr: "",
				err:    nil,
			},
			"gh pr checks 123 --json bucket,name,state,link": {
				stdout: checksJSON,
				stderr: "",
				err:    nil,
			},
		},
	}

	gh := &GitHubVCS{Dir: "/test", Run: mock.run}
	status, err := gh.ViewPR("123")
	require.NoError(t, err)
	require.NotNil(t, status)
	assert.Equal(t, PRStateOpen, status.State)
	assert.Equal(t, 123, status.Number)
	assert.Equal(t, "https://github.com/owner/repo/pull/123", status.URL)
	assert.Equal(t, "feature-branch", status.Branch)
	assert.False(t, status.IsDraft)
	assert.Equal(t, ReviewStatusApproved, status.ReviewStatus)
	assert.Equal(t, CIStatusPassing, status.CIStatus)
}

func TestGitHubVCS_ViewPR_NotFound(t *testing.T) {
	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"gh pr view 999 --json state,number,url,headRefName,isDraft,reviewDecision": {
				stdout: "",
				stderr: "no pull requests found",
				err:    &exec.ExitError{},
			},
		},
	}

	gh := &GitHubVCS{Dir: "/test", Run: mock.run}
	status, err := gh.ViewPR("999")
	require.Error(t, err)
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "not found")
}

func TestGitHubVCS_ViewPR_AuthFailure(t *testing.T) {
	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"gh pr view 123 --json state,number,url,headRefName,isDraft,reviewDecision": {
				stdout: "",
				stderr: "",
				err:    exec.ErrNotFound,
			},
		},
	}

	gh := &GitHubVCS{Dir: "/test", Run: mock.run}
	status, err := gh.ViewPR("123")
	require.Error(t, err)
	assert.Nil(t, status)
	assert.Contains(t, err.Error(), "not found")
}

func TestGitHubVCS_ListChecks_AllPassing(t *testing.T) {
	checksJSON := `[
		{"bucket": "pass", "name": "test", "state": "success", "link": "https://github.com/owner/repo/actions/runs/1234"},
		{"bucket": "pass", "name": "lint", "state": "success", "link": "https://github.com/owner/repo/actions/runs/1235"}
	]`

	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"gh pr checks 123 --json bucket,name,state,link": {
				stdout: checksJSON,
				stderr: "",
				err:    nil,
			},
		},
	}

	gh := &GitHubVCS{Dir: "/test", Run: mock.run}
	checks, err := gh.ListChecks("123")
	require.NoError(t, err)
	require.Len(t, checks, 2)
	assert.Equal(t, "test", checks[0].Name)
	assert.Equal(t, CIStatusPassing, checks[0].Status)
	assert.Equal(t, "lint", checks[1].Name)
	assert.Equal(t, CIStatusPassing, checks[1].Status)
}

func TestGitHubVCS_ListChecks_SomeFailing(t *testing.T) {
	checksJSON := `[
		{"bucket": "pass", "name": "test", "state": "success", "link": "https://github.com/owner/repo/actions/runs/1234"},
		{"bucket": "fail", "name": "lint", "state": "failure", "link": "https://github.com/owner/repo/actions/runs/1235"}
	]`

	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"gh pr checks 123 --json bucket,name,state,link": {
				stdout: checksJSON,
				stderr: "",
				err:    nil,
			},
		},
	}

	gh := &GitHubVCS{Dir: "/test", Run: mock.run}
	checks, err := gh.ListChecks("123")
	require.NoError(t, err)
	require.Len(t, checks, 2)
	assert.Equal(t, CIStatusPassing, checks[0].Status)
	assert.Equal(t, CIStatusFailing, checks[1].Status)
}

func TestGitHubVCS_ListChecks_Pending(t *testing.T) {
	checksJSON := `[
		{"bucket": "pending", "name": "test", "state": "pending", "link": "https://github.com/owner/repo/actions/runs/1234"}
	]`

	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"gh pr checks 123 --json bucket,name,state,link": {
				stdout: checksJSON,
				stderr: "",
				err:    nil,
			},
		},
	}

	gh := &GitHubVCS{Dir: "/test", Run: mock.run}
	checks, err := gh.ListChecks("123")
	require.NoError(t, err)
	require.Len(t, checks, 1)
	assert.Equal(t, CIStatusPending, checks[0].Status)
}

func TestGitHubVCS_ListChecks_NoChecks(t *testing.T) {
	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"gh pr checks 123 --json bucket,name,state,link": {
				stdout: "[]",
				stderr: "",
				err:    nil,
			},
		},
	}

	gh := &GitHubVCS{Dir: "/test", Run: mock.run}
	checks, err := gh.ListChecks("123")
	require.NoError(t, err)
	assert.Empty(t, checks)
}

func TestGitHubVCS_GetFailedLogs_Success(t *testing.T) {
	checksJSON := `[
		{"bucket": "fail", "name": "test", "state": "failure", "link": "https://github.com/owner/repo/actions/runs/1234/job/5678"}
	]`

	runLogs := "Test failed: assertion error on line 42"

	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"gh pr checks 123 --json bucket,name,state,link": {
				stdout: checksJSON,
				stderr: "",
				err:    nil,
			},
			"gh run view 1234 --log-failed": {
				stdout: runLogs,
				stderr: "",
				err:    nil,
			},
		},
	}

	gh := &GitHubVCS{Dir: "/test", Run: mock.run}
	logs, err := gh.GetFailedLogs("123")
	require.NoError(t, err)
	require.Len(t, logs, 1)
	assert.Equal(t, "test", logs[0].CheckName)
	assert.Equal(t, "1234", logs[0].JobID)
	assert.Equal(t, runLogs, logs[0].Logs)
}

func TestGitHubVCS_GetFailedLogs_PartialFailure(t *testing.T) {
	checksJSON := `[
		{"bucket": "fail", "name": "test", "state": "failure", "link": "https://github.com/owner/repo/actions/runs/1234/job/5678"},
		{"bucket": "fail", "name": "lint", "state": "failure", "link": "https://github.com/owner/repo/actions/runs/5678/job/9999"}
	]`

	runLogs := "Test failed: assertion error on line 42"

	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"gh pr checks 123 --json bucket,name,state,link": {
				stdout: checksJSON,
				stderr: "",
				err:    nil,
			},
			"gh run view 1234 --log-failed": {
				stdout: runLogs,
				stderr: "",
				err:    nil,
			},
			"gh run view 5678 --log-failed": {
				stdout: "",
				stderr: "run not found",
				err:    &exec.ExitError{},
			},
		},
	}

	gh := &GitHubVCS{Dir: "/test", Run: mock.run}
	logs, err := gh.GetFailedLogs("123")
	require.NoError(t, err)
	require.Len(t, logs, 2)

	// First check succeeded
	assert.Equal(t, "test", logs[0].CheckName)
	assert.Equal(t, "1234", logs[0].JobID)
	assert.Equal(t, runLogs, logs[0].Logs)

	// Second check failed to retrieve logs
	assert.Equal(t, "lint", logs[1].CheckName)
	assert.Equal(t, "5678", logs[1].JobID)
	assert.Contains(t, logs[1].Logs, "Failed to retrieve logs", "should contain error message about failed log retrieval")
}

func TestGitHubVCS_MergePR_Success(t *testing.T) {
	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"gh pr merge 123 --delete-branch": {
				stdout: "Merged #123",
				stderr: "",
				err:    nil,
			},
		},
	}

	gh := &GitHubVCS{Dir: "/test", Run: mock.run}
	err := gh.MergePR("123")
	assert.NoError(t, err)
}

func TestGitHubVCS_MergePR_Conflict(t *testing.T) {
	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"gh pr merge 123 --delete-branch": {
				stdout: "",
				stderr: "pull request is not mergeable",
				err:    &exec.ExitError{},
			},
		},
	}

	gh := &GitHubVCS{Dir: "/test", Run: mock.run}
	err := gh.MergePR("123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be merged")
}

func TestGitHubVCS_MergePR_ReviewRequired(t *testing.T) {
	mock := &mockCommandRunner{
		outputs: map[string]mockOutput{
			"gh pr merge 123 --delete-branch": {
				stdout: "",
				stderr: "review required",
				err:    &exec.ExitError{},
			},
		},
	}

	gh := &GitHubVCS{Dir: "/test", Run: mock.run}
	err := gh.MergePR("123")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "review required")
}

func TestExtractGitHubActionsRunID_ValidURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected string
	}{
		{
			name:     "with job ID",
			url:      "https://github.com/owner/repo/actions/runs/1234/job/5678",
			expected: "1234",
		},
		{
			name:     "without job ID",
			url:      "https://github.com/owner/repo/actions/runs/9876",
			expected: "9876",
		},
		{
			name:     "invalid format",
			url:      "https://github.com/owner/repo/pull/123",
			expected: "",
		},
		{
			name:     "non-numeric run ID",
			url:      "https://github.com/owner/repo/actions/runs/abc",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractGitHubActionsRunID(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDeriveCIStatus(t *testing.T) {
	tests := []struct {
		name     string
		checks   []CheckResult
		expected CIStatus
	}{
		{
			name:     "no checks",
			checks:   []CheckResult{},
			expected: CIStatusNone,
		},
		{
			name: "all passing",
			checks: []CheckResult{
				{Name: "test", Status: CIStatusPassing},
				{Name: "lint", Status: CIStatusPassing},
			},
			expected: CIStatusPassing,
		},
		{
			name: "some failing",
			checks: []CheckResult{
				{Name: "test", Status: CIStatusPassing},
				{Name: "lint", Status: CIStatusFailing},
			},
			expected: CIStatusFailing,
		},
		{
			name: "some pending",
			checks: []CheckResult{
				{Name: "test", Status: CIStatusPassing},
				{Name: "lint", Status: CIStatusPending},
			},
			expected: CIStatusPending,
		},
		{
			name: "failing takes precedence over pending",
			checks: []CheckResult{
				{Name: "test", Status: CIStatusFailing},
				{Name: "lint", Status: CIStatusPending},
			},
			expected: CIStatusFailing,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deriveCIStatus(tt.checks)
			assert.Equal(t, tt.expected, result)
		})
	}
}
