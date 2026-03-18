# Interactive Picker Libraries

*Researched: 2026-03-18*
*Scope: Evaluation of Go CLI picker/prompt libraries for implementing internal/ui/picker.go, focused on API ergonomics, feature completeness, maintenance status, dependencies, and suitability for an interactive roadmap item selector.*

---

## Key Findings

**Recommendation: Use `charmbracelet/huh` for the roadmap item picker implementation.**

`huh` provides the best combination of modern API design, active maintenance, built-in fuzzy filtering, type safety, and integration flexibility. It offers a simpler API than raw Bubbletea while maintaining full composability for future enhancements. The library is actively maintained (last commit March 18, 2026), has 6.7k stars, and is specifically designed for forms and pickers.

**Key advantages:**
- Clean, ergonomic API with method chaining
- Generic type support for type-safe value binding
- Built-in fuzzy filtering via `.Filtering(true)`
- Seamless Bubbletea integration for future feature expansion
- Active development by Charm (same team as Bubbletea)
- Accessible mode support for better dictation/screen readers
- 10 direct dependencies, all from the Charm ecosystem

**Alternative for minimal dependencies:** If binary size or dependency count becomes a concern, `pterm` offers built-in fuzzy search with only 9 direct dependencies and no requirement for understanding Bubbletea's architecture.

---

## Detailed Analysis

### charmbracelet/huh

**GitHub:** https://github.com/charmbracelet/huh
**Stars:** 6.7k
**Last Update:** March 18, 2026 (actively maintained)
**Maintenance:** Excellent - regular commits throughout 2025-2026, part of Charm ecosystem

**Core Features:**
- Interactive forms and prompts (Select, MultiSelect, Input, Text, Confirm)
- Generic type support - `Select[T comparable]` for type-safe values
- Built-in fuzzy filtering with `.Filtering(bool)`
- Dynamic forms with `Func` variants for dependent fields
- Five predefined themes plus custom theme support
- Accessibility mode for screen readers/dictation
- Full Bubbletea integration - forms are `tea.Model` instances

**API Example:**
```go
var country string

huh.NewSelect[string]().
    Title("Pick a country.").
    Options(
        huh.NewOption("United States", "US"),
        huh.NewOption("Germany", "DE"),
        huh.NewOption("Brazil", "BR"),
    ).
    Value(&country).
    Filtering(true).
    Run()
```

**Dynamic options example:**
```go
huh.NewSelect[string]().
    Value(&state).
    OptionsFunc(func() []huh.Option[string] {
        return fetchStatesForCountry(country)
    }, &country).  // Recompute when country changes
    Run()
```

**Dependencies:** 10 direct (bubbles, bubbletea, lipgloss, catppuccin, charmbracelet/x packages, mitchellh/hashstructure), 17 indirect. All well-maintained packages from the Charm ecosystem.

**Binary Size Impact:** Moderate - includes full Bubbletea framework plus styling libraries.

**Pros:**
- Best API ergonomics of all evaluated libraries
- Type safety with generics
- Built-in fuzzy filtering
- Part of actively maintained Charm ecosystem
- Can be embedded in larger Bubbletea apps
- Dynamic options for dependent selections
- Accessibility features

**Cons:**
- Larger dependency tree than standalone libraries
- Requires basic understanding of Bubbletea for advanced usage
- Relatively newer (but built on mature Bubbletea foundation)

**Best for:** Projects already using or willing to adopt the Charm ecosystem, teams valuing API ergonomics and future extensibility.

---

### pterm/pterm

**GitHub:** https://github.com/pterm/pterm
**Stars:** 5.4k
**Last Update:** March 4, 2026 (actively maintained)
**Maintenance:** Excellent - 30+ commits in 2025, active through 2026

**Core Features:**
- Interactive Select, Multiselect, TextInput, Confirm, Continue
- Built-in fuzzy filtering with `.WithFilter(true)`
- Cross-platform support (Windows CMD, macOS, Linux)
- Max height control for option lists
- Interrupt handling with custom callbacks
- 28,774+ automated tests

**API Example:**
```go
selectedOption, _ := pterm.DefaultInteractiveSelect.
    WithOptions(options).
    WithDefaultOption("Create new project").
    WithFilter(true).
    WithFilterInputPlaceholder("Type to filter...").
    WithMaxHeight(10).
    Show("What would you like to do?")
```

**Dependencies:** 9 direct (atomicgo.dev packages, gookit/color, lithammer/fuzzysearch, mattn/go-runewidth, golang.org/x/term, golang.org/x/text), 9 indirect. Includes dedicated fuzzy search library.

**Binary Size Impact:** Moderate - includes fuzzy search and styling libraries.

**Pros:**
- Clean, straightforward API
- Built-in fuzzy filtering with dedicated library
- Excellent cross-platform support
- Well-tested (28k+ unit tests)
- No Bubbletea dependency
- Actively maintained
- Fewer dependencies than huh

**Cons:**
- Less composable than Bubbletea-based solutions
- No generic type support (strings only)
- Limited dynamic options capabilities
- Less flexibility for advanced customization

**Best for:** Projects wanting fuzzy search without Bubbletea, simpler use cases, teams prioritizing test coverage.

---

### manifoldco/promptui

**GitHub:** https://github.com/manifoldco/promptui
**Stars:** 6.4k
**Last Update:** October 30, 2021 (NOT actively maintained)
**Maintenance:** Poor - no commits since October 2021, appears abandoned

**Core Features:**
- Select and Prompt modes
- Pagination and scrolling
- Search support (requires custom Searcher implementation)
- Template-based customization with colors/styles
- Vim mode support
- Custom key bindings

**API Example:**
```go
prompt := promptui.Select{
    Label: "Select Day",
    Items: []string{"Monday", "Tuesday", "Wednesday"},
    Size:  4,
    Searcher: func(input string, index int) bool {
        return strings.Contains(strings.ToLower(items[index]), strings.ToLower(input))
    },
}

_, result, err := prompt.Run()
```

**Dependencies:** 4 total (chzyer/readline plus 3 indirect). Very minimal.

**Binary Size Impact:** Small - minimal dependencies.

**Pros:**
- Minimal dependencies
- Template-based customization
- Vim mode support
- Simple API for basic use cases
- Mature codebase

**Cons:**
- **No active maintenance since 2021**
- No built-in fuzzy search (must implement custom Searcher)
- No generic type support
- Outdated Go module version (v0.9.0)
- No recent bug fixes or feature updates
- Limited future-proofing

**Best for:** Legacy projects or scenarios where minimal dependencies outweigh maintenance concerns. **NOT recommended for new projects.**

---

### charmbracelet/bubbletea

**GitHub:** https://github.com/charmbracelet/bubbletea
**Stars:** 40.7k
**Last Update:** Recent (1,838 commits, actively maintained)
**Maintenance:** Excellent - flagship project of Charm

**Core Features:**
- Full TUI framework based on The Elm Architecture
- Cell-based renderer with color downsampling
- Keyboard and mouse event handling
- Clipboard integration
- Declarative UI programming model

**API Complexity:** Moderate to high. Requires implementing three core methods (Init, Update, View) and understanding event-driven architecture.

**Basic Example:**
```go
type model struct {
    choices  []string
    cursor   int
    selected map[int]struct{}
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        // Handle key events
    }
    return m, nil
}

func (m model) View() string {
    // Render UI
}
```

**Dependencies:** Moderate - core Charm libraries for terminal handling, styling, etc.

**Binary Size Impact:** Moderate - full TUI framework.

**Pros:**
- Most flexible and powerful option
- Can build complex, full-screen TUIs
- Active development with large community (40.7k stars)
- Strong ecosystem (Bubbles components, Lip Gloss styling)
- Future-proof architecture

**Cons:**
- Steeper learning curve
- Requires more boilerplate for simple pickers
- Overkill for basic selection menus
- Need to understand event-driven architecture

**Best for:** Complex TUI applications, projects needing custom behavior beyond standard pickers, teams willing to invest in learning the framework. **Use `huh` instead for simple pickers.**

---

### AlecAivazis/survey

**GitHub:** https://github.com/AlecAivazis/survey
**Stars:** 4.1k
**Last Update:** April 19, 2024 (ARCHIVED)
**Maintenance:** None - project explicitly archived with message "⚠️ This project is no longer maintained"

**Core Features:**
- Multiple prompt types (Input, Multiline, Password, Confirm, Select, MultiSelect, Editor)
- Built-in validators
- Filtering with customizable functions
- Help text support
- Default values

**API Example:**
```go
prompt := &survey.Select{
    Message: "Choose a color:",
    Options: []string{"red", "blue", "green"},
}
survey.AskOne(prompt, &color)
```

**Dependencies:** Unknown count, uses Go modules.

**Pros:**
- Simple API
- Good feature set
- Built-in validation

**Cons:**
- **ARCHIVED - no longer maintained**
- Author recommends Charm's Bubbletea as alternative
- No bug fixes or security updates
- Not suitable for production use

**Best for:** Nothing - **DO NOT USE**. Project is archived.

---

### charmbracelet/gum

**GitHub:** https://github.com/charmbracelet/gum
**Stars:** 23.1k
**Last Update:** Recent (actively maintained)
**Maintenance:** Excellent - part of Charm ecosystem

**Type:** CLI tool (not a Go library)

**Description:** Standalone binary for shell script enhancement, not designed for embedding in Go applications.

**Usage:** Shell scripts invoke `gum choose`, `gum filter`, etc. as external commands.

**Assessment:** Not suitable for `internal/ui/picker.go` implementation. Would require shelling out to external binary instead of using native Go API. Mentioned for completeness only.

---

### Other Libraries

**go-prompt** (https://github.com/c-bata/go-prompt)
**Stars:** 5.5k
**Focus:** Auto-completion, not selection menus
**Assessment:** Designed for python-prompt-toolkit style completion, not interactive pickers. Not suitable for this use case.

**gocui, termbox-go, liner, wmenu, strumt:**
Lower-level or less feature-complete options. Not evaluated in detail as the above libraries clearly surpass them for this specific use case.

---

## Recommendations

### Primary Recommendation: charmbracelet/huh

Use `huh` for implementing `internal/ui/picker.go`. It provides the optimal balance of:

1. **API Ergonomics:** Clean, chainable methods with sensible defaults
2. **Feature Completeness:** Built-in fuzzy filtering, validation, theming, accessibility
3. **Type Safety:** Generic types prevent runtime errors
4. **Maintenance:** Actively developed with recent commits (March 2026)
5. **Extensibility:** Full Bubbletea integration allows future enhancements
6. **Reusability:** Forms are composable across multiple commands

**Implementation approach:**

```go
// internal/ui/picker.go
package ui

import "charm.land/huh/v2"

type PickerOption struct {
    Label string
    Value string
}

func ShowPicker(title string, options []PickerOption) (string, error) {
    var selected string

    huhOptions := make([]huh.Option[string], len(options))
    for i, opt := range options {
        huhOptions[i] = huh.NewOption(opt.Label, opt.Value)
    }

    err := huh.NewForm(
        huh.NewGroup(
            huh.NewSelect[string]().
                Title(title).
                Options(huhOptions...).
                Value(&selected).
                Filtering(true).
                Height(15),
        ),
    ).Run()

    return selected, err
}
```

**API Design Patterns:**
- Accept slice of option structs (label + value) for clarity
- Enable filtering by default for better UX
- Set reasonable height (15 items) as default
- Return error for cancellation handling
- Keep simple cases simple, expose advanced options as needed

**Features to expose:**
- Basic: title, options, selected value
- Optional: default selection, height, filtering toggle, validation
- Advanced: custom themes, accessibility mode, dynamic options

### Fallback Option: pterm

If the Charm ecosystem is deemed too heavyweight or if Bubbletea integration is unwanted, use `pterm.DefaultInteractiveSelect`. It provides:

- Built-in fuzzy filtering without Bubbletea
- Fewer dependencies (9 vs 10 direct)
- Simpler mental model (no event loop)
- Excellent test coverage

**Trade-offs:**
- Less composable (harder to embed in larger TUIs)
- String-only values (no type safety)
- Less flexible for complex requirements

### Do NOT Use

- **survey:** Archived, no maintenance
- **promptui:** Not maintained since 2021
- **bubbletea directly:** Too low-level for simple pickers (use huh instead)
- **gum:** CLI tool, not a Go library

---

## Open Questions

### 1. Should we support multi-select for roadmap items?

**Why it matters:** Multi-select allows batch operations (e.g., mark multiple items as completed). The spec mentions "item selection" but doesn't specify single vs. multi.

**What I tried:** Reviewed all libraries for multi-select support. Both `huh` and `pterm` support it with minimal API changes.

**Recommendation:** Start with single-select for MVP. Add multi-select later if batch operations become necessary. `huh.NewMultiSelect()` makes this trivial to add.

### 2. What level of theming/customization is desired?

**Why it matters:** Affects library choice and API design. `huh` offers five themes plus custom styling. `pterm` has limited but sufficient styling.

**What I tried:** Examined theming APIs in both libraries. `huh` uses Catppuccin and custom theme structs. `pterm` has more limited color control.

**Recommendation:** Start with default theme. Expose theme selection as config option if users request it.

### 3. Should picker support keyboard shortcuts beyond basic navigation?

**Why it matters:** Advanced users may want vim-style navigation (j/k) or custom bindings. `promptui` has explicit vim mode. `huh` follows standard Bubbletea key handling.

**What I tried:** Reviewed key binding documentation. `huh` uses standard Bubbletea keys (arrows, enter, esc, /). `pterm` is less documented.

**Recommendation:** Default key bindings are sufficient for MVP. Both libraries support standard patterns users expect.

### 4. How should cancellation be handled?

**Why it matters:** Users pressing ESC or Ctrl+C should cleanly exit without errors. Different libraries handle this differently.

**What I tried:** Checked error handling docs. `huh` returns error on cancellation. `pterm` allows custom interrupt handlers.

**Recommendation:** Return error on cancellation. Let caller decide whether to exit, retry, or show message. This maintains flexibility across different commands (in/out/plan).

### 5. Should dynamic options be supported from day one?

**Why it matters:** Dynamic options (e.g., filter roadmap items by status) add complexity but improve UX. `huh` has built-in support via `OptionsFunc`.

**What I tried:** Reviewed dynamic options APIs. `huh` makes this first-class via `OptionsFunc(f func() []Option[T], bindings any)`. `pterm` requires manual refresh.

**Recommendation:** Design API to support dynamic options even if not used initially. `huh` makes this natural:

```go
type PickerConfig struct {
    Title           string
    Options         []PickerOption
    DynamicOptions  func() []PickerOption  // Optional
}
```

---

## References

### Primary Sources
- Huh: https://github.com/charmbracelet/huh
- Huh GoDoc: https://pkg.go.dev/github.com/charmbracelet/huh
- PTerm: https://github.com/pterm/pterm
- PTerm GoDoc: https://pkg.go.dev/github.com/pterm/pterm
- Promptui: https://github.com/manifoldco/promptui
- Promptui GoDoc: https://pkg.go.dev/github.com/manifoldco/promptui
- Bubbletea: https://github.com/charmbracelet/bubbletea
- Survey: https://github.com/AlecAivazis/survey (ARCHIVED)
- Gum: https://github.com/charmbracelet/gum
- go-prompt: https://github.com/c-bata/go-prompt

### Community Resources
- Awesome Go: https://awesome-go.com (Advanced Console UIs section)

### Commit History (Maintenance Verification)
- Huh commits: https://github.com/charmbracelet/huh/commits/main (last: March 18, 2026)
- PTerm commits: https://github.com/pterm/pterm/commits/master (last: March 4, 2026)
- Promptui commits: https://github.com/manifoldco/promptui/commits/master (last: October 30, 2021)

### Dependency Files
- Huh go.mod: https://github.com/charmbracelet/huh/blob/main/go.mod
- PTerm go.mod: https://github.com/pterm/pterm/blob/master/go.mod
- Promptui go.mod: https://github.com/manifoldco/promptui/blob/master/go.mod
