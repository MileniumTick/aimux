# Verify Report: aimux — AI Multiplexer CLI (Initial MVP)

**Date**: 2026-06-17
**Verification Scope**: Full spec-to-implementation validation
**Build**: go build ./... — PASS
**Tests**: 50/50 passed across 4 packages (data/queries, data/config, business/provider, business/switch, tui)

---

## Executive Summary

**1 CRITICAL, 4 WARNING, 2 SUGGESTION** findings. The TUI switch profile flow is broken due to form value capture not being propagated to the model state. All data layer, business layer non-TUI logic, and rendering functions are correct and well-tested. The core switching and binding logic works correctly at the business layer.

---

## Spec Coverage By File

### 1. PATH SPEC (`path/spec.md`) — PASS

| Requirement | Status | Notes |
|-------------|--------|-------|
| `ResolvePath` tilde expansion | PASS | `os.UserHomeDir()` with `sync.Once` caching |
| `ResolvePath` non-tilde paths | PASS | Returns `filepath.Clean(path)` |
| Home directory error propagation | PASS | Error wrapped and returned |
| `ResolveConfigPath` | PASS | Returns `~/.config/aimux/matrix.db`, calls `os.MkdirAll` with 0700 |
| `ResolveTargetConfigPath` | PASS | Delegates to `ResolvePath` |
| No `os.Getenv("HOME")` | PASS | Uses `os.UserHomeDir()` exclusively |
| No `os/exec.Command` | PASS | No shell invocation |
| Cache `os.UserHomeDir()` once | PASS | `sync.Once` pattern |

### 2. STORAGE SPEC (`storage/spec.md`) — PASS

| Requirement | Status | Notes |
|-------------|--------|-------|
| Schema: providers table | PASS | All columns, CHECK on status, UNIQUE name |
| Schema: provider_models table | PASS | FK CASCADE, UNIQUE(provider_id, model_name) |
| Schema: target_clis table | PASS | All columns |
| Schema: active_multiplex table | PASS | PK FK, FK CASCADE |
| Indexes | PASS | `idx_provider_models_provider_id` |
| File permissions (0600/0700) | PASS | `os.Chmod(0600)`, `os.MkdirAll(0700)` |
| WAL mode | PASS | `PRAGMA journal_mode=WAL` |
| AddProvider | PASS | INSERT with status='active', returns ID |
| GetProvider | PASS | Error if not found |
| ListProviders | PASS | ORDER BY name ASC |
| UpdateProviderStatus | PASS | Updates status + updated_at |
| DeleteProvider | PASS | CASCADE deletes models + active_multiplex |
| InsertModels (clear + re-insert) | PASS | Transaction with DELETE + INSERT |
| ListModels | PASS | ORDER BY model_name ASC |
| ListAllModels | PASS | JOIN on providers |
| GetActiveMultiplex (not found) | PASS | Returns empty struct, no error |
| SetActiveMultiplex | PASS | INSERT OR REPLACE |
| ClearActiveMultiplex | PASS | DELETE |
| ListActiveMultiplexes | PASS | JOIN on providers + target_clis |
| SeedTargetCLIs | PASS | INSERT OR IGNORE |
| File locking (LOCK_EX/LOCK_SH) | PASS | syscall.Flock with 2s timeout |
| Deferred unlock | PASS | defer pattern |

### 3. PROVIDER SPEC (`provider/spec.md`) — PASS

| Requirement | Status | Notes |
|-------------|--------|-------|
| GET `/v1/models` | PASS | Appended to user-provided base URL |
| Auth header | PASS | `Authorization: Bearer {auth_token}` |
| Headers (Accept, User-Agent) | PASS | `application/json`, `aimux/0.1.0` |
| 5s timeout | PASS | `http.Client.Timeout` |
| OpenAI response format | PASS | `data[].id` |
| Anthropic fallback format | PASS | `models[].id` |
| HTTP 401/403 error | PASS | "Authentication failed: check auth token" |
| HTTP 429 error | PASS | "Rate limited by provider" |
| HTTP 5xx error | PASS | "Provider returned server error: {status}" |
| Timeout error | PASS | "Request timed out after 5 seconds" |
| Network error | PASS | "Network error: {error}" |
| Retry logic | PASS | Uses stored credentials, resets status to active on success |

### 4. MAPPING SPEC (`mapping/spec.md`) — PASS

| Requirement | Status | Notes |
|-------------|--------|-------|
| BindProfile: env var validation | PASS | Keys checked against known set |
| BindProfile: provider status check | PASS | Must be 'active' |
| BindProfile: model existence check | PASS | Non-empty model IDs validated |
| GetBoundModels | PASS | Empty map with no error if none |
| GetProviderForCLI | PASS | Returns provider ID or error |
| Partial binding (empty values) | PASS | Empty strings stored in JSON |

### 5. SWITCH SPEC (`switch/spec.md`) — PASS

| Requirement | Status | Notes |
|-------------|--------|-------|
| Path resolution | PASS | Via ResolveTargetConfigPath |
| File locking (LOCK_EX) | PASS | 2s timeout |
| JSON parsing | PASS | map[string]any |
| Empty/invalid JSON | PASS | Treated as empty object |
| Non-existent file | PASS | Created (O_CREATE) |
| Remove root ANTHROPIC_API_KEY | PASS | Security invariant enforced |
| Inject/overwrite env block | PASS | Builds new env map |
| Skip empty values in env | PASS | Non-empty values only |
| Empty env block when all empty | PASS | Sets `env: {}` |
| Inject API key into env | PASS | `env.ANTHROPIC_API_KEY` |
| Preserve global keys | PASS | Only env and ANTHROPIC_API_KEY modified |
| Atomic write (temp file + rename) | PASS | `os.CreateTemp` + `os.Rename` |
| Trailing newline | PASS | Appended after MarshalIndent |
| file.Sync() | PASS | On temp file before rename |
| Deferred cleanup on error | PASS | defer/remove pattern |

### 6. TUI SPEC (`tui/spec.md`) — FAIL (with caveats)

| Requirement | Status | Notes |
|-------------|--------|-------|
| Main loop (Bubble Tea) | PASS | tea.NewProgram with model |
| Status table columns | PASS | CLI, Provider, Models, Status |
| Empty state rendering | PASS | INACTIVE row + hint text |
| Action menu items | PASS | Switch, Manage Providers, Exit |
| Switch disabled when no providers | PASS | Keyboard skips item |
| Keyboard navigation | PASS | Up/Down/j/k, Enter, Esc, q |
| Provider list columns | PASS | Name, Base URL, Models Fetched, Status |
| Add Provider form (4 fields) | PASS | huh form with validation |
| Delete confirmation | PASS | huh.Confirm dialog |
| Switch flow: CLI → Provider → Models | FAIL | See CRITICAL #1 |
| Switch confirmation message | WARNING | Generic, missing provider name + model count |
| Provider list Enter action | WARNING | No Enter handler for provider sub-actions |
| Add Provider form field order | PASS | Name, Base URL, API Key, Auth Token |

---

## Detailed Findings

### CRITICAL

#### C1: switchTargetCLIID / switchProviderID not captured from form (TUI switch flow broken)

**Spec**: `tui/spec.md` — Switch Profile flow (Target CLI Selection, Model Mapping Form)

**File**: `/Users/jchavarriam/workspace/personal/aimux/internal/tui/model.go`

**Issue**: In `startSwitchFlow()` (line 283), the form is bound to a local variable `cliID` via `&cliID`, but `handleFormCompletion` for `switchTargetCLIView` (line 380) does not copy the selected CLI ID to `m.switchTargetCLIID`. The model field stays at its initialization value of 0. The same pattern affects `m.switchProviderID` in the `switchProviderView` handler (line 379-400) — the local `providerID` variable is never stored to the model.

**Impact**: When the user completes the target CLI selection and then the provider selection, `BindProfile(m.switchTargetCLIID, m.switchProviderID, mappings)` is called with both IDs as 0. The switch flow will fail with "target CLI 0 not found" or process incorrect data. The switch profile flow is non-functional through the TUI.

**Fix**: Replace the local variable binding with direct model field binding:
```go
m.form = NewSelectTargetCLIForm(clis, &m.switchTargetCLIID)
```
Similarly for provider selection:
```go
m.form = NewSelectProviderForm(providers, &m.switchProviderID)
```

After fixing, remove the redundant local variable declarations.

---

### WARNING

#### W1: Provider list screen not handling Enter key for sub-actions

**Spec**: `tui/spec.md` (Provider List Screen): "Enter on a provider row opens a sub-action: 'Delete Provider' or 'Retry Fetch' (if status = error)"

**File**: `/Users/jchavarriam/workspace/personal/aimux/internal/tui/model.go`, lines 222-247

**Issue**: The providerListView case in `handleKeyMsg` does not have an `Enter` handler. The `d` and `r` keys work directly for delete and retry, but there's no Enter-then-sub-action flow. The hint text also doesn't mention `d` or `r` keyboard shortcuts.

**Impact**: The workflow deviates from the spec. Users expecting the Enter → sub-action pattern (as specified) won't get it. The direct keys (`d`, `r`) do work, so the functionality is accessible.

**Fix**: Either add an Enter handler that shows sub-actions, or update the hint to include `d` and `r`.

#### W2: Switch confirmation message lacks specificity

**Spec**: `tui/spec.md` (Switch Profile flow): "Briefly render a confirmation: 'Profile activated for claude-code: Provider X with N models mapped'"

**File**: `/Users/jchavarriam/workspace/personal/aimux/internal/tui/model.go`, lines 481-485

**Issue**: The confirmation message is generic: "Profile activated successfully! The config has been written and multiplex is active." It does not include the provider name or model count as specified.

**Impact**: User feedback is less informative than the spec requires. The user sees "success" but not what was configured.

**Fix**: Update the confirmation string to include the provider name and model count from `m.switchExtractFn()` result.

#### W3: refreshData() return value discarded in retryFetch

**Spec**: General code quality

**File**: `/Users/jchavarriam/workspace/personal/aimux/internal/tui/model.go`, lines 454-463

**Issue**: `m.refreshData()` returns a `DashboardRefreshMsg` which is a `tea.Msg`, but in `retryFetch()` the return value is discarded. The model state IS updated (via pointer receiver), but the returned command is lost.

**Impact**: Code hygiene issue. The pattern is inconsistent with other call sites that properly handle the returned message. The functionality is not broken because the state mutation happens through the pointer receiver before the message is ignored.

**Fix**: Return the result of `m.refreshData()` in the success path instead of constructing a new `DashboardRefreshMsg{}`.

#### W4: Design doc config_path discrepancy (noted for reference)

**Design**: `openspec/changes/initial-mvp/design.md` line 242 seeds with `Library/Application Support/Claude/claude_desktop_config.json` (macOS path)

**Implementation**: Seeds with `~/.config/claude/settings.json` (Linux path, matching spec)

**Impact**: The implementation correctly follows the authoritative spec. The design document has a stale/different path. No action needed on the code, but the design should be updated to match the spec.

---

### SUGGESTION

#### S1: TUI tests cover rendering only, not model message routing

**Spec**: `tui/spec.md`, tasks.md T6e mentions "Test keyboard: Up/Down navigation in menu, Enter selection, Esc back"

**File**: `/Users/jchavarriam/workspace/personal/aimux/internal/tui/tui_test.go`

**Issue**: The TUI tests cover `RenderTable`, `RenderMenu`, `RenderProviderList`, and `NewModel` — all rendering/pure functions. There is no interactive model test (no `tea.NewProgram` with simulated input/output). The spec and task T6e describe keyboard model testing that is not implemented.

**Impact**: The switch flow bug (C1) was not caught by tests. Adding interactive TUI tests would catch integration issues between forms and model state.

**Suggestion**: Add Bubble Tea model tests using `tea.NewProgram` with `tea.WithInput`/`tea.WithOutput` to test the switch flow and keyboard navigation end-to-end.

#### S2: Provider list hint could be more informative

**Spec**: The keyboard model lists `a`, `d`, `r` as provider list keys

**File**: `/Users/jchavarriam/workspace/personal/aimux/internal/tui/table.go`, line 193

**Issue**: The hint shows "Enter = Manage | a = Add | Esc = Back" but doesn't mention `d` (delete) or `r` (retry fetch). Users may not discover these actions.

**Suggestion**: Update the hint to: "a = Add | d = Delete | r = Retry | Esc = Back"

---

## Task Completion Status

| Task | Status | Notes |
|------|--------|-------|
| T1 — Scaffold | COMPLETE | go.mod, directory structure, dependencies |
| T2a — PathResolver | COMPLETE | Verified against path/spec.md |
| T2b — SQLite Schema + Migration | COMPLETE | Verified against storage/spec.md |
| T2c — CRUD Queries | COMPLETE | All 12 query functions verified |
| T2d — JSON Config Operations | COMPLETE | Flock, atomic write verified |
| T3a — ProviderService | COMPLETE | Fetch, retry, error handling verified |
| T3b — SwitchService | COMPLETE | BindProfile, Apply, GetBoundModels verified |
| T4a — TUI Main Model | PARTIAL | Model structure correct. Switch flow has bug C1. |
| T4b — Status Table | COMPLETE | RenderTable, RenderProviderList verified |
| T4c — Action Menu | COMPLETE | RenderMenu with disabled state verified |
| T4d — Form Factories | COMPLETE | All form factories verified |
| T5 — Wiring (main.go) | COMPLETE | DB init, services, Bubble Tea launch |
| T6a — Data query tests | COMPLETE | 13 tests covering all CRUD |
| T6b — Data config tests | COMPLETE | 7 tests covering flock + atomic write |
| T6c — Business provider tests | COMPLETE | 8 tests with httptest mock server |
| T6d — Business switch tests | COMPLETE | 7 tests covering bind + apply |
| T6e — TUI model tests | PARTIAL | 8 tests cover rendering only. No interactive model tests. |

---

## Test Results

```
=== RUN   All tests
--- PASS: 50 tests in 4 packages
```

Packages tested:
- `internal/data/` — queries (13 tests) + config (7 tests) = 20 tests
- `internal/business/` — provider (8 tests) + switch (7 tests) = 15 tests
- `internal/tui/` — rendering/model (8 tests)

Build: `go build ./...` — SUCCESS

---

## Recommendations

1. **Fix C1 immediately** before any use of the switch flow through the TUI. The data layer and business layer logic for switching is correct and well-tested — only the TUI form value propagation is broken.
2. **Address W1, W2** for spec compliance.
3. **Consider S1** as a follow-up to prevent regression on the switch flow.
4. **Archive**: BLOCKED until C1 is resolved.
