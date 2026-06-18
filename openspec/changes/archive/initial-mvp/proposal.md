# Proposal: aimux — AI Multiplexer CLI (Initial MVP)

## Problem Statement

Developers who work with multiple AI coding tools (Claude Code, Copilot, Codex, Pi) face a fragmented configuration landscape. Each tool reads its own set of environment variables, config files, and model preferences. Switching between tools requires manually editing shell configs, restarting terminals, or maintaining brittle shell scripts.

The core pain: there is no deterministic, zero-latency way to tell a developer's local AI stack "use this provider and these models for all tools, right now." Wrappers add latency; shell scripts break across environments; manual edits are error-prone.

**Why now**: AI dev tools are proliferating faster than configuration standards. Developers need a single control plane that works across tools without adding latency or shell-specific logic, and that gives them explicit (not auto-detected) control over which models each tool uses.

**Success looks like**: `aimux` (no args) shows a TUI dashboard. The user picks a provider-model mapping, applies it atomically, and every AI tool on their machine immediately targets those models. Total time: under 10 seconds.

---

## Scope

### In Scope (MVP)

1. **Interactive TUI Dashboard** — Full Bubble Tea loop with status table showing current multiplex state (active provider per CLI, available providers, available models). Action menu with Switch, Manage Providers, and Exit.
2. **Passive Provider Model Fetch** — User adds a provider (name, base URL, API key, auth token) via a huh form. On save, the tool GETs `/v1/models` from the provider and stores the raw model ID strings in SQLite. No auto-classification, no validation of model capabilities.
3. **Manual Variable Mapping** — For Claude Code specifically (the target CLI for MVP), the user binds raw model strings from available providers to Claude Code's env vars: `ANTHROPIC_DEFAULT_HAIKU_MODEL`, `ANTHROPIC_DEFAULT_SONNET_MODEL`, `ANTHROPIC_DEFAULT_OPUS_MODEL`, `CLAUDE_CODE_SUBAGENT_MODEL`. Implemented via `huh.NewSelect()` forms.
4. **Atomic Profile Switching** — User selects "Switch" from the menu, picks a target CLI and a saved profile. The tool opens the CLI's `settings.json`, parses it, injects the `"env"` block with the mapped variables, removes `ANTHROPIC_API_KEY` (security), preserves all non-env keys (global settings), and writes atomically (temp file + rename).
5. **Multi-Shell Path Resolution** — All paths use `os.UserHomeDir()` for runtime resolution. No shell scripts, no `$SHELL` detection. Config files must remain readable by the native CLI without preprocessing.

### Out of Scope (MVP)

- Auto-detection of installed CLIs (user must register target CLIs)
- Model capability classification or ranking (models are opaque strings)
- Provider health pings or latency metrics
- Multi-user or remote configuration
- GUI or web interface
- Shell script or alias generation
- CI/CD integration
- Audit logging beyond the SQLite state table
- Cross-machine sync
- Support for non-Claude-Code target CLIs in the mapping step (future: Copilot, Codex, Pi)

---

## Approach

### Architecture Overview

```
┌─────────────────────────────────────────────────┐
│                   aimux CLI                      │
│                                                  │
│  ┌─────────────────────────────────────┐        │
│  │        Bubble Tea TUI Loop          │        │
│  │  - StatusTable (Model/Provider state)│        │
│  │  - ActionMenu (Switch/Manage/Exit)   │        │
│  └──────────────┬──────────────────────┘        │
│                 │                                │
│  ┌──────────────▼──────────────────────┐        │
│  │          huh Form Layer             │        │
│  │  - Add Provider form               │        │
│  │  - Model Mapping Select             │        │
│  └──────────────┬──────────────────────┘        │
│                 │                                │
│  ┌──────────────▼──────────────────────┐        │
│  │        Business Logic Layer         │        │
│  │  - ProviderService (CRUD + fetch)   │        │
│  │  - MappingService (bind vars)       │        │
│  │  - SwitchService (atomic mutation)  │        │
│  │  - PathResolver (os.UserHomeDir)    │        │
│  └──────────────┬──────────────────────┘        │
│                 │                                │
│  ┌──────────────▼──────────────────────┐        │
│  │         Data Access Layer           │        │
│  │  - SQLite (matrix.db)              │        │
│  │  - JSON File I/O (settings.json)   │        │
│  └─────────────────────────────────────┘        │
└─────────────────────────────────────────────────┘
```

### Layering Rationale

Three-layer separation is chosen over a flat structure for these concrete reasons:

**TUI / Form layer**: Bubble Tea models dominate state management. Keeping them decoupled from business logic means the TUI can be replaced or tested in isolation. If we later add a non-interactive mode (flags only), the business layer is reusable unchanged.

**Business logic layer**: Provider fetching, mapping logic, and profile switching are independent concerns with distinct failure modes (HTTP errors during fetch vs. JSON parse errors during switch). Separating them prevents a single error handler from conflating unrelated issues.

**Data access layer**: SQLite and JSON file I/O have no cross-dependency. SQLite handles state; JSON handles mutation targets. Keeping them in one layer with explicit interfaces makes it possible to swap storage without touching business logic.

### Data Flow

**Profile Switch (the core operation):**
1. User selects "Switch" from dashboard menu
2. TUI layer prompts: pick a registered target CLI (from SQLite) and a profile (saved provider-model mapping)
3. Business layer calls PathResolver to get the absolute config file path
4. Business layer reads the JSON file, parses into a generic map
5. Removes `ANTHROPIC_API_KEY` from the root (security invariant)
6. Injects/overwrites the `"env"` key with the profile's mapped variables
7. Preserves all other root-level keys verbatim (global settings)
8. Writes to a temp file, then renames atomically over the original
9. Updates `active_multiplex` table in SQLite
10. TUI refreshes status table

**Provider Registration:**
1. User selects "Manage Providers" then "Add Provider"
2. huh form collects: name, base URL, API key, auth token
3. On save, business layer validates the URL format, stores in SQLite
4. Immediately performs GET `{base_url}/v1/models` (HTTP client with auth token)
5. On success: stores each model ID as a `provider_models` row (raw string, no transformation)
6. On failure: stores provider record with a flag ("fetch failed"), shows error in TUI
7. TUI shows updated provider list

---

## Database Schema

Following the PRD schema exactly, with the following implementation notes:

**`active_multiplex`**: This table uses `target_cli_id` as both PK and FK, meaning each CLI can have at most one active provider at a time. This enforces the design constraint that a CLI is always in exactly one multiplex state. When switching, the existing row is replaced (INSERT OR REPLACE).

**`provider_models`**: Models are stored as raw strings from the provider API. No normalization, no parsing of model capabilities. The `model_name` column is a VARCHAR that stores whatever the provider returns (e.g. `"claude-sonnet-4-20250514"`, `"gpt-4o"`, `"o3-mini"`).

**Auth tokens**: Both `api_key` and `auth_token` are stored in plaintext in SQLite. This is a local-only tool running on the developer's own machine with filesystem permissions controlling access (default `0600` on the DB file). No encryption at rest for MVP.

---

## Architecture Decisions

### AD-01: Direct Config Mutation Over Wrappers

**Status**: Accepted

**Context**: The tool could either wrap CLI invocations (inject env vars before each call) or directly modify the CLI's own config files.

**Decision**: Mutate config files directly. The wrapped approach adds latency on every invocation and breaks when the CLI updates its launch mechanism. Direct mutation means the CLI runs its native binary with zero overhead.

**Consequence**: We must parse JSON perfectly and preserve adjacent keys. A bad write corrupts the target CLI's config. Mitigation: atomic write (temp file + rename), and the PRD mandates preserving global settings.

### AD-02: SQLite Over Flat File State

**Status**: Accepted

**Context**: The state (providers, models, active mappings) could be stored in a JSON/YAML file in `~/.config/aimux/` instead of SQLite.

**Decision**: Use SQLite. The data has relational structure (providers -> models, many-to-many mappings). SQLite gives us atomic transactions, constraints (unique provider names, FK integrity on active_multiplex), and efficient querying for the dashboard status table.

**Consequence**: Dependencies on `modernc.org/sqlite` (pure Go, no CGO) are added. The DB file at `~/.config/aimux/matrix.db` is the single source of truth for multiplex state.

### AD-03: Bubble Tea Over BubbleZone or Tcell

**Status**: Accepted

**Context**: Go has several TUI frameworks. Bubble Tea is the most actively maintained, has the largest ecosystem, and integrates natively with huh (forms) and lipgloss (styling).

**Decision**: Use charmbracelet/bubbletea, matching the PRD. The combination of Tea + huh + lipgloss forms a cohesive toolkit that covers models, forms, and styling without mixing frameworks.

**Consequence**: The TUI is inherently single-window (no multi-pane async). The dashboard must be a single Bubble Tea model composing sub-models for the status table and the action menu.

### AD-04: Pure Go SQLite (modernc.org/sqlite)

**Status**: Accepted

**Context**: The standard mattn/go-sqlite3 requires CGO and a system-installed SQLite library. This complicates cross-compilation and distribution.

**Decision**: Use `modernc.org/sqlite` (pure Go implementation via `modernc.org/sqlite` or `pup.js.org/sqlite`). No CGO dependency, single static binary.

**Consequence**: Slightly larger binary size. Slightly slower query execution than CGO-bound SQLite. Acceptable for a CLI tool with small datasets (< 1000 rows).

### AD-05: Atomic Write via Temp File + Rename

**Status**: Accepted

**Context**: JSON config files must not be left in a half-written state. A crash during write could corrupt the target CLI's settings, breaking the developer's workflow.

**Decision**: Write to a temp file in the same directory, then `os.Rename()` over the target. This is atomic on POSIX systems (same filesystem). On macOS (the primary target), this is guaranteed atomic.

**Consequence**: Requires write permission on the target directory. The temp file is cleaned up on success (via the rename) or left as a `.tmp` artifact on failure (acceptable: no data loss).

---

## Risks and Mitigations

| Risk | Severity | Likelihood | Mitigation |
|------|----------|------------|------------|
| Corrupt target CLI config on failed write | Critical | Low | Atomic write via temp file + rename; parse before write with validation |
| Provider API is unreachable or slow | Medium | High | Timeout (5s default); store "fetch failed" flag; allow manual retry |
| SQLite DB corruption | Medium | Low | SQLite WAL mode; single-writer constraint (CLI tool, one user) |
| User adds wrong API key / invalid URL | Medium | Medium | Form validation on URL format; no validation on API key (not testable without hitting the API) |
| Bubble Tea model gets out of sync with DB state | Low | Low | Refresh from DB before rendering dashboard; no caching of state in the model |
| Go module dependency drift | Low | Medium | `go mod tidy` on build; pin minor versions in go.mod |
| Running on non-macOS (Linux target) | Low | Low | Test atomic rename behavior on Linux; os.Rename() is POSIX-compliant |
| settings.json has unexpected structure | Medium | Medium | Parse as `map[string]any`; inject `"env"` as a sub-map; do not assume key ordering or specific structure |

---

## Estimated Effort

| Module | Files | Estimated LOC | Complexity |
|--------|-------|---------------|------------|
| Project scaffold (go.mod, main.go, directory structure) | 3-4 | 50 | Low |
| SQLite schema + data access layer | 3-4 | 250 | Medium |
| Path resolver + config discovery | 1-2 | 80 | Low |
| Provider management (CRUD + fetch models) | 2-3 | 200 | Medium |
| Variable mapping forms (huh.Select chains) | 2-3 | 180 | Medium |
| Profile switching (JSON read/merge/write) | 2-3 | 250 | High |
| TUI dashboard (Bubble Tea main loop + table) | 3-4 | 350 | High |
| Error handling + edge cases | 1-2 | 100 | Medium |
| **Total** | **17-25** | **~1460** | — |

---

## Rollback Plan

Since this is an initial MVP, rollback means removing the tool entirely:

1. **Uninstall binary**: `rm $(which aimux)` or remove from Go install path
2. **Remove state**: `rm -rf ~/.config/aimux/` (deletes matrix.db and any cached state)
3. **Reverse config changes**: If settings.json was mutated, the atomically written file preserves the original structure with the `"env"` block injected. To fully restore pre-aimux state, the user must remove the `"env"` block and restore `ANTHROPIC_API_KEY` from their backup or shell config.

For the conmutator (profile switch), the system does not create backups before mutation. Mitigation: the atomic write means the file is always in a valid state, but the old `"env"` block is overwritten. If the user switches back to a previous profile, the profile is re-read from SQLite and re-injected. No data loss within the tool's domain.

---

## Proposal Question Round

The following questions would benefit from PRD-level clarification before proceeding to specs/design. They should be resolved before or during the spec phase:

1. **Scope of "target CLIs"**: The MVP maps only Claude Code variables. Should the SQLite schema and TUI support registering other CLIs (Copilot, Codex, Pi) with their own env vars, even if mapping is deferred? Or should the MVP be strict about only Claude Code? This affects the schema design for `target_clis` and the mapping forms.

2. **Profile switching UX**: When the user "switches" to a provider, should they switch ALL four model variables at once (from one profile), or should they be able to mix-and-match (e.g. Sonnet from Provider A, Haiku from Provider B)? The PRD implies profile-based, which is simpler. But mixing is a natural user need.

3. **Provider fetch failures**: If `/v1/models` returns an error (timeout, 401, 500), should the provider record still be saved so the user can retry later? Or should the entire registration fail? The PRD says "passive fetch" but doesn't specify failure behavior.

4. **Startup state**: What should the dashboard show when `matrix.db` is empty (first run)? A welcome message? A quick-start guide? An empty table with "Add Provider" as the only action? This affects the Bubble Tea initial model state.

5. **Concurrency and locking**: Since aimux mutates config files that other processes (the AI tools themselves) also read/write, is there a risk of concurrent modification? Should aimux use file locking (flock) before reading/writing `settings.json`?

---

## Assumptions (Requiring User Review)

- **Single-user, single-machine**: No multi-user, no remote config sync
- **Claude Code primary target**: MVP mapping forms are hardcoded for Claude Code's four env vars
- **No auto-detection**: Users register providers and target CLIs manually
- **Plaintext secrets**: API keys and auth tokens stored in plaintext in SQLite
- **POSIX atomic rename**: os.Rename() provides sufficient atomicity for config files
- **JSON-based target CLIs**: Target CLIs use JSON config files (settings.json). YAML/TOML support is future work
- **No backup before mutation**: The user is responsible for their own config backups
