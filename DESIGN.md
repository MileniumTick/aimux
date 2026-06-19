# aimux Visual Design

> Design decisions for the aimux TUI: color palette, layout system, interaction patterns, and theme architecture.

---

## Color Palette: Synthwave Outrun

The TUI uses a neon-dark aesthetic with ultra-deep purple background and electric cyan + pink accents.

| Token | Hex | Use |
|-------|-----|-----|
| `BaseBackground` | `#0F051D` | Full-screen background, ultra-deep purple/blue void |
| `SurfaceCard` | `#1A0F30` | Card/chip backgrounds, subtle lift from base |
| `BorderMuted` | `#3D2663` | Panel borders, dividers, separators |
| `TextPrimary` | `#FFFFFF` | Headlines, selected items, focused labels |
| `TextSecondary` | `#A894D3` | Descriptions, unfocused labels, secondary text |
| `TextMuted` | `#5A4B76` | Disabled text, placeholders, captions |
| `AccentPink` | `#FF007F` | Primary accent: selections, keybind labels, focused indicators, buttons |
| `AccentCyan` | `#00F0FF` | Secondary accent: success states, data counters, active badges |
| `StatusError` | `#FF3B6B` | Error notifications, error status badges |
| `StatusWarn` | `#FFB000` | Warning notifications |

### Why this palette

- **Dark background** (`#0F051D`): Terminal-friendly, reduces eye strain in long coding sessions. Not pure black — the purple tint gives depth.
- **Neon pink + cyan** accent pair: High contrast against dark background. Pink draws attention to actionable elements. Cyan signals data/status.
- **Lavender secondary text** (`#A894D3`): Warm enough to be readable, muted enough to recede behind white primary text.
- **No green for success**: Cyan replaces green — consistent with the synthwave aesthetic.

---

## Layout System

### Hybrid Adaptive Layout

The TUI uses two layout modes, switched per-view:

| Mode | Width | Use |
|------|-------|-----|
| **Centered** | Fixed-width content, centered horizontally/vertically | Forms, menus, tables, cards, most views |
| **Fluid** | Full-width, edge-to-edge | Diff view with side-by-side panels |

**Centered mode** is the default. Content width is capped at terminal width minus padding. Forms use top vertical alignment so the search/filter bar stays visible.

**Fluid mode** only activates for the switch confirmation (dry-run diff) view, where side-by-side panels need all available space.

### Layout Regions

```
┌─ Header (1 line) ─────────────────────────────────┐
│  ◆ aimux ◆  ·  Dashboard                          │
├────────────────────────────────────────────────────┤
│                                                    │
│              Body (N lines)                        │
│       ┌─ Summary Card ────────────┐               │
│       │ Providers  3 active       │               │
│       │ CLIs       4 active       │               │
│       └───────────────────────────┘               │
│                                                    │
│       ┌─ Actions ─────────────────┐               │
│       │ ▸ Switch                  │               │
│       │   Manage Providers        │               │
│       │   Manage CLIs             │               │
│       │   Restore Backup          │               │
│       │   Exit                    │               │
│       └───────────────────────────┘               │
│                                                    │
├────────────────────────────────────────────────────┤
│  Footer (1 line)                                   │
│  ↑/k • up  ↓/j • down  enter • select   ? • help  │
└────────────────────────────────────────────────────┘
```

### Responsive Behavior

- **Minimum dimensions**: 50×15 characters. Below this, a contingency message is shown.
- **Header**: Always 1 line: logo (◆ aimux ◆) + breadcrumb path.
- **Footer**: Always 1 line: keybinding bar or notification toast.
- **Body**: Fills remaining height. Content is padded with background-color fills so the entire viewport is painted.
- **Body right-fill**: Append-only — never re-renders existing ANSI content. Preserves huh form internals.

### Components

| Component | Description |
|-----------|-------------|
| **Dashboard** | Logo + Summary card + Menu card + optional Welcome message |
| **Provider List** | Tabular view with name, status badge, base URL, model count, in-use indicator |
| **Forms** | `huh` library forms with custom Synthwave theme. Paginated groups for long forms (Identity → Endpoint → Credentials) |
| **Stepper** | 5-step progress indicator for Switch Flow: `● ● ○ ○ ○` with step labels |
| **Manage Bindings** | List of bound providers with add/remove/edit actions |
| **Confirmation (Dry-run)** | Side-by-side diff with scrollable viewport |
| **Loading Overlay** | Centered spinner + contextual message, blocks all navigation except quit |
| **Toast Notifications** | Bottom-of-body colored bar: cyan for success, amber for warning, hot pink for error |

---

## Theme Architecture

All styles live in a single `aimuxTheme` struct (`internal/tui/theme.go`), initialized once at module load. Constants for hex values, struct for pre-built styles.

```go
var aimuxT = newAimuxTheme()

type aimuxTheme struct {
    Accent, AccentDim    lipgloss.Color
    TextPrimary, TextSecondary, TextMuted lipgloss.Color
    BgBase, Surface, Border lipgloss.Color
    Green, Red, Warn, Cyan lipgloss.Color

    // Pre-built styles
    Header, Title, Help, Inactive, ItemTitle, ItemDesc
    SelTitle, SelDesc, StatusOK, StatusWarn, StatusError, Muted
    Card, CardTitle
    FooterKey, FooterDesc, FooterSep
    StepDotDone, StepDotCurrent, StepDotFuture
    DiffAdded, DiffRemoved, DiffContext, DiffMuted, Viewport, DiffHeader
}
```

**No inline hex anywhere**. All components reference `aimuxT.*` fields. Changing the palette is a single-file edit.

### huh Form Theme

Forms use a custom `HuhTheme()` derived from `huh.ThemeBase()`:

- **Focused fields**: Left border in `AccentPink` (`#FF007F`)
- **Selectors**: `❯` prefix in pink for focused, empty space for blurred
- **Options**: `●` prefix in pink for selected, `○` in muted for unselected
- **Text input**: `❯` prompt in pink, cyan cursor
- **Buttons**: Pink background + white text for focused, muted for blurred
- **Group titles**: White bold

---

## Interaction Patterns

### Navigation (Dashboard)

- `↑/k` `↓/j`: Navigate menu items
- `Enter`: Select
- `?`: Toggle full help overlay
- `q` / `Ctrl+C`: Quit
- `Z`: Quick undo (restore last backup)

### Provider List

- `↑/↓`: Navigate providers
- `Enter`: Start Switch Flow with selected provider
- `a`: Add provider
- `e`: Edit provider
- `d`: Delete provider (with confirmation dialog)
- `r`: Retry model fetch
- `t`: Test connectivity
- `Esc`: Back to dashboard

### Forms

- `Tab` / `Shift+Tab`: Navigate between fields/groups
- `Enter`: Submit / advance to next group
- `Esc`: Abort and return to previous view (overrides huh's field-back behavior)
- `Space`: Toggle multi-select checkboxes

### Switch Flow (5-step)

- Stepper shows current position (1-5/5)
- `Enter`: Advance to next step / confirm
- `Esc`: Go back to previous step
- In Manage Bindings view: `a` to add, `d` to remove, `e` to edit

### Loading States

- Any async operation shows a spinner + contextual message
- All navigation input blocked except `q` / `Ctrl+C`
- On completion, spinner dismissed, result notification shown

### Notifications

- Appear as a colored bar at the bottom of the body
- Info (3s TTL), Warning (5s TTL), Error (persists until Esc)
- Error notifications also logged to `aimux.log`

---

## Keybinding Philosophy

- **Vim-friendly**: `k`/`j` alternates for navigation
- **Single-letter actions**: `a`dd, `e`dit, `d`elete, `r`etry, `t`est — mnemonic, no modifiers
- **Consistent Esc**: Esc always means "go back / cancel"
- **Consistent Enter**: Enter always means "confirm / advance"
- **Quick undo**: Capital `Z` — deliberate, hard to press accidentally

---

## Accessibility

- **Minimum terminal size**: 50×15. Below this, a clear message explains the requirement.
- **Color is not the only signal**: Status badges use text labels (ACTIVE/INACTIVE, OK/ERROR) alongside color.
- **Keyboard-only**: No mouse required. All actions have keyboard shortcuts.
- **High contrast**: White text on dark purple background exceeds WCAG AA contrast ratio.
