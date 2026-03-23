// Package vcs provides a unified interface for interacting with version control
// systems through their CLI tools. It wraps the gh (GitHub) and glab (GitLab)
// CLI tools with exec.Command to check PR/MR status, CI results, and merge operations.
//
// The VCS interface abstracts over platform-specific differences, allowing
// muster to work with both GitHub and GitLab repositories. Implementations
// use CommandRunner function injection for testability.
package vcs

import (
	"bytes"
	"os/exec"
	"strings"
)

// PRState represents the state of a pull/merge request.
type PRState string

const (
	// PRStateOpen indicates the PR/MR is open and active.
	PRStateOpen PRState = "open"
	// PRStateClosed indicates the PR/MR was closed without merging.
	PRStateClosed PRState = "closed"
	// PRStateMerged indicates the PR/MR was successfully merged.
	PRStateMerged PRState = "merged"
)

// CIStatus represents the overall CI/pipeline status.
type CIStatus string

const (
	// CIStatusPending indicates CI is still running.
	CIStatusPending CIStatus = "pending"
	// CIStatusPassing indicates all CI checks passed.
	CIStatusPassing CIStatus = "passing"
	// CIStatusFailing indicates one or more CI checks failed.
	CIStatusFailing CIStatus = "failing"
	// CIStatusNone indicates no CI is configured.
	CIStatusNone CIStatus = "none"
)

// ReviewStatus represents the code review status.
type ReviewStatus string

const (
	// ReviewStatusApproved indicates the PR/MR has been approved.
	ReviewStatusApproved ReviewStatus = "approved"
	// ReviewStatusChangesRequested indicates changes were requested.
	ReviewStatusChangesRequested ReviewStatus = "changes_requested"
	// ReviewStatusReviewRequired indicates review is needed.
	ReviewStatusReviewRequired ReviewStatus = "review_required"
	// ReviewStatusNone indicates no review process is configured.
	ReviewStatusNone ReviewStatus = "none"
)

// PRStatus contains the complete status of a pull/merge request.
type PRStatus struct {
	// State is the PR/MR state (open/closed/merged).
	State PRState
	// Number is the PR/MR number.
	Number int
	// URL is the web URL of the PR/MR.
	URL string
	// Branch is the head branch name.
	Branch string
	// IsDraft indicates if this is a draft PR/MR.
	IsDraft bool
	// ReviewStatus is the review state.
	ReviewStatus ReviewStatus
	// CIStatus is the overall CI state.
	CIStatus CIStatus
}

// CheckResult represents the result of a single CI check/job.
type CheckResult struct {
	// Name is the check/job name.
	Name string
	// Status is the check status (pending/passing/failing).
	Status CIStatus
	// URL is the link to the check/job details (optional).
	URL string
}

// FailedCheckLog contains logs from a failed CI check.
type FailedCheckLog struct {
	// CheckName is the name of the failed check.
	CheckName string
	// JobID is the platform-specific job/run identifier (optional).
	JobID string
	// Logs is the captured log output.
	Logs string
}

// CommandRunner is a function type that executes a command in a given directory
// and returns stdout, stderr, and error. This abstraction allows tests to inject
// mock implementations without actually invoking CLI tools.
type CommandRunner func(dir string, name string, args ...string) (stdout string, stderr string, err error)

// ExecCommandRunner is the default CommandRunner implementation that uses exec.Command.
// It sets the working directory and captures stdout/stderr to strings.
func ExecCommandRunner(dir string, name string, args ...string) (string, string, error) {
	//nolint:gosec // G204: Command and args come from internal VCS implementations, not user input
	cmd := exec.Command(name, args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

// VCS is the interface for version control system operations.
// It provides methods for checking authentication, viewing PR/MR status,
// inspecting CI checks, retrieving failed logs, and merging.
type VCS interface {
	// CheckAuth verifies that the CLI tool is authenticated.
	// Returns an error if not authenticated or if the tool is not installed.
	CheckAuth() error

	// ViewPR retrieves the status of a PR/MR by reference (number, branch, or URL).
	// Returns an error if the PR/MR is not found or if there's a CLI error.
	ViewPR(ref string) (*PRStatus, error)

	// ListChecks retrieves all CI checks for a PR/MR by reference.
	// Returns an empty slice if no checks are configured.
	ListChecks(ref string) ([]CheckResult, error)

	// GetFailedLogs retrieves logs for failed CI checks for a PR/MR by reference.
	// Returns partial results if some logs cannot be retrieved (best-effort).
	// Returns an empty slice and error if log retrieval is not supported or fails.
	GetFailedLogs(ref string) ([]FailedCheckLog, error)

	// MergePR merges a PR/MR by reference and deletes the source branch.
	// Returns an error if merge fails due to conflicts, review requirements, or CI failures.
	MergePR(ref string) error
}
