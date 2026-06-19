# aimux — AI Provider Multiplexer

> Route multiple AI dev CLIs (Claude Code, OpenCode, Codex, Copilot, pi) through your own providers with a single TUI. Multi-provider, model selection, self-update, and centralized backups.

---

## Table of Contents

- [What is aimux?](#what-is-aimux)
- [Architecture](#architecture)
- [Installation](#installation)
- [Quick Start (TUI)](#quick-start-tui)
- [Quick Start (CLI)](#quick-start-cli)
- [User Manual](#user-manual)
  - [Dashboard](#dashboard)
  - [Providers (Añadir/Editar/Eliminar)](#providers)
  - [Switch Flow](#switch-flow)
  - [Multi-Provider](#multi-provider)
  - [CLI Management](#cli-management)
  - [Restore Backup](#restore-backup-from-tui)
- [Example: Claude Code + Bifrost (Anthropic)](#example-claude-code--bifrost-anthropic)
- [Example: Copilot + Local LLM](#example-copilot--local-llm)
- [Backup System](#backup-system)
- [CLI Reference](#cli-reference)
- [Development](#development)
- [FAQ](#faq)
- [User Manual (Español)](docs/manual-de-usuario.md)
- [Full Documentation](#full-documentation)

---

## What is aimux?

**aimux** is a **single-binary TUI + CLI tool** that lets you centralize AI provider credentials and switch between providers for your dev CLIs.

Instead of editing each CLI's config file manually (`~/.claude/settings.json`, `~/.codex/config.toml`, etc.), aimux:

1. Stores provider credentials (API key, auth token, base URL) in a local SQLite DB
2. Discovers available models from each provider's API
3. Lets you bind a provider to a CLI with specific model mappings
4. Mutates the CLI's config file automatically
5. Makes centralized backups before every mutation

### Supported CLIs

| CLI | Config File | Mutator | Multi-Provider |
|-----|------------|---------|---------------|
| **Claude Code** | `~/.config/claude/settings.json` | `claude-settings-json` | ❌ |
| **OpenCode** | `~/.config/opencode/config.json` | `opencode-provider-json` | ✅ |
| **Codex** | `~/.codex/config.toml` | `codex-config-toml` | ❌ |
| **GitHub Copilot** | Shell profile (`~/.zshrc`, `~/.bashrc`, `~/.config/fish/config.fish`) | `copilot-shell-profile` | ✅ |
| **pi** | `~/.pi/agent/models.json` | `pi-dual-json` | ✅ |

### Supported Provider Types

| Type | Authentication | Model Discovery |
|------|---------------|-----------------|
| **OpenAI / OpenAI-compatible** | Bearer token | `GET /v1/models` |
| **Anthropic** | `x-api-key` header | `GET /v1/models` |
| **Google AI (Gemini)** | API key query param | `GET /v1beta/models` |

### Discovery URL (optional)

Each provider can have a **Discovery URL** separate from its Base URL. Useful when `/v1/models` lives at a different address than the chat endpoint. Leave empty to use Base URL for both.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                         aimux                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │                   TUI (Bubbletea)                    │   │
│  │  ┌──────────┐  ┌──────────┐  ┌───────────────────┐  │   │
│  │  │ Dashboard│  │Provider  │  │ Switch Flow       │  │   │
│  │  │ (Table)  │  │Mgmt     │  │ CLI→Provider→Model │  │   │
│  │  └──────────┘  └──────────┘  └───────────────────┘  │   │
│  └──────────────────────────────────────────────────────┘   │
│  ┌──────────────────────────────────────────────────────┐   │
│  │               CLI (os.Args)                          │   │
│  │  apply · list · backups · restore · version · update │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │            Application Layer (use cases)              │   │
│  │  ┌──────────────┐  ┌─────────────────────────────┐   │   │
│  │  │  ProviderUC  │  │    SwitchUseCases           │   │   │
│  │  │  • CRUD      │  │  • Apply (mutate config)    │   │   │
│  │  │  • Fetch     │  │  • BindProfile              │   │   │
│  │  │  • Retry     │  │  • DryRun                   │   │   │
│  │  │  • Test      │  │  • Backups / Restore        │   │   │
│  │  └──────────────┘  └─────────────────────────────┘   │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │            Domain Layer (interfaces + models)         │   │
│  │  Provider · ProviderModel · TargetCLI · ActiveMultiplex  │   │
│  │  ProviderRepository · TargetCLIRepository · MultiplexRepo │   │
│  │  ConfigMutator interface                            │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                             │
│  ┌──────────────────────────────────────────────────────┐   │
│  │           Infrastructure Layer                        │   │
│  │  ┌─────────┐ ┌──────────┐ ┌─────────┐ ┌──────────┐  │   │
│  │  │ SQLite  │ │Mutators  │ │Config   │ │ Update   │  │   │
│  │  │ (repo)  │ │(5 impls) │ │(JSON)   │ │(self)    │  │   │
│  │  └─────────┘ └──────────┘ └─────────┘ └──────────┘  │   │
│  └──────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### DDD Layers

- **Domain** (`internal/domain/`): interfaces + value objects — no dependencies
- **Application** (`internal/application/`): use cases, orchestration logic
- **Infrastructure** (`internal/infrastructure/`): SQLite repos, config mutators, update logic
- **TUI** (`internal/tui/`): Bubbletea views (dashboard, forms, tables)
- **CLI** (`main.go`): command routing, help text

### Data Flow

```
User (TUI) → ProviderUseCases → SQLiteProviderRepo → SQLite DB
                                       ↓
                              HTTP GET /v1/models
                                       ↓
                              Parse response → InsertModels
                                       ↓
User (TUI) → SwitchUseCases.BindProfile() → multiplex table
                                       ↓
User (TUI) → SwitchUseCases.Apply() → ConfigMutator.Mutate()
                                       ↓
                              Back up config → Write mutation
                                       ↓
                              Prune old backups
```

---

## Installation

### From source

```bash
git clone https://github.com/MileniumTick/aimux.git
cd aimux
go build -o aimux .
sudo mv aimux /usr/local/bin/
```

### From Go install

```bash
go install github.com/MileniumTick/aimux@latest
```

### Verify

```bash
aimux version
# → aimux 0.2.0
```

---

## Quick Start (TUI)

```bash
# Launch the TUI (no arguments)
aimux
```

You'll see the **dashboard** with your pre-configured CLIs and any active providers.

```
  aimux
  ┌──────────┬──────────┬──────────────────────────────┬────────┐
  │ CLI      │ Provider │ Models                       │ Status │
  ├──────────┼──────────┼──────────────────────────────┼────────┤
  │ claude-  │ ---      │ ---                          │INACTIVE│
  │ code     │          │                              │        │
  │ codex    │ ---      │ ---                          │INACTIVE│
  │ github-  │ ---      │ ---                          │INACTIVE│
  │ copilot  │          │                              │        │
  │ opencode │ ---      │ ---                          │INACTIVE│
  │ pi-ai    │ ---      │ ---                          │INACTIVE│
  └──────────┴──────────┴──────────────────────────────┴────────┘

      Switch
      Manage Providers
      Manage CLIs
      Exit
```

1. Select **Manage Providers** and press Enter
2. Press **a** to add a provider
3. Fill in: Name, Base URL, Discovery URL (optional), API Key, Auth Token, API Type
4. Back in the provider list, select your provider and press **Enter**
5. Follow the **Switch flow** (5 steps): CLI → Provider → Map/Select Models → Advanced Review → Confirm
6. Review the dry-run diff and press Enter to apply
7. Done! Your CLI is now configured to use your provider

---

## Quick Start (CLI)

```bash
# See active multiplexes
aimux list

# Apply (re-apply) active provider bindings for a CLI
aimux apply claude-code

# List centralized backups
aimux backups claude-code
# → Backups for 'claude-code' (newest first):
#   [0] 2026-06-18T03-21-00Z
#   [1] 2026-06-18T02-15-00Z

# Restore the latest backup
aimux restore claude-code
# → Restored latest backup: /Users/you/.config/aimux/backups/settings.json-abc123def4/settings.json.2026-06-18T03-21-00Z

# Check version and auto-update
aimux version

# Self-update binary
aimux update
```

---

## User Manual

### Dashboard

The dashboard is the main view when you launch `aimux` without arguments. It shows:

| Section | Description |
|---------|-------------|
| **Summary** | Numeric overview: active/errored providers, active/inactive CLIs |
| **Welcome** | Shown on first run with no providers — guides you to add one |
| **Menu** | Choose between Switch, Manage Providers, Manage CLIs, Restore Backup, and Exit |
| **Notifications** | Green/red bar at the bottom for success/error messages |

**Keys:** `↑/↓` or `k/j` to navigate menu · `Enter` to select · `q` to quit · `?` for help overlay · `Z` to undo last apply.

### Providers

The **Provider List** shows all configured providers with their name, base URL, model status, and health status.

**Keys within provider list:**

| Key | Action |
|-----|--------|
| `↑/↓` | Navigate providers |
| `a` | **Add** a new provider |
| `e` | **Edit** the selected provider |
| `d` | **Delete** the selected provider |
| `r` | **Retry** model fetch |
| `t` | **Test** connectivity |
| `Enter` | Start **Switch flow** with this provider |
| `Esc` | Back to dashboard |

#### Adding a Provider

The **Add Provider** form asks for:

1. **Name** — a friendly identifier (e.g. "Bifrost", "My OpenAI", "Bifrost Anthropic")
2. **Base URL** — full URL including scheme (e.g. `https://api.openai.com/v1`, `https://ai.intranet.istmocenter.com`)
3. **API Key** — shown as password input
4. **Auth Token** — optional if same as API Key
5. **Discovery URL** — optional, for model discovery. Leave empty to reuse Base URL.
6. **API Type** — `OpenAI`, `Anthropic`, or `Google AI (Gemini)`

After submitting, aimux immediately fetches available models from `GET /v1/models` and populates the model list.

#### Editing a Provider

The Edit form is pre-filled with the current values. Name is read-only. You can update Base URL, API Key, Auth Token, and API Type. Models are re-fetched after save.

### Switch Flow

The **Switch Flow** binds a provider to a CLI. It has **5 steps** shown in a stepper:

```
Step 1/5: Select Target CLI
  ● ◉ ○ ○ ○

Step 2/5: Select Provider

Step 3/5: Map / Select Models (varies per CLI)

Step 4/5: Advanced Model Configuration Review

Step 5/5: Confirm & Apply (Dry-run with diff view)
```

#### Step 1: Select CLI

Type to filter. The CLI type determines the model UI:

| CLI | Model UI | Multi-provider?
|-----|----------|---------------
| **claude-code** | Per-env-var mapping | ❌
| **codex** | Per-env-var mapping | ❌
| **opencode** | Multi-select (checkboxes) | ✅
| **pi-ai** | Multi-select (checkboxes) | ✅
| **github-copilot** | Single-model select | ✅

#### Step 2: Select Provider

Filter by typing. Providers with errors show `[ERROR]`.

#### Step 3: Map / Select Models

**For Claude Code / Codex (env-var mapping):**
Each env var gets its own selector. Use "(Apply to all)" on fields 2+.

```
ANTHROPIC_DEFAULT_HAIKU_MODEL  → deepseek-v4-flash
ANTHROPIC_DEFAULT_SONNET_MODEL → deepseek-v4-pro
ANTHROPIC_DEFAULT_OPUS_MODEL   → (Not Selected)
```

**For pi / OpenCode (multi-select):**
Toggle models with Space. All pre-selected by default.

**For Copilot (single-select):**
Pick one model for `COPILOT_MODEL`.

#### Step 4: Advanced Config Review

Shows metadata per model (context window, max tokens, reasoning, cost, context suffix).

#### Step 5: Dry-run & Apply

Side-by-side diff shows current config vs new env vars. Press **Enter** to apply.

After apply, press **Z** on the dashboard for instant undo (restores latest backup).

### Multi-Provider

CLIs that support multi-provider (OpenCode, pi, Copilot) show a **Manage Bindings** view when the chosen CLI already has active bindings. From here:

- **`a`** — Add another provider
- **`d`** — Remove the selected binding
- **`e`** — Edit which models are mapped
- **Enter** — Apply all bindings at once

This lets you run, e.g., OpenCode with two providers: one for fast models, one for reasoning.

### CLI Management

Select **Manage CLIs** to edit config paths. For Copilot, a note is shown instead — the shell profile is auto-detected from `$SHELL`:

| Shell | Profile |
|-------|---------|
| `zsh` | `~/.zshrc` |
| `bash` | `~/.bashrc` |
| `fish` | `~/.config/fish/config.fish` |

### Restore Backup from TUI

Select **Restore Backup** from the dashboard menu:

1. Pick the CLI
2. Choose a backup from the timestamp-ordered list (newest first)
3. Confirm — backup overwrites the current config file

---

## Example: Claude Code + Bifrost (Anthropic)

This example walks through configuring **Claude Code** to use a custom **Anthropic-compatible** provider (Bifrost) hosted at `https://ai.intranet.istmocenter.com`.

### Step 1: Launch aimux

```bash
aimux
```

### Step 2: Add the provider

Navigate to **Manage Providers** → press **a** and fill in:

```
Name:        Bifrost (Anthropic)
Base URL:    https://ai.intranet.istmocenter.com
API Key:     <your-anthropic-api-key>
Auth Token:  <same or leave blank if same as API Key>
API Type:    Anthropic
```

Press Enter to submit. Aimux will call `GET https://ai.intranet.istmocenter.com/v1/models` with your API key, discover available models, and show the provider as **active** (green check).

### Step 3: Start the Switch flow

From the **Manage Providers** list, select "Bifrost (Anthropic)" and press **Enter**.

### Step 4: Select target CLI

Select **claude-code** from the CLI list.

### Step 5: Map models

Map the env vars to available models. Use "(Apply to all)" on fields 2+ to reuse the first model selection.

```
ANTHROPIC_DEFAULT_HAIKU_MODEL  → deepseek-v4-flash
ANTHROPIC_DEFAULT_SONNET_MODEL → deepseek-v4-pro
```

### Step 6: Review advanced config (Step 4/5)

Shows metadata per model: context window, max tokens, reasoning support, costs.

### Step 7: Confirm and apply

The dry-run shows a diff view — current config on the left, new env vars on the right.

Press **Enter** to apply. This will:

1. ✅ Backup `/Users/you/.config/claude/settings.json` to `~/.config/aimux/backups/settings.json-<hash>/`
2. ✅ Write the new config with `ANTHROPIC_BASE_URL=https://ai.intranet.istmocenter.com`, `ANTHROPIC_AUTH_TOKEN=...`, and model mappings
3. ✅ Prune old backups (keeps 5)
4. ✅ Show a green success notification

Press **Z** on the dashboard to undo (restore latest backup).

### Step 8: Verify

```bash
# Claude Code now uses your provider
claude

# You can verify with aimux CLI
aimux list
# → claude-code → Bifrost (Anthropic) (ACTIVE)
```

### What was written to Claude Code's config?

After applying, `~/.config/claude/settings.json` contains:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "https://ai.intranet.istmocenter.com",
    "ANTHROPIC_AUTH_TOKEN": "sk-ant-...",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "deepseek-v4-flash",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "deepseek-v4-pro"
  }
}
```

> **Note:** Aimux uses `ANTHROPIC_AUTH_TOKEN` (not `ANTHROPIC_API_KEY`) because Claude Code's login flow interferes with `ANTHROPIC_API_KEY`. The token is set in the `env` block to avoid polluting global environment variables.

### Quick undo

After applying, the dashboard shows `Z to undo`. Press **Z** to restore the latest backup instantly.

### Quick undo

After applying, the dashboard shows a `Z to undo` hint. Press **Z** to instantly restore the latest backup.

### Same example via CLI only

```bash
# The TUI is the primary entry point, but you can also:
aimux list                  # see what's active
aimux apply claude-code     # re-apply the binding
aimux backups claude-code   # see backup history
aimux restore claude-code   # restore previous config
```

---

## Example: Copilot + Local LLM

This example configures **GitHub Copilot** to use a local OpenAI-compatible server (Ollama, llama.cpp, etc.).

### Step 1: Add the provider

```
Name:          Local LLM
Base URL:      http://localhost:8080/v1
API Key:       (leave empty — local server needs no auth)
Auth Token:    (leave empty)
API Type:      OpenAI
```

### Step 2: Start Switch flow

From the provider list, select "Local LLM" and press Enter.

### Step 3: Select CLI

Choose **github-copilot**.

### Step 4: Select model

Copilot uses a single model. Pick one from the list.

### Step 5: Apply

aimux writes to your shell profile (`~/.zshrc`, `~/.bashrc`, or `~/.config/fish/config.fish`):

```bash
# >>> aimux copilot provider
# Managed by aimux — DO NOT EDIT BETWEEN MARKERS
export COPILOT_PROVIDER_BASE_URL="http://localhost:8080/v1"
export COPILOT_PROVIDER_TYPE="openai"
export COPILOT_MODEL="llama-3.1-8b"
# <<< aimux copilot provider
```

> **Note:** Copilot reads process env vars, not `.env` files. aimux writes to your shell profile instead. Restart the terminal or run `source ~/.zshrc` for changes to take effect.

---

## Backup System

Aimux makes **centralized backups** before every config mutation. Backups are stored in `~/.config/aimux/backups/` — NOT next to your CLI's config file.

### Structure

```
~/.config/aimux/backups/
├── settings.json-abc123def4/       ← hash of absolute config path
│   ├── settings.json.2026-06-18T03-21-00Z
│   ├── settings.json.2026-06-18T02-15-00Z
│   └── ...
├── config.json-987fedcba0/
│   ├── config.json.2026-06-18T04-00-00Z
│   └── ...
└── ...
```

### Retention

- Aimux keeps the **5 most recent backups** per config file and prunes older ones.
- You can list and restore backups via CLI:

```bash
aimux backups claude-code
aimux restore claude-code   # restores the newest backup
```

### Why centralized?

Old approach: backups were created as `settings.json.aimux-backup-2026-06-18T03:21:00Z` inside `~/.config/claude/`. This polluted the CLI's own config directory.

New approach: all backups are in `~/.config/aimux/backups/`, organized by a hash of the absolute source path so files sharing a basename (e.g. multiple `settings.json` files) don't collide.

### Environment variable

Override the backup root for testing or custom locations:

```bash
export AIMUX_BACKUP_ROOT=/mnt/backups/aimux
aimux apply claude-code
```

---

## CLI Reference

```
Usage:
  aimux                    Launch TUI (default)
  aimux apply <cli-name>   Apply active provider binding for a CLI
  aimux list               Show active multiplexes
  aimux backups <cli-name> List centralized backups for a CLI
  aimux restore <cli-name> Restore the latest backup for a CLI
  aimux version            Show version and check for updates
  aimux update             Update aimux to the latest release

Examples:
  aimux apply claude-code
  aimux backups claude-code
  aimux restore claude-code
```

### `aimux` (no arguments)

Launches the Bubbletea TUI with the dashboard view.

### `aimux apply <cli-name>`

Re-applies the active provider binding for the given CLI. Creates a backup and mutates the CLI's config file.

```bash
aimux apply claude-code
# → Applied. Backup saved to: /Users/you/.config/aimux/backups/settings.json-<hash>/settings.json.2026-06-18T03-21-00Z
```

### `aimux list`

Shows all active multiplexes — which CLI is bound to which provider.

```bash
$ aimux list
Active multiplexes:
  claude-code     → Bifrost (Anthropic)   (2026-06-18 11:07:51)
  opencode        → Bifrost               (2026-06-18 09:41:42)
  pi-ai           → Bifrost               (2026-06-18 11:01:20)
```

### `aimux backups <cli-name>`

Lists centralized backups for a CLI's config file, newest first.

```bash
$ aimux backups claude-code
Backups for 'claude-code' (newest first):
  [0] 2026-06-18T03-21-00Z
  [1] 2026-06-18T02-15-00Z
```

### `aimux restore <cli-name>`

Restores the **latest** backup for a CLI's config file. Overwrites the current config with the backup content using atomic write.

```bash
$ aimux restore claude-code
Restored latest backup: /Users/you/.config/aimux/backups/settings.json-<hash>/settings.json.2026-06-18T03-21-00Z
```

### `aimux version`

Shows the binary version and checks for updates using the GitHub releases API.

```bash
$ aimux version
aimux 0.2.0
Update available: v0.2.0 → v0.3.0
```

### `aimux update`

Self-updates the binary from the latest GitHub release. Detects Homebrew installs and delegates to `brew upgrade aimux`.

---

## Development

### Prerequisites

- Go 1.25+
- SQLite (embedded via `modernc.org/sqlite` — no external dependency needed)

### Build

```bash
git clone https://github.com/MileniumTick/aimux.git
cd aimux
go build -o aimux .
```

### Test

```bash
go test ./... -v
# → 95 passed in 8 packages
```

### Code structure

```
├── main.go                          Entrypoint, CLI routing
├── internal/
│   ├── application/
│   │   ├── path.go                  Path resolution (tilde expansion, config dir)
│   │   ├── provider_svc.go          Provider use cases (CRUD, fetch, retry, test)
│   │   ├── provider_svc_test.go
│   │   ├── multiplex_svc.go         Switch use cases (apply, bind, dry-run, multi-provider)
│   │   ├── multiplex_svc_test.go
│   │   └── helpers_test.go          Test setup (in-memory SQLite, seed data)
│   ├── domain/
│   │   ├── provider.go              Provider, ProviderModel, ModelMetadata types
│   │   ├── targetcli.go             TargetCLI, BackupResult, ConfigMutator interface
│   │   └── multiplex.go             ActiveMultiplex type
│   ├── infrastructure/
│   │   ├── config/
│   │   │   ├── utils.go             Atomic write, flock, backup system
│   │   │   ├── utils_test.go
│   │   │   └── model_catalog.go     Known model metadata (context window, etc.)
│   │   ├── mutators/                One file per CLI's config format
│   │   │   ├── claude_json.go       Claude Code settings.json
│   │   │   ├── claude_json_test.go
│   │   │   ├── opencode_json.go     OpenCode config.json
│   │   │   ├── codex_toml.go        Codex config.toml
│   │   │   ├── copilot_shell.go      Copilot shell profile
│   │   │   ├── pi_dual.go           pi agent models.json
│   │   │   └── *test.go
│   │   ├── sqlite/                  SQLite repositories + migrations + seed
│   │   │   ├── db.go
│   │   │   ├── provider_repo.go
│   │   │   ├── targetcli_repo.go
│   │   │   ├── multiplex_repo.go
│   │   │   └── queries_test.go
│   │   └── update/                  Self-update system
│   │       ├── cache.go
│   │       ├── checker.go
│   │       ├── updater.go
│   │       └── models.go
│   └── tui/
│       ├── model.go                 Bubbletea model, view, update loop
│       ├── table.go                 Dashboard table renderer
│       ├── forms.go                 All forms (add/edit provider, switch, map)
│       ├── menu.go                  Side menu and styles
│       └── tui_test.go
├── openspec/                        SDD/OpenSpec design artifacts
└── .gitignore
```

### Adding a new mutator

1. Create a new file in `internal/infrastructure/mutators/` implementing `domain.ConfigMutator`
2. Register it in `main.go`'s `mutatorRegistry` map
3. Add a seed row in `SeedTargetCLIs` in `internal/infrastructure/sqlite/db.go`
4. Write tests

### Design decisions

- **Single binary**: Go + embedded SQLite (`modernc.org/sqlite`) = zero external deps
- **Atomic writes**: Config mutations use temp-file + rename + fsync for crash safety
- **Flock locking**: File-level locking on config reads to prevent concurrent corruption
- **Trailing comma tolerance**: Hand-edited JSON is common; the parser retries after stripping trailing commas
- **ANTHROPIC_AUTH_TOKEN over ANTHROPIC_API_KEY**: Claude Code's OAuth login interferes with `API_KEY`; `AUTH_TOKEN` bypasses it
- **Centralized backups**: Backups live in `~/.config/aimux/backups/`, not next to the CLI's config file
- **`[1m]` suffix auto-detection**: Models with 1M+ context window get the suffix appended automatically for Claude Code
- **Multi-provider**: OpenCode, pi, and Copilot can have multiple simultaneous provider bindings
- **Copilot via shell profile**: Copilot reads process env vars, so aimux writes to `~/.zshrc`/`~/.bashrc`/`~/.config/fish/config.fish` instead of `.env` files
- **Discovery URL**: Separate URL for model discovery, useful when `/v1/models` differs from the chat endpoint

---

## FAQ

### Does aimux store my API keys?

Yes, in a SQLite database at `~/.config/aimux/matrix.db` with file permissions `0600` (owner-only read/write). The database is local to your machine. The API key is also written to the target CLI's config file when you apply a binding.

### Can I use aimux without the TUI?

Partially. The primary workflow (adding providers, binding, mapping models) goes through the TUI. The CLI supports `apply`, `list`, `backups`, `restore`, `version`, and `update`.

### What happens if the model fetch fails?

The provider is created with `status = "error"`. You can use **Retry** (`r` key) or **Test** (`t` key) in the provider list to re-attempt connectivity.

### Does it work with any OpenAI-compatible API?

Yes. Set the API Type to "OpenAI" and point the Base URL to your compatible endpoint. The mutator will use the standard `Bearer` auth header.

### Can I use it with a local LLM server?

Yes. Any server that exposes an OpenAI-compatible `/v1/models` endpoint works. Set `https://localhost:8080` (or your server's URL) as the Base URL.

### How do I revert a bad apply?

```bash
aimux backups claude-code     # see available backups
aimux restore claude-code     # restore the latest one
```

Or press **Z** on the dashboard for instant undo (restores the latest backup of the last applied CLI).

### Can I have multiple providers for the same CLI?

Yes, for **OpenCode**, **pi**, and **GitHub Copilot**. These CLIs support multi-provider. After binding the first provider, aimux shows a **Manage Bindings** view where you can add, edit, and remove provider bindings.

### How does aimux configure Copilot?

Copilot reads process environment variables, not `.env` files. Aimux writes `COPILOT_PROVIDER_*` vars to your shell profile (`~/.zshrc`, `~/.bashrc`, `~/.config/fish/config.fish`), wrapped in markers for idempotent updates. After applying, restart the terminal or `source` your profile.

### What is the Discovery URL for?

Some providers expose model discovery at a different URL than chat completions. The **Discovery URL** is optional — leave it empty to use the Base URL for both.

### What storage engine does aimux use?

SQLite via `modernc.org/sqlite` — a pure Go SQLite implementation with no CGO dependency. The database file is at `~/.config/aimux/matrix.db`.

---

## Full Documentation

| Doc | Description |
|-----|-------------|
| [ARCHITECTURE.md](ARCHITECTURE.md) | Layers, interfaces, data flow, key design decisions, package contracts |
| [DESIGN.md](DESIGN.md) | TUI visual design decisions: color palette, layout system, theme architecture |
| [docs/DATABASE.md](docs/DATABASE.md) | Schema reference, migration history, seed data, query patterns |
| [docs/SECURITY.md](docs/SECURITY.md) | Security model: data at rest, mutation integrity, threat model |
| [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) | Build, test, release, contribution guide |
| [docs/manual-de-usuario.md](docs/manual-de-usuario.md) | Manual de usuario en español |

---

## License

MIT
