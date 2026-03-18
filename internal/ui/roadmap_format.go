package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"text/tabwriter"

	"github.com/abenz1267/muster/internal/roadmap"
)

// RoadmapTableItem is a display struct for table output, decoupled from roadmap.RoadmapItem
type RoadmapTableItem struct {
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	Priority string `json:"priority"`
	Status   string `json:"status"`
}

// RoadmapDetailItem is a display struct for detail output, decoupled from roadmap.RoadmapItem
type RoadmapDetailItem struct {
	Slug     string  `json:"slug"`
	Title    string  `json:"title"`
	Priority string  `json:"priority"`
	Status   string  `json:"status"`
	Context  string  `json:"context"`
	PRUrl    *string `json:"pr_url,omitempty"`
	Branch   *string `json:"branch,omitempty"`
}

// FormatRoadmapTable formats a list of roadmap items as a table or JSON array
func FormatRoadmapTable(items []roadmap.RoadmapItem) (string, error) {
	mode := GetOutputMode()

	switch mode {
	case JSONMode:
		// Empty list outputs []
		if len(items) == 0 {
			return "[]", nil
		}

		// Convert to display structs
		displayItems := make([]RoadmapTableItem, len(items))
		for i, item := range items {
			displayItems[i] = RoadmapTableItem{
				Slug:     item.Slug,
				Title:    item.Title,
				Priority: string(item.Priority),
				Status:   string(item.Status),
			}
		}

		// Output unwrapped array
		data, err := json.MarshalIndent(displayItems, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil

	case TableMode:
		fallthrough
	default:
		// Empty list shows friendly message
		if len(items) == 0 {
			return "No roadmap items found. Run 'muster add' to create one.", nil
		}

		// Use tabwriter for table formatting
		var buf bytes.Buffer
		w := tabwriter.NewWriter(&buf, 0, 0, 2, ' ', 0)

		// Write header
		if _, err := fmt.Fprintln(w, "SLUG\tTITLE\tPRIORITY\tSTATUS"); err != nil {
			return "", err
		}

		// Write rows
		for _, item := range items {
			if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
				item.Slug,
				item.Title,
				item.Priority,
				item.Status,
			); err != nil {
				return "", err
			}
		}

		if err := w.Flush(); err != nil {
			return "", err
		}
		// Remove trailing newline
		result := buf.String()
		if len(result) > 0 && result[len(result)-1] == '\n' {
			result = result[:len(result)-1]
		}
		return result, nil
	}
}

// FormatRoadmapDetail formats a single roadmap item with full details
func FormatRoadmapDetail(item roadmap.RoadmapItem) (string, error) {
	mode := GetOutputMode()

	switch mode {
	case JSONMode:
		// Convert to display struct
		displayItem := RoadmapDetailItem{
			Slug:     item.Slug,
			Title:    item.Title,
			Priority: string(item.Priority),
			Status:   string(item.Status),
			Context:  item.Context,
			PRUrl:    item.PRUrl,
			Branch:   item.Branch,
		}

		// Output single object
		data, err := json.MarshalIndent(displayItem, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil

	case TableMode:
		fallthrough
	default:
		// Labeled key-value layout
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "Slug:     %s\n", item.Slug)
		fmt.Fprintf(&buf, "Title:    %s\n", item.Title)
		fmt.Fprintf(&buf, "Priority: %s\n", item.Priority)
		fmt.Fprintf(&buf, "Status:   %s\n", item.Status)
		fmt.Fprintf(&buf, "Context:  %s", item.Context)

		if item.PRUrl != nil {
			fmt.Fprintf(&buf, "\nPR URL:   %s", *item.PRUrl)
		}
		if item.Branch != nil {
			fmt.Fprintf(&buf, "\nBranch:   %s", *item.Branch)
		}

		return buf.String(), nil
	}
}

// FormatRoadmapItem formats a brief confirmation message for a roadmap item
func FormatRoadmapItem(item roadmap.RoadmapItem) (string, error) {
	mode := GetOutputMode()

	switch mode {
	case JSONMode:
		// Brief JSON output with key fields
		displayItem := RoadmapTableItem{
			Slug:     item.Slug,
			Title:    item.Title,
			Priority: string(item.Priority),
			Status:   string(item.Status),
		}

		data, err := json.MarshalIndent(displayItem, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil

	case TableMode:
		fallthrough
	default:
		// Brief confirmation message
		return fmt.Sprintf("Added: %s - %s [%s, %s]",
			item.Slug,
			item.Title,
			item.Priority,
			item.Status,
		), nil
	}
}
