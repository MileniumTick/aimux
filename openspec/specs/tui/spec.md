# TUI — Dashboard and Forms Spec

## Scope

All interactive screens in the aimux CLI: the main Bubble Tea dashboard loop, the action menu, and all huh forms (Add Provider, Model Mapping, Switch Profile). No non-interactive (flag-only) mode in MVP.

## Dashboard

### Main Loop

- The CLI MUST start a Bubble Tea program when invoked with no arguments: `aimux`.
- The program MUST NOT exit until the user selects "Exit" from the action menu or sends SIGINT (Ctrl+C).
- On startup, the TUI MUST query `ListActiveMultiplexes()` and `ListProviders()` to populate the status table.
- The TUI MUST refresh from the database before every render cycle (no local caching of state in the model).

### Status Table (First Default View)

- MUST render a table with columns: **CLI** | **Provider** | **Models** | **Status**
- **CLI**: The name from `target_clis` (e.g., `claude-code`)
- **Provider**: The provider name from the active multiplex, or `---` if none
- **Models**: A comma-separated list of the four mapped model IDs, or `---` if none
- **Status**: `ACTIVE` if an active multiplex exists, `INACTIVE` if not
- The table MUST be rendered using `lipgloss` styling with alternating row colors and a header row.
- The table MUST handle terminal resize events gracefully (reflow columns proportionally).

### Empty State (First Run)

Given the `providers` table is empty (no providers registered)  
When the dashboard loads  
Then the status table MUST show the single `claude-code` CLI row with `---` in all data columns and `INACTIVE` status  
And a hint line MUST be displayed beneath the table: "No providers configured. Use 'Manage Providers' to add one."  
And the action menu MUST offer "Manage Providers" as the default (highlighted) option

### Action Menu

- MUST appear as a footer or sidebar menu below the status table.
- Menu items MUST be rendered as a vertical list with keyboard navigation (Up/Down arrows, Enter to select).
- Menu items:
  1. **Switch** — Navigate to the Switch Profile flow
  2. **Manage Providers** — Navigate to the Manage Providers flow
  3. **Exit** — Quit the Bubble Tea program

### Keyboard Model (Global)

| Key | Action |
|-----|--------|
| Up / k | Move selection up in the current menu or list |
| Down / j | Move selection down in the current menu or list |
| Enter | Confirm the current selection or form submission |
| Esc | Go back to the previous screen (dashboard from forms) |
| Ctrl+C | Immediate program exit (Bubble Tea default) |
| q | Alternative exit from dashboard (SHOULD work as Ctrl+C does at dashboard level) |

## "Manage Providers" Flow

### Provider List Screen

- Shows a table of all registered providers with columns: **Name** | **Base URL** | **Models Fetched** | **Status**
- **Status** column shows `OK` (status = active, models exist) or `ERROR` (status = error)
- Shows a footer hint: "Enter = Manage | a = Add | Esc = Back"
- Navigation: Enter on a provider row opens a sub-action: "Delete Provider" or "Retry Fetch" (if status = error)
- Keyboard: `a` to add a new provider, `d` to delete the selected provider (with confirmation), `r` to retry fetch for error-status providers, Esc to return to dashboard

### Add Provider Form (huh)

- Form fields (in order):
  1. **Name** — Text input, required, validated non-empty
  2. **Base URL** — Text input, required, validated as parseable URL (scheme + host required)
  3. **API Key** — Text input, masked (`huh.NewInput().EchoMode(EchoModePassword)`), required, validated non-empty
  4. **Auth Token** — Text input, masked, required, validated non-empty
- On submit:
  1. Call `AddProvider()` to persist to SQLite
  2. Call `fetchProviderModels(providerID, baseURL, authToken)` (HTTP GET)
  3. On fetch success: call `InsertModels()` with returned IDs, show brief success toast/notification
  4. On fetch failure: call `UpdateProviderStatus(id, "error")`, show error notification in TUI
  5. Return to Provider List Screen with updated table

### Delete Provider Confirmation

- SHOW a short confirm dialog: "Delete [provider name]? This will remove all associated models and active mappings. [Y/n]"
- On confirm: call `DeleteProvider()`, return to Provider List

## "Switch Profile" Flow

### Target CLI Selection

- Shows a list of registered target CLIs (from `target_clis` table).
- For MVP, only `claude-code` exists.
- User selects one CLI with Enter.

### Model Mapping Form

- After selecting a CLI, show a provider selection: list of all providers with `status = 'active'`.
- User picks one provider.
- Then show a series of `huh.Select()` forms, one per env var in the CLI's `env_vars` array:
  1. **ANTHROPIC_DEFAULT_HAIKU_MODEL** — Select from provider's available models
  2. **ANTHROPIC_DEFAULT_SONNET_MODEL** — Select from provider's available models
  3. **ANTHROPIC_DEFAULT_OPUS_MODEL** — Select from provider's available models
  4. **CLAUDE_CODE_SUBAGENT_MODEL** — Select from provider's available models
- Each form SHALL offer "Go Back / Select Later" as a NOT SELECTED option (value = empty string).
- On submit:
  1. Build model mappings JSON: `{"ANTHROPIC_DEFAULT_HAIKU_MODEL": "selected-id", ...}`
  2. Call `SetActiveMultiplex()`
  3. Briefly render a confirmation: "Profile activated for claude-code: Provider X with N models mapped"
  4. Return to dashboard (status table refreshes)

## Acceptance Scenarios

### Dashboard — Empty State

Given no providers exist in the database  
When `aimux` is run  
Then the dashboard shows `claude-code` with INACTIVE status  
And a hint is displayed beneath the table  
And the action menu has "Manage Providers" selected by default

### Add Provider — Success Path

Given the provider list screen is shown  
When the user presses `a` and fills the form with valid data  
And the `/v1/models` fetch succeeds  
Then the provider appears in the list with Status = OK  
And the dashboard returns to the provider list screen

### Add Provider — Fetch Failure

Given the provider list screen is shown  
When the user fills the form with valid data  
And the `/v1/models` fetch fails (timeout or HTTP error)  
Then the provider appears in the list with Status = ERROR  
And an error notification is shown briefly in the TUI  
And the "Retry Fetch" action becomes available for that provider

### Switch Profile — Full Mapping

Given a provider with models exists and is active  
When the user selects "Switch" from the dashboard menu  
And selects the `claude-code` CLI  
And selects the provider  
And picks a model for each of the four env vars  
Then the dashboard returns to the main view  
And the status table shows the provider name and model mappings for `claude-code`  
And the status column shows ACTIVE

### Exit

Given the dashboard is showing  
When the user selects "Exit" from the action menu or presses Ctrl+C or q  
Then the Bubble Tea program terminates gracefully (exit code 0)
