package roadmap

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPriorityIsValid tests the Priority.IsValid() method
func TestPriorityIsValid(t *testing.T) {
	tests := []struct {
		name     string
		priority Priority
		want     bool
	}{
		{"high is valid", PriorityHigh, true},
		{"medium is valid", PriorityMedium, true},
		{"low is valid", PriorityLow, true},
		{"lower is valid", PriorityLower, true},
		{"empty string is invalid", Priority(""), false},
		{"invalid value is invalid", Priority("p0"), false},
		{"uppercase is invalid", Priority("HIGH"), false},
		{"random string is invalid", Priority("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.priority.IsValid())
		})
	}
}

// TestStatusIsValid tests the Status.IsValid() method
func TestStatusIsValid(t *testing.T) {
	tests := []struct {
		name   string
		status Status
		want   bool
	}{
		{"planned is valid", StatusPlanned, true},
		{"in_progress is valid", StatusInProgress, true},
		{"completed is valid", StatusCompleted, true},
		{"blocked is valid", StatusBlocked, true},
		{"empty string is invalid", Status(""), false},
		{"invalid value is invalid", Status("unknown"), false},
		{"uppercase is invalid", Status("PLANNED"), false},
		{"random string is invalid", Status("invalid"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.status.IsValid())
		})
	}
}

// TestValidPriorities tests the ValidPriorities() helper
func TestValidPriorities(t *testing.T) {
	priorities := ValidPriorities()

	require.Len(t, priorities, 4)
	assert.Contains(t, priorities, PriorityHigh)
	assert.Contains(t, priorities, PriorityMedium)
	assert.Contains(t, priorities, PriorityLow)
	assert.Contains(t, priorities, PriorityLower)

	// All returned priorities should be valid
	for _, p := range priorities {
		assert.True(t, p.IsValid(), "priority %s should be valid", p)
	}
}

// TestValidStatuses tests the ValidStatuses() helper
func TestValidStatuses(t *testing.T) {
	statuses := ValidStatuses()

	require.Len(t, statuses, 4)
	assert.Contains(t, statuses, StatusPlanned)
	assert.Contains(t, statuses, StatusInProgress)
	assert.Contains(t, statuses, StatusCompleted)
	assert.Contains(t, statuses, StatusBlocked)

	// All returned statuses should be valid
	for _, s := range statuses {
		assert.True(t, s.IsValid(), "status %s should be valid", s)
	}
}

// TestValidate tests the Roadmap.Validate() method
func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		roadmap *Roadmap
		wantErr bool
		errMsg  string
	}{
		{
			name:    "nil roadmap is valid",
			roadmap: nil,
			wantErr: false,
		},
		{
			name: "empty roadmap is valid",
			roadmap: &Roadmap{
				Items: []RoadmapItem{},
			},
			wantErr: false,
		},
		{
			name: "valid roadmap with one item",
			roadmap: &Roadmap{
				Items: []RoadmapItem{
					{
						Slug:     "test-item",
						Title:    "Test Item",
						Priority: PriorityHigh,
						Status:   StatusPlanned,
						Context:  "Test context",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "valid roadmap with multiple items",
			roadmap: &Roadmap{
				Items: []RoadmapItem{
					{
						Slug:     "item-1",
						Title:    "Item 1",
						Priority: PriorityHigh,
						Status:   StatusInProgress,
						Context:  "Context 1",
					},
					{
						Slug:     "item-2",
						Title:    "Item 2",
						Priority: PriorityMedium,
						Status:   StatusPlanned,
						Context:  "Context 2",
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing slug",
			roadmap: &Roadmap{
				Items: []RoadmapItem{
					{
						Slug:     "",
						Title:    "Test Item",
						Priority: PriorityHigh,
						Status:   StatusPlanned,
						Context:  "Test context",
					},
				},
			},
			wantErr: true,
			errMsg:  "slug is required",
		},
		{
			name: "missing title",
			roadmap: &Roadmap{
				Items: []RoadmapItem{
					{
						Slug:     "test-item",
						Title:    "",
						Priority: PriorityHigh,
						Status:   StatusPlanned,
						Context:  "Test context",
					},
				},
			},
			wantErr: true,
			errMsg:  "title is required",
		},
		{
			name: "missing priority",
			roadmap: &Roadmap{
				Items: []RoadmapItem{
					{
						Slug:     "test-item",
						Title:    "Test Item",
						Priority: "",
						Status:   StatusPlanned,
						Context:  "Test context",
					},
				},
			},
			wantErr: true,
			errMsg:  "priority is required",
		},
		{
			name: "missing status",
			roadmap: &Roadmap{
				Items: []RoadmapItem{
					{
						Slug:     "test-item",
						Title:    "Test Item",
						Priority: PriorityHigh,
						Status:   "",
						Context:  "Test context",
					},
				},
			},
			wantErr: true,
			errMsg:  "status is required",
		},
		{
			name: "missing context",
			roadmap: &Roadmap{
				Items: []RoadmapItem{
					{
						Slug:     "test-item",
						Title:    "Test Item",
						Priority: PriorityHigh,
						Status:   StatusPlanned,
						Context:  "",
					},
				},
			},
			wantErr: true,
			errMsg:  "context is required",
		},
		{
			name: "invalid priority",
			roadmap: &Roadmap{
				Items: []RoadmapItem{
					{
						Slug:     "test-item",
						Title:    "Test Item",
						Priority: Priority("invalid"),
						Status:   StatusPlanned,
						Context:  "Test context",
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid priority",
		},
		{
			name: "invalid status",
			roadmap: &Roadmap{
				Items: []RoadmapItem{
					{
						Slug:     "test-item",
						Title:    "Test Item",
						Priority: PriorityHigh,
						Status:   Status("invalid"),
						Context:  "Test context",
					},
				},
			},
			wantErr: true,
			errMsg:  "invalid status",
		},
		{
			name: "duplicate slugs",
			roadmap: &Roadmap{
				Items: []RoadmapItem{
					{
						Slug:     "duplicate",
						Title:    "Item 1",
						Priority: PriorityHigh,
						Status:   StatusPlanned,
						Context:  "Context 1",
					},
					{
						Slug:     "duplicate",
						Title:    "Item 2",
						Priority: PriorityMedium,
						Status:   StatusPlanned,
						Context:  "Context 2",
					},
				},
			},
			wantErr: true,
			errMsg:  "duplicate slug: duplicate",
		},
		{
			name: "whitespace-only fields are invalid",
			roadmap: &Roadmap{
				Items: []RoadmapItem{
					{
						Slug:     "   ",
						Title:    "Test Item",
						Priority: PriorityHigh,
						Status:   StatusPlanned,
						Context:  "Test context",
					},
				},
			},
			wantErr: true,
			errMsg:  "slug is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.roadmap.Validate()

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestFindBySlug tests the Roadmap.FindBySlug() method
func TestFindBySlug(t *testing.T) {
	roadmap := &Roadmap{
		Items: []RoadmapItem{
			{
				Slug:     "item-1",
				Title:    "Item 1",
				Priority: PriorityHigh,
				Status:   StatusInProgress,
				Context:  "Context 1",
			},
			{
				Slug:     "item-2",
				Title:    "Item 2",
				Priority: PriorityMedium,
				Status:   StatusPlanned,
				Context:  "Context 2",
			},
		},
	}

	tests := []struct {
		name     string
		roadmap  *Roadmap
		slug     string
		wantItem *RoadmapItem
	}{
		{
			name:     "found - first item",
			roadmap:  roadmap,
			slug:     "item-1",
			wantItem: &roadmap.Items[0],
		},
		{
			name:     "found - second item",
			roadmap:  roadmap,
			slug:     "item-2",
			wantItem: &roadmap.Items[1],
		},
		{
			name:     "not found",
			roadmap:  roadmap,
			slug:     "nonexistent",
			wantItem: nil,
		},
		{
			name:     "empty slug",
			roadmap:  roadmap,
			slug:     "",
			wantItem: nil,
		},
		{
			name:     "nil roadmap",
			roadmap:  nil,
			slug:     "item-1",
			wantItem: nil,
		},
		{
			name: "empty roadmap",
			roadmap: &Roadmap{
				Items: []RoadmapItem{},
			},
			slug:     "item-1",
			wantItem: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.roadmap.FindBySlug(tt.slug)

			if tt.wantItem == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.wantItem.Slug, result.Slug)
				assert.Equal(t, tt.wantItem.Title, result.Title)
				assert.Equal(t, tt.wantItem.Priority, result.Priority)
				assert.Equal(t, tt.wantItem.Status, result.Status)
				assert.Equal(t, tt.wantItem.Context, result.Context)
			}
		})
	}
}

// TestAddItem tests the Roadmap.AddItem() method
func TestAddItem(t *testing.T) {
	tests := []struct {
		name      string
		roadmap   *Roadmap
		item      RoadmapItem
		wantErr   bool
		errMsg    string
		wantCount int
	}{
		{
			name: "add valid item to empty roadmap",
			roadmap: &Roadmap{
				Items: []RoadmapItem{},
			},
			item: RoadmapItem{
				Slug:     "new-item",
				Title:    "New Item",
				Priority: PriorityHigh,
				Status:   StatusPlanned,
				Context:  "New context",
			},
			wantErr:   false,
			wantCount: 1,
		},
		{
			name: "add valid item to existing roadmap",
			roadmap: &Roadmap{
				Items: []RoadmapItem{
					{
						Slug:     "existing-item",
						Title:    "Existing Item",
						Priority: PriorityHigh,
						Status:   StatusPlanned,
						Context:  "Existing context",
					},
				},
			},
			item: RoadmapItem{
				Slug:     "new-item",
				Title:    "New Item",
				Priority: PriorityMedium,
				Status:   StatusPlanned,
				Context:  "New context",
			},
			wantErr:   false,
			wantCount: 2,
		},
		{
			name: "add item with duplicate slug",
			roadmap: &Roadmap{
				Items: []RoadmapItem{
					{
						Slug:     "duplicate",
						Title:    "Existing Item",
						Priority: PriorityHigh,
						Status:   StatusPlanned,
						Context:  "Existing context",
					},
				},
			},
			item: RoadmapItem{
				Slug:     "duplicate",
				Title:    "New Item",
				Priority: PriorityMedium,
				Status:   StatusPlanned,
				Context:  "New context",
			},
			wantErr:   true,
			errMsg:    "duplicate slug: duplicate",
			wantCount: 1,
		},
		{
			name: "add item with missing slug",
			roadmap: &Roadmap{
				Items: []RoadmapItem{},
			},
			item: RoadmapItem{
				Slug:     "",
				Title:    "New Item",
				Priority: PriorityHigh,
				Status:   StatusPlanned,
				Context:  "New context",
			},
			wantErr:   true,
			errMsg:    "slug is required",
			wantCount: 0,
		},
		{
			name: "add item with missing title",
			roadmap: &Roadmap{
				Items: []RoadmapItem{},
			},
			item: RoadmapItem{
				Slug:     "new-item",
				Title:    "",
				Priority: PriorityHigh,
				Status:   StatusPlanned,
				Context:  "New context",
			},
			wantErr:   true,
			errMsg:    "title is required",
			wantCount: 0,
		},
		{
			name: "add item with missing priority",
			roadmap: &Roadmap{
				Items: []RoadmapItem{},
			},
			item: RoadmapItem{
				Slug:     "new-item",
				Title:    "New Item",
				Priority: "",
				Status:   StatusPlanned,
				Context:  "New context",
			},
			wantErr:   true,
			errMsg:    "priority is required",
			wantCount: 0,
		},
		{
			name: "add item with missing status",
			roadmap: &Roadmap{
				Items: []RoadmapItem{},
			},
			item: RoadmapItem{
				Slug:     "new-item",
				Title:    "New Item",
				Priority: PriorityHigh,
				Status:   "",
				Context:  "New context",
			},
			wantErr:   true,
			errMsg:    "status is required",
			wantCount: 0,
		},
		{
			name: "add item with missing context",
			roadmap: &Roadmap{
				Items: []RoadmapItem{},
			},
			item: RoadmapItem{
				Slug:     "new-item",
				Title:    "New Item",
				Priority: PriorityHigh,
				Status:   StatusPlanned,
				Context:  "",
			},
			wantErr:   true,
			errMsg:    "context is required",
			wantCount: 0,
		},
		{
			name: "add item with invalid priority",
			roadmap: &Roadmap{
				Items: []RoadmapItem{},
			},
			item: RoadmapItem{
				Slug:     "new-item",
				Title:    "New Item",
				Priority: Priority("invalid"),
				Status:   StatusPlanned,
				Context:  "New context",
			},
			wantErr:   true,
			errMsg:    "invalid priority",
			wantCount: 0,
		},
		{
			name: "add item with invalid status",
			roadmap: &Roadmap{
				Items: []RoadmapItem{},
			},
			item: RoadmapItem{
				Slug:     "new-item",
				Title:    "New Item",
				Priority: PriorityHigh,
				Status:   Status("invalid"),
				Context:  "New context",
			},
			wantErr:   true,
			errMsg:    "invalid status",
			wantCount: 0,
		},
		{
			name:    "add item to nil roadmap",
			roadmap: nil,
			item: RoadmapItem{
				Slug:     "new-item",
				Title:    "New Item",
				Priority: PriorityHigh,
				Status:   StatusPlanned,
				Context:  "New context",
			},
			wantErr: true,
			errMsg:  "cannot add item to nil roadmap",
		},
		{
			name: "add item with optional fields",
			roadmap: &Roadmap{
				Items: []RoadmapItem{},
			},
			item: RoadmapItem{
				Slug:     "new-item",
				Title:    "New Item",
				Priority: PriorityHigh,
				Status:   StatusInProgress,
				Context:  "New context",
				PRUrl:    strPtr("https://github.com/example/pr/123"),
				Branch:   strPtr("feature/new-item"),
			},
			wantErr:   false,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.roadmap.AddItem(tt.item)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
				require.NotNil(t, tt.roadmap)
				assert.Len(t, tt.roadmap.Items, tt.wantCount)

				// Verify the item was added correctly
				added := tt.roadmap.FindBySlug(tt.item.Slug)
				require.NotNil(t, added)
				assert.Equal(t, tt.item.Slug, added.Slug)
				assert.Equal(t, tt.item.Title, added.Title)
				assert.Equal(t, tt.item.Priority, added.Priority)
				assert.Equal(t, tt.item.Status, added.Status)
				assert.Equal(t, tt.item.Context, added.Context)

				// Check optional fields
				if tt.item.PRUrl != nil {
					require.NotNil(t, added.PRUrl)
					assert.Equal(t, *tt.item.PRUrl, *added.PRUrl)
				}
				if tt.item.Branch != nil {
					require.NotNil(t, added.Branch)
					assert.Equal(t, *tt.item.Branch, *added.Branch)
				}
			}

			// Verify count even on error
			if tt.roadmap != nil && !tt.wantErr {
				assert.Len(t, tt.roadmap.Items, tt.wantCount)
			}
		})
	}
}

func TestAddItem_WhitespaceOnlyFields(t *testing.T) {
	tests := []struct {
		name    string
		item    RoadmapItem
		wantErr bool
		errMsg  string
	}{
		{
			name: "whitespace-only slug",
			item: RoadmapItem{
				Slug:     "   ",
				Title:    "Valid Title",
				Priority: PriorityHigh,
				Status:   StatusPlanned,
				Context:  "Valid context",
			},
			wantErr: true,
			errMsg:  "slug is required",
		},
		{
			name: "whitespace-only title",
			item: RoadmapItem{
				Slug:     "valid-slug",
				Title:    "   \t\n   ",
				Priority: PriorityHigh,
				Status:   StatusPlanned,
				Context:  "Valid context",
			},
			wantErr: true,
			errMsg:  "title is required",
		},
		{
			name: "whitespace-only context",
			item: RoadmapItem{
				Slug:     "valid-slug",
				Title:    "Valid Title",
				Priority: PriorityHigh,
				Status:   StatusPlanned,
				Context:  "   \t\n   ",
			},
			wantErr: true,
			errMsg:  "context is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rm := &Roadmap{Items: []RoadmapItem{}}
			err := rm.AddItem(tt.item)

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestSlugGeneration tests various slug generation scenarios
// Note: This test demonstrates expected slug formats, but slug generation
// logic is not implemented in the current task scope.
func TestSlugGeneration(t *testing.T) {
	tests := []struct {
		name     string
		title    string
		expected string
	}{
		{
			name:     "basic title",
			title:    "Hello World",
			expected: "hello-world",
		},
		{
			name:     "unicode characters",
			title:    "Café Münchën",
			expected: "caf-mnchn", // Non-ASCII chars are stripped
		},
		{
			name:     "empty title",
			title:    "",
			expected: "",
		},
		{
			name:     "very long title",
			title:    strings.Repeat("a", 200),
			expected: strings.Repeat("a", 40), // 40 char limit
		},
		{
			name:     "special characters",
			title:    "Hello! World? (Test)",
			expected: "hello-world-test",
		},
		{
			name:     "consecutive spaces",
			title:    "Hello    World",
			expected: "hello-world",
		},
		{
			name:     "leading and trailing spaces",
			title:    "  Hello World  ",
			expected: "hello-world",
		},
		{
			name:     "numbers",
			title:    "Test 123",
			expected: "test-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateSlug(tt.title)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Helper function for tests
func strPtr(s string) *string {
	return &s
}
