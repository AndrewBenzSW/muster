package vcs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// GitHubVCS implements the VCS interface for GitHub using the gh CLI tool.
type GitHubVCS struct {
	// Dir is the working directory for git/gh commands.
	Dir string
	// Run is the CommandRunner function for executing CLI commands.
	Run CommandRunner
}

// NewGitHub creates a new GitHubVCS instance with the default ExecCommandRunner.
func NewGitHub(dir string) *GitHubVCS {
	return &GitHubVCS{
		Dir: dir,
		Run: ExecCommandRunner,
	}
}

// CheckAuth verifies that gh is installed and authenticated.
func (g *GitHubVCS) CheckAuth() error {
	_, _, err := g.Run(g.Dir, "gh", "auth", "status")
	if err != nil {
		// Check if gh is not found
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("gh CLI not found: install from https://cli.github.com")
		}
		// Check if auth failed (exit error indicates command ran but returned non-zero)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("gh not authenticated: run 'gh auth login'")
		}
		return fmt.Errorf("gh auth check failed: %w", err)
	}
	return nil
}

// ghPRView represents the JSON output from `gh pr view --json`.
type ghPRView struct {
	State          string `json:"state"`          // OPEN, MERGED, CLOSED
	Number         int    `json:"number"`         // PR number
	URL            string `json:"url"`            // Web URL
	HeadRefName    string `json:"headRefName"`    // Branch name
	IsDraft        bool   `json:"isDraft"`        // Draft status
	ReviewDecision string `json:"reviewDecision"` // APPROVED, CHANGES_REQUESTED, REVIEW_REQUIRED, or empty
}

// ViewPR retrieves the status of a GitHub pull request.
func (g *GitHubVCS) ViewPR(ref string) (*PRStatus, error) {
	stdout, stderr, err := g.Run(g.Dir, "gh", "pr", "view", ref,
		"--json", "state,number,url,headRefName,isDraft,reviewDecision")
	if err != nil {
		if strings.Contains(stderr, "no pull requests found") || strings.Contains(stderr, "could not resolve") {
			return nil, fmt.Errorf("PR not found: %s", ref)
		}
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("gh CLI not found: install from https://cli.github.com")
		}
		return nil, fmt.Errorf("gh pr view failed: %w (stderr: %s)", err, stderr)
	}

	var view ghPRView
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		return nil, fmt.Errorf("failed to parse gh pr view output: %w", err)
	}

	// Map GitHub state to PRState
	var state PRState
	switch strings.ToUpper(view.State) {
	case "OPEN":
		state = PRStateOpen
	case "MERGED":
		state = PRStateMerged
	case "CLOSED":
		state = PRStateClosed
	default:
		state = PRState(strings.ToLower(view.State))
	}

	// Map reviewDecision to ReviewStatus
	var reviewStatus ReviewStatus
	switch strings.ToUpper(view.ReviewDecision) {
	case "APPROVED":
		reviewStatus = ReviewStatusApproved
	case "CHANGES_REQUESTED":
		reviewStatus = ReviewStatusChangesRequested
	case "REVIEW_REQUIRED":
		reviewStatus = ReviewStatusReviewRequired
	case "":
		reviewStatus = ReviewStatusNone
	default:
		reviewStatus = ReviewStatusNone
	}

	// Get CI status from checks
	checks, err := g.ListChecks(ref)
	ciStatus := CIStatusNone
	if err == nil {
		ciStatus = deriveCIStatus(checks)
	}

	return &PRStatus{
		State:        state,
		Number:       view.Number,
		URL:          view.URL,
		Branch:       view.HeadRefName,
		IsDraft:      view.IsDraft,
		ReviewStatus: reviewStatus,
		CIStatus:     ciStatus,
	}, nil
}

// ghCheck represents a single check from `gh pr checks --json`.
type ghCheck struct {
	Bucket string `json:"bucket"` // pass, fail, pending
	Name   string `json:"name"`
	State  string `json:"state"`
	Link   string `json:"link"`
}

// ListChecks retrieves all CI checks for a GitHub pull request.
func (g *GitHubVCS) ListChecks(ref string) ([]CheckResult, error) {
	stdout, stderr, err := g.Run(g.Dir, "gh", "pr", "checks", ref,
		"--json", "bucket,name,state,link")
	if err != nil {
		if strings.Contains(stderr, "no pull requests found") || strings.Contains(stderr, "could not resolve") {
			return nil, fmt.Errorf("PR not found: %s", ref)
		}
		if strings.Contains(stderr, "no checks") || strings.Contains(stdout, "[]") {
			// No checks configured
			return []CheckResult{}, nil
		}
		return nil, fmt.Errorf("gh pr checks failed: %w (stderr: %s)", err, stderr)
	}

	var checks []ghCheck
	if err := json.Unmarshal([]byte(stdout), &checks); err != nil {
		return nil, fmt.Errorf("failed to parse gh pr checks output: %w", err)
	}

	results := make([]CheckResult, 0, len(checks))
	for _, check := range checks {
		var status CIStatus
		switch strings.ToLower(check.Bucket) {
		case "pass":
			status = CIStatusPassing
		case "fail":
			status = CIStatusFailing
		case "pending":
			status = CIStatusPending
		default:
			status = CIStatusNone
		}

		results = append(results, CheckResult{
			Name:   check.Name,
			Status: status,
			URL:    check.Link,
		})
	}

	return results, nil
}

// GetFailedLogs retrieves logs for failed CI checks on a GitHub pull request.
// It extracts GitHub Actions run IDs from check URLs and fetches logs using `gh run view --log-failed`.
// For non-GitHub-Actions checks (third-party status checks), it skips them gracefully.
func (g *GitHubVCS) GetFailedLogs(ref string) ([]FailedCheckLog, error) {
	checks, err := g.ListChecks(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to list checks: %w", err)
	}

	var logs []FailedCheckLog
	for _, check := range checks {
		if check.Status != CIStatusFailing {
			continue
		}

		// Extract GitHub Actions run ID from URL
		// Format: https://github.com/owner/repo/actions/runs/<runID>/job/<jobID>
		runID := extractGitHubActionsRunID(check.URL)
		if runID == "" {
			// Not a GitHub Actions check or URL format not recognized, skip
			continue
		}

		// Fetch failed logs for this run
		stdout, stderr, err := g.Run(g.Dir, "gh", "run", "view", runID, "--log-failed")
		if err != nil {
			// Log retrieval failed, but continue with other checks (best-effort)
			logs = append(logs, FailedCheckLog{
				CheckName: check.Name,
				JobID:     runID,
				Logs:      fmt.Sprintf("Failed to retrieve logs: %v (stderr: %s)", err, stderr),
			})
			continue
		}

		logs = append(logs, FailedCheckLog{
			CheckName: check.Name,
			JobID:     runID,
			Logs:      stdout,
		})
	}

	return logs, nil
}

// extractGitHubActionsRunID extracts the run ID from a GitHub Actions check URL.
// Returns empty string if the URL doesn't match the expected format.
func extractGitHubActionsRunID(url string) string {
	// Expected format: https://github.com/owner/repo/actions/runs/<runID>/job/<jobID>
	// or: https://github.com/owner/repo/actions/runs/<runID>
	parts := strings.Split(url, "/runs/")
	if len(parts) != 2 {
		return ""
	}

	// Extract the numeric run ID (before the next slash or end of string)
	afterRuns := parts[1]
	runIDStr := afterRuns
	if idx := strings.Index(afterRuns, "/"); idx != -1 {
		runIDStr = afterRuns[:idx]
	}

	// Validate it's numeric
	if _, err := strconv.Atoi(runIDStr); err != nil {
		return ""
	}

	return runIDStr
}

// MergePR merges a GitHub pull request and deletes the source branch.
func (g *GitHubVCS) MergePR(ref string) error {
	_, stderr, err := g.Run(g.Dir, "gh", "pr", "merge", ref, "--delete-branch")
	if err != nil {
		if strings.Contains(stderr, "pull request is not mergeable") {
			return fmt.Errorf("PR cannot be merged: conflicts or checks required")
		}
		if strings.Contains(stderr, "no pull requests found") {
			return fmt.Errorf("PR not found: %s", ref)
		}
		if strings.Contains(stderr, "review required") || strings.Contains(stderr, "reviews required") {
			return fmt.Errorf("PR merge blocked: review required")
		}
		return fmt.Errorf("gh pr merge failed: %w (stderr: %s)", err, stderr)
	}
	return nil
}

// deriveCIStatus determines the overall CI status from a list of checks.
func deriveCIStatus(checks []CheckResult) CIStatus {
	if len(checks) == 0 {
		return CIStatusNone
	}

	hasFailing := false
	hasPending := false
	for _, check := range checks {
		if check.Status == CIStatusFailing {
			hasFailing = true
		}
		if check.Status == CIStatusPending {
			hasPending = true
		}
	}

	if hasFailing {
		return CIStatusFailing
	}
	if hasPending {
		return CIStatusPending
	}
	return CIStatusPassing
}
