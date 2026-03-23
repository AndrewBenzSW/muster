package vcs

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// GitLabVCS implements the VCS interface for GitLab using the glab CLI tool.
type GitLabVCS struct {
	// Dir is the working directory for git/glab commands.
	Dir string
	// Run is the CommandRunner function for executing CLI commands.
	Run CommandRunner
}

// NewGitLab creates a new GitLabVCS instance with the default ExecCommandRunner.
func NewGitLab(dir string) *GitLabVCS {
	return &GitLabVCS{
		Dir: dir,
		Run: ExecCommandRunner,
	}
}

// CheckAuth verifies that glab is installed and authenticated.
func (gl *GitLabVCS) CheckAuth() error {
	_, _, err := gl.Run(gl.Dir, "glab", "auth", "status")
	if err != nil {
		// Check if glab is not found
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("glab CLI not found: install from https://gitlab.com/gitlab-org/cli")
		}
		// Check if auth failed (exit error indicates command ran but returned non-zero)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Errorf("glab not authenticated: run 'glab auth login'")
		}
		return fmt.Errorf("glab auth check failed: %w", err)
	}
	return nil
}

// glabMRView represents the JSON output from `glab mr view --output json`.
type glabMRView struct {
	State        string `json:"state"`         // opened, merged, closed
	IID          int    `json:"iid"`           // MR number
	WebURL       string `json:"web_url"`       // Web URL
	SourceBranch string `json:"source_branch"` // Branch name
	Draft        bool   `json:"draft"`         // Draft status
}

// ViewPR retrieves the status of a GitLab merge request.
func (gl *GitLabVCS) ViewPR(ref string) (*PRStatus, error) {
	stdout, stderr, err := gl.Run(gl.Dir, "glab", "mr", "view", ref, "--output", "json")
	if err != nil {
		if strings.Contains(stderr, "no merge request found") || strings.Contains(stderr, "not found") {
			return nil, fmt.Errorf("MR not found: %s", ref)
		}
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("glab CLI not found: install from https://gitlab.com/gitlab-org/cli")
		}
		return nil, fmt.Errorf("glab mr view failed: %w (stderr: %s)", err, stderr)
	}

	var view glabMRView
	if err := json.Unmarshal([]byte(stdout), &view); err != nil {
		return nil, fmt.Errorf("failed to parse glab mr view output: %w", err)
	}

	// Map GitLab state to PRState
	var state PRState
	switch strings.ToLower(view.State) {
	case "opened":
		state = PRStateOpen
	case "merged":
		state = PRStateMerged
	case "closed":
		state = PRStateClosed
	default:
		state = PRState(strings.ToLower(view.State))
	}

	// GitLab doesn't expose reviewDecision in the same way; we'll default to ReviewStatusNone
	// A more sophisticated implementation could parse approvals, but that requires additional API calls
	reviewStatus := ReviewStatusNone

	// Get CI status from pipeline by querying the branch directly
	// We avoid calling ListChecks here to prevent infinite recursion
	ciStatus := CIStatusNone
	ciOut, _, ciErr := gl.Run(gl.Dir, "glab", "ci", "status", "-b", view.SourceBranch, "-F", "json")
	if ciErr == nil {
		var ciStatusData glabCIStatus
		if json.Unmarshal([]byte(ciOut), &ciStatusData) == nil {
			var checks []CheckResult
			for _, job := range ciStatusData {
				checks = append(checks, CheckResult{
					Name:   job.Name,
					Status: mapGitLabJobStatus(job.Status),
					URL:    job.WebURL,
				})
			}
			ciStatus = deriveCIStatus(checks)
		}
	}

	return &PRStatus{
		State:        state,
		Number:       view.IID,
		URL:          view.WebURL,
		Branch:       view.SourceBranch,
		IsDraft:      view.Draft,
		ReviewStatus: reviewStatus,
		CIStatus:     ciStatus,
	}, nil
}

// glabCIStatus represents the JSON output from `glab ci status -b <branch> -F json`.
type glabCIStatus []struct {
	Name   string `json:"name"`
	Status string `json:"status"` // success, failed, running, pending, canceled, skipped
	WebURL string `json:"web_url"`
}

// mapGitLabJobStatus maps a GitLab job status string to a CIStatus.
func mapGitLabJobStatus(status string) CIStatus {
	switch strings.ToLower(status) {
	case "success":
		return CIStatusPassing
	case "failed":
		return CIStatusFailing
	case "running", "pending":
		return CIStatusPending
	case "canceled", "skipped":
		return CIStatusPending
	default:
		return CIStatusNone
	}
}

// ListChecks retrieves all CI pipeline jobs for a GitLab merge request.
func (gl *GitLabVCS) ListChecks(ref string) ([]CheckResult, error) {
	// Get the branch name for the MR by running glab mr view directly
	mrOut, stderr, err := gl.Run(gl.Dir, "glab", "mr", "view", ref, "--output", "json")
	if err != nil {
		if strings.Contains(stderr, "no merge request found") || strings.Contains(stderr, "not found") {
			return nil, fmt.Errorf("MR not found: %s", ref)
		}
		if errors.Is(err, exec.ErrNotFound) {
			return nil, fmt.Errorf("glab CLI not found: install from https://gitlab.com/gitlab-org/cli")
		}
		return nil, fmt.Errorf("glab mr view failed: %w (stderr: %s)", err, stderr)
	}

	var mrView glabMRView
	if err := json.Unmarshal([]byte(mrOut), &mrView); err != nil {
		return nil, fmt.Errorf("failed to parse glab mr view output: %w", err)
	}

	stdout, stderr, err := gl.Run(gl.Dir, "glab", "ci", "status", "-b", mrView.SourceBranch, "-F", "json")
	if err != nil {
		if strings.Contains(stderr, "no pipeline") || strings.Contains(stderr, "not found") {
			// No CI configured
			return []CheckResult{}, nil
		}
		return nil, fmt.Errorf("glab ci status failed: %w (stderr: %s)", err, stderr)
	}

	var jobs glabCIStatus
	if err := json.Unmarshal([]byte(stdout), &jobs); err != nil {
		return nil, fmt.Errorf("failed to parse glab ci status output: %w", err)
	}

	results := make([]CheckResult, 0, len(jobs))
	for _, job := range jobs {
		results = append(results, CheckResult{
			Name:   job.Name,
			Status: mapGitLabJobStatus(job.Status),
			URL:    job.WebURL,
		})
	}

	return results, nil
}

// glabCIList represents the JSON output from `glab ci list -b <branch> --output json`.
type glabCIList []struct {
	ID     int    `json:"id"`
	Status string `json:"status"`
}

// glabCIView represents the JSON output from `glab ci view <jobID> --output json`.
type glabCIView struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Trace string `json:"trace"` // Log output
}

// GetFailedLogs retrieves logs for failed CI jobs on a GitLab merge request.
// This is best-effort: it attempts to get job IDs and fetch logs, but returns
// partial results if some operations fail.
func (gl *GitLabVCS) GetFailedLogs(ref string) ([]FailedCheckLog, error) {
	checks, err := gl.ListChecks(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to list checks: %w", err)
	}

	// Get the branch name
	prStatus, err := gl.ViewPR(ref)
	if err != nil {
		return nil, fmt.Errorf("failed to get MR info: %w", err)
	}

	// Get pipeline/job IDs for the branch
	stdout, stderr, err := gl.Run(gl.Dir, "glab", "ci", "list", "-b", prStatus.Branch, "--output", "json")
	if err != nil {
		// Best-effort: return empty slice with wrapped error
		return []FailedCheckLog{}, fmt.Errorf("failed to list pipelines (best-effort): %w (stderr: %s)", err, stderr)
	}

	var pipelines glabCIList
	if err := json.Unmarshal([]byte(stdout), &pipelines); err != nil {
		return []FailedCheckLog{}, fmt.Errorf("failed to parse glab ci list output (best-effort): %w", err)
	}

	// For each failed check, try to find matching job and fetch logs
	var logs []FailedCheckLog
	for _, check := range checks {
		if check.Status != CIStatusFailing {
			continue
		}

		// Try to find a failed pipeline/job
		// Note: This is a simplified approach. In reality, we'd need to match jobs to pipelines more precisely.
		// For now, we'll just try to fetch logs for the first failed pipeline we find.
		foundLog := false
		for _, pipeline := range pipelines {
			if strings.ToLower(pipeline.Status) == "failed" {
				// Try to get job logs
				jobIDStr := fmt.Sprintf("%d", pipeline.ID)
				stdout, stderr, err := gl.Run(gl.Dir, "glab", "ci", "view", jobIDStr, "--output", "json")
				if err != nil {
					// Best-effort: continue to next job
					logs = append(logs, FailedCheckLog{
						CheckName: check.Name,
						JobID:     jobIDStr,
						Logs:      fmt.Sprintf("Failed to retrieve logs: %v (stderr: %s)", err, stderr),
					})
					foundLog = true
					break
				}

				var jobView glabCIView
				if err := json.Unmarshal([]byte(stdout), &jobView); err != nil {
					logs = append(logs, FailedCheckLog{
						CheckName: check.Name,
						JobID:     jobIDStr,
						Logs:      fmt.Sprintf("Failed to parse job view: %v", err),
					})
					foundLog = true
					break
				}

				logs = append(logs, FailedCheckLog{
					CheckName: check.Name,
					JobID:     jobIDStr,
					Logs:      jobView.Trace,
				})
				foundLog = true
				break
			}
		}

		if !foundLog {
			// No logs found for this check
			logs = append(logs, FailedCheckLog{
				CheckName: check.Name,
				JobID:     "",
				Logs:      "No logs available",
			})
		}
	}

	return logs, nil
}

// MergePR merges a GitLab merge request and removes the source branch.
func (gl *GitLabVCS) MergePR(ref string) error {
	_, stderr, err := gl.Run(gl.Dir, "glab", "mr", "merge", ref, "--remove-source-branch", "--yes")
	if err != nil {
		if strings.Contains(stderr, "cannot be merged") || strings.Contains(stderr, "merge conflicts") {
			return fmt.Errorf("MR cannot be merged: conflicts or checks required")
		}
		if strings.Contains(stderr, "not found") {
			return fmt.Errorf("MR not found: %s", ref)
		}
		if strings.Contains(stderr, "pipeline") && strings.Contains(stderr, "failed") {
			return fmt.Errorf("MR merge blocked: pipeline failed")
		}
		return fmt.Errorf("glab mr merge failed: %w (stderr: %s)", err, stderr)
	}
	return nil
}
