// Package roadmap provides data models and file I/O for roadmap management.
//
// The roadmap package implements a structured approach to managing project roadmaps
// with typed enums for priority and status, validation of required fields, and
// persistence to/from JSON format.
package roadmap

import (
	"errors"
)

// Sentinel error for roadmap parsing
var (
	// ErrRoadmapParse indicates a JSON parsing error
	ErrRoadmapParse = errors.New("roadmap parse error")
)

// Priority represents the priority level of a roadmap item
type Priority string

const (
	// PriorityHigh represents highest priority
	PriorityHigh Priority = "high"
	// PriorityMedium represents medium priority
	PriorityMedium Priority = "medium"
	// PriorityLow represents low priority
	PriorityLow Priority = "low"
	// PriorityLower represents lowest priority
	PriorityLower Priority = "lower"
)

// IsValid returns true if the Priority value is one of the defined constants
func (p Priority) IsValid() bool {
	switch p {
	case PriorityHigh, PriorityMedium, PriorityLow, PriorityLower:
		return true
	default:
		return false
	}
}

// ValidPriorities returns a slice of all valid priority values
func ValidPriorities() []Priority {
	return []Priority{PriorityHigh, PriorityMedium, PriorityLow, PriorityLower}
}

// Status represents the current status of a roadmap item
type Status string

const (
	// StatusPlanned represents an item that is planned
	StatusPlanned Status = "planned"
	// StatusInProgress represents an item currently being worked on
	StatusInProgress Status = "in_progress"
	// StatusCompleted represents a completed item
	StatusCompleted Status = "completed"
	// StatusBlocked represents a blocked item
	StatusBlocked Status = "blocked"
)

// IsValid returns true if the Status value is one of the defined constants
func (s Status) IsValid() bool {
	switch s {
	case StatusPlanned, StatusInProgress, StatusCompleted, StatusBlocked:
		return true
	default:
		return false
	}
}

// ValidStatuses returns a slice of all valid status values
func ValidStatuses() []Status {
	return []Status{StatusPlanned, StatusInProgress, StatusCompleted, StatusBlocked}
}

// RoadmapItem represents a single item in the roadmap
type RoadmapItem struct {
	// Slug is the unique identifier for the item (required)
	Slug string `json:"slug"`

	// Title is the human-readable title (required)
	Title string `json:"title"`

	// Priority is the priority level (required)
	Priority Priority `json:"priority"`

	// Status is the current status (required)
	Status Status `json:"status"`

	// Context provides additional context or description (required)
	Context string `json:"context"`

	// PRUrl is the URL to the pull request (optional)
	PRUrl *string `json:"pr_url,omitempty"`

	// Branch is the git branch name (optional)
	Branch *string `json:"branch,omitempty"`
}

// Roadmap represents a collection of roadmap items
type Roadmap struct {
	// Items is the list of roadmap items
	Items []RoadmapItem `json:"items"`
}
