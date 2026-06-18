# Spec: TUI Update Footer

## Requirement

A persistent footer bar at the bottom of the TUI screen showing the current version and, when an update is available, a notification message. The footer is always visible regardless of the current view.

## Scope

- Add `updateInfo` field to the TUI model
- Add `SetUpdateInfo()` method on the model
- Render footer in the `View()` method for all views
- Styling: dim/italic appearance

## Implementation

### Model Changes (`model.go`)

Add to the `model` struct:

```go
type model struct {
    // ... existing fields ...

    // Update information
    updateInfo UpdateInfo
}
```

Add `SetUpdateInfo()`:

```go
func (m *model) SetUpdateInfo(info UpdateInfo) {
    m.updateInfo = info
}
```

Where `UpdateInfo` is defined in the TUI package (not to be confused with the `update.UpdateInfo` struct from the infrastructure package — this is a TUI-specific DTO):

```go
type UpdateInfo struct {
    CurrentVersion string
    LatestVersion  string
    HasUpdate      bool
}
```

### Footer Rendering

Add a `renderFooter()` method:

```go
func (m *model) renderFooter() string {
    if m.updateInfo.HasUpdate {
        return dimItalicStyle.Render(
            fmt.Sprintf("aimux v%s · v%s available — run `aimux update`", m.updateInfo.CurrentVersion, m.updateInfo.LatestVersion),
        )
    }
    return dimItalicStyle.Render(fmt.Sprintf("aimux v%s", m.updateInfo.CurrentVersion))
}
```

When `UpdateInfo` has not been populated yet (the background check is still running or hasn't been set), show the version without an update check:

```go
func (m *model) renderFooter() string {
    version := m.updateInfo.CurrentVersion
    if version == "" {
        version = "?"
    }
    if m.updateInfo.HasUpdate {
        return dimItalicStyle.Render(
            fmt.Sprintf("aimux v%s · v%s available — run `aimux update`", version, m.updateInfo.LatestVersion),
        )
    }
    return dimItalicStyle.Render(fmt.Sprintf("aimux v%s", version))
}
```

### Style Definition (`styles.go` or inline)

```go
var dimItalicStyle = lipgloss.NewStyle().
    Italic(true).
    Foreground(lipgloss.AdaptiveColor{Light: "#9B9B9B", Dark: "#626262"})
```

This matches the existing styling conventions in the codebase. If no `styles.go` exists, add the style as a package-level variable in `model.go` or a new `styles.go` file.

### View Integration

In `model.View()`, append the footer to every view's output:

```go
func (m *model) View() string {
    var content string

    // ... existing view logic ...

    content = // whatever the current view renders

    return content + "\n" + m.renderFooter()
}
```

The footer must be appended AFTER the main content, separated by a newline. Because Bubble Tea renders with `AltScreen()`, the footer naturally sits at the bottom.

### View-Specific Integration Points

**dashboardView**: `RenderTable(...) + "\n" + RenderMenu(...) + "\n" + footer`
**providerListView**: `RenderProviderList(...) + "\n" + footer`
**switchConfirmationView**: confirmation message + `"\n" + footer`
**errorView**: error message + `"\n" + footer`
**All views**: footer appended at the end

For views with active forms (addProviderView, deleteProviderView, etc.), the form rendering takes over the full `View()` return — the footer is NOT appended during active form rendering. Forms handle their own layout.

### Initial State

When the model is first created (before the update check completes):

- `updateInfo` is the zero value: `UpdateInfo{}`
- `CurrentVersion` is `""` (renders as `"?"` in the footer)
- `HasUpdate` is `false`
- Footer shows: `aimux v?`

## Scenarios

### S1: Update available shows notification footer

**Given** the model has `HasUpdate=true`, `CurrentVersion="1.2.0"`, `LatestVersion="1.3.0"`
**When** the view renders
**Then** the footer shows: `aimux v1.2.0 · v1.3.0 available — run \`aimux update\``
**And** the text is styled with dim/italic

### S2: No update available shows version only

**Given** the model has `HasUpdate=false`, `CurrentVersion="1.2.0"`
**When** the view renders
**Then** the footer shows: `aimux v1.2.0`
**And** the text is styled with dim/italic

### S3: Footer visible across all views

**Given** the TUI is in dashboard view
**When** the view renders
**Then** the footer is visible at the bottom

**Given** the TUI switches to providerListView
**When** the view renders
**Then** the footer is visible at the bottom

**Given** the TUI switches to errorView
**When** the view renders
**Then** the footer is visible at the bottom

### S4: Footer not shown during active form

**Given** the user is filling out a form (addProviderView, deleteProviderView, etc.)
**When** the form renders
**Then** the footer is NOT appended to the form view
**And** the form controls its own full rendering

### S5: Footer style is dim/italic

**Given** the footer is rendered
**When** inspected
**Then** it uses italic rendering
**And** the color is a muted gray (not the primary text color)

### S6: Before update check completes

**Given** the background update check has not completed yet
**When** the TUI renders for the first time
**Then** the footer shows `aimux v?`
**And** no error is shown
**And** the TUI is not blocked
