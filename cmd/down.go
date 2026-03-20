package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/abenz1267/muster/internal/docker"
	"github.com/spf13/cobra"
)

var downCmd = &cobra.Command{
	Use:   "down [slug]",
	Short: "Stop and remove Docker containers",
	Long: `Stop and remove Docker containers managed by muster.

Without arguments: stops all containers for the current project.
With slug argument: stops containers matching the specified slug.
With --all: stops all containers for the current project (ignores slug argument).
With --orphans: stops containers whose slugs are no longer in_progress in the roadmap.

Examples:
  muster down                    # Stop all containers in current project
  muster down my-feature         # Stop containers for 'my-feature' slug
  muster down --orphans          # Stop orphaned containers (not in_progress)
  muster down --project myproj   # Stop all containers in 'myproj' project
  muster down --all              # Explicitly stop all containers in current project`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()

		// Get flags
		all, err := cmd.Flags().GetBool("all")
		if err != nil {
			return err
		}
		orphans, err := cmd.Flags().GetBool("orphans")
		if err != nil {
			return err
		}
		projectFlag, err := cmd.Flags().GetString("project")
		if err != nil {
			return err
		}
		verbose, _ := cmd.Flags().GetBool("verbose")

		// Determine project name
		var project string
		if projectFlag != "" {
			project = projectFlag
		} else {
			// Use current directory basename
			cwd, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}
			project = filepath.Base(cwd)
		}

		if verbose {
			fmt.Fprintf(os.Stderr, "Project: %s\n", project)
		}

		// Create Docker client
		client, err := docker.NewClient()
		if err != nil {
			return fmt.Errorf("failed to create Docker client: %w", err)
		}
		defer func() { _ = client.Close() }()

		// Check Docker is running
		pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if err := client.Ping(pingCtx); err != nil {
			return fmt.Errorf("docker check failed: %w", err)
		}

		// Determine which containers to stop
		var containersToStop []docker.ContainerInfo

		if orphans {
			// Find orphaned containers
			containersToStop, err = findOrphanContainers(ctx, client, project, verbose)
			if err != nil {
				return fmt.Errorf("failed to find orphaned containers: %w", err)
			}
		} else if all || len(args) == 0 {
			// Stop all containers for the project
			listCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			containersToStop, err = client.ListContainers(listCtx, project, "")
			if err != nil {
				return fmt.Errorf("failed to list containers: %w", err)
			}
		} else {
			// Stop containers for specific slug
			slug := args[0]
			listCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			containersToStop, err = client.ListContainers(listCtx, project, slug)
			if err != nil {
				return fmt.Errorf("failed to list containers for slug %q: %w", slug, err)
			}
		}

		if len(containersToStop) == 0 {
			fmt.Fprintf(os.Stderr, "No containers to stop.\n")
			return nil
		}

		// Display containers to be stopped
		fmt.Fprintf(os.Stderr, "Stopping %d container(s):\n", len(containersToStop))
		for _, ctr := range containersToStop {
			fmt.Fprintf(os.Stderr, "  - %s (project=%s, slug=%s, status=%s)\n",
				ctr.Name, ctr.Project, ctr.Slug, ctr.Status)
		}

		// Stop containers using docker compose down for each project
		// Group containers by project for efficient stopping
		projectGroups := make(map[string][]docker.ContainerInfo)
		for _, ctr := range containersToStop {
			projectGroups[ctr.Project] = append(projectGroups[ctr.Project], ctr)
		}

		for proj, containers := range projectGroups {
			if verbose {
				fmt.Fprintf(os.Stderr, "Stopping containers for project %s...\n", proj)
			}

			// Use docker compose down for the project
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to get home directory: %w", err)
			}
			composePath := filepath.Join(homeDir, ".cache", "muster", "compose", proj, "docker-compose.yml")

			// Check if compose file exists
			if _, err := os.Stat(composePath); err == nil {
				// Use compose down
				downCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				defer cancel()
				if err := client.ComposeDown(downCtx, composePath, proj); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: docker compose down failed for project %s: %v\n", proj, err)
					// Continue to try stopping individual containers
				} else {
					if verbose {
						fmt.Fprintf(os.Stderr, "Successfully stopped containers for project %s\n", proj)
					}
					continue
				}
			}

			// If compose file doesn't exist or compose down failed, warn user
			fmt.Fprintf(os.Stderr, "Warning: compose file not found at %s\n", composePath)
			fmt.Fprintf(os.Stderr, "Containers for project %s may still be running.\n", proj)
			fmt.Fprintf(os.Stderr, "Use 'docker ps' to check and 'docker stop <container>' to stop manually.\n")

			// List the containers for manual cleanup
			for _, ctr := range containers {
				fmt.Fprintf(os.Stderr, "  Container ID: %s (Name: %s)\n", ctr.ID, ctr.Name)
			}
		}

		return nil
	},
}

// findOrphanContainers finds containers whose slugs are no longer in_progress in the roadmap.
// Per S3: ignores containers < 1 hour old (using muster.created label).
func findOrphanContainers(ctx context.Context, client *docker.Client, project string, verbose bool) ([]docker.ContainerInfo, error) {
	// List all containers for the project
	listCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	allContainers, err := client.ListContainers(listCtx, project, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	if len(allContainers) == 0 {
		return nil, nil
	}

	// Load roadmap to check in_progress slugs
	inProgressSlugs, err := loadInProgressSlugs()
	if err != nil {
		if verbose {
			fmt.Fprintf(os.Stderr, "Warning: failed to load roadmap, treating all containers as potential orphans: %v\n", err)
		}
		// If roadmap can't be loaded, treat all containers as potential orphans (subject to age check)
		inProgressSlugs = make(map[string]bool)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "In-progress slugs: %v\n", mapKeys(inProgressSlugs))
	}

	// Filter orphaned containers
	var orphans []docker.ContainerInfo
	now := time.Now()
	for _, ctr := range allContainers {
		// Skip containers without slugs (shouldn't happen for muster-managed containers)
		if ctr.Slug == "" {
			continue
		}

		// Check if slug is in_progress
		if inProgressSlugs[ctr.Slug] {
			if verbose {
				fmt.Fprintf(os.Stderr, "Skipping %s: slug %q is in_progress\n", ctr.Name, ctr.Slug)
			}
			continue
		}

		// Check container age (ignore containers < 1 hour old)
		createdStr := ctr.Labels[docker.LabelCreated]
		if createdStr != "" {
			created, err := time.Parse(time.RFC3339, createdStr)
			if err == nil {
				age := now.Sub(created)
				if age < time.Hour {
					if verbose {
						fmt.Fprintf(os.Stderr, "Skipping %s: container is only %v old (< 1 hour)\n", ctr.Name, age.Round(time.Minute))
					}
					continue
				}
			}
		}

		// This is an orphaned container
		orphans = append(orphans, ctr)
	}

	return orphans, nil
}

// loadInProgressSlugs loads the roadmap.json file and returns a set of slugs with status "in_progress".
// Checks both .muster/roadmap.json (new location) and .roadmap.json (legacy location).
func loadInProgressSlugs() (map[string]bool, error) {
	// Try new location first
	paths := []string{
		".muster/roadmap.json",
		".roadmap.json",
	}

	var data []byte
	var err error
	for _, path := range paths {
		data, err = os.ReadFile(path) //nolint:gosec // G304: Reading roadmap file from project directory is intended behavior
		if err == nil {
			break
		}
	}

	if err != nil {
		return nil, fmt.Errorf("roadmap.json not found in .muster/ or project root: %w", err)
	}

	// Parse roadmap JSON
	// Support both array format and wrapper format
	var items []RoadmapItem

	// Try array format first
	if err := json.Unmarshal(data, &items); err != nil {
		// Try wrapper format
		var wrapper struct {
			Items []RoadmapItem `json:"items"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return nil, fmt.Errorf("failed to parse roadmap.json: %w", err)
		}
		items = wrapper.Items
	}

	// Build set of in_progress slugs
	inProgress := make(map[string]bool)
	for _, item := range items {
		if item.Status == "in_progress" && item.Slug != "" {
			inProgress[item.Slug] = true
		}
	}

	return inProgress, nil
}

// RoadmapItem represents a single item in the roadmap.
type RoadmapItem struct {
	Slug     string `json:"slug"`
	Status   string `json:"status"`
	Title    string `json:"title,omitempty"`
	Priority string `json:"priority,omitempty"`
}

// mapKeys returns the keys of a map as a slice (for debugging).
func mapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func init() {
	rootCmd.AddCommand(downCmd)

	// Add flags
	downCmd.Flags().Bool("all", false, "Stop all containers for the project")
	downCmd.Flags().Bool("orphans", false, "Stop only orphaned containers (not in_progress in roadmap)")
	downCmd.Flags().String("project", "", "Specify project name (default: current directory basename)")
}
