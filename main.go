package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/MileniumTick/aimux/internal/application"
	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/daemon"
	"github.com/MileniumTick/aimux/internal/infrastructure/mutators"
	sqlite2 "github.com/MileniumTick/aimux/internal/infrastructure/sqlite"
	"github.com/MileniumTick/aimux/internal/infrastructure/update"
	"github.com/MileniumTick/aimux/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

// version is the aimux binary version. Override at build time with
// -ldflags "-X main.version=x.y.z". Defaults to a dev marker.
var version = "dev"

func main() {
	closeLog, err := application.SetupLogFile()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not set up log file: %v\n", err)
	}
	if closeLog != nil {
		defer closeLog()
	}

	db, cleanup, err := setupDB()
	if err != nil {
		log.Printf("database setup failed: %v", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer cleanup()

	providerRepo := &sqlite2.ProviderRepository{DB: db}
	cliRepo := &sqlite2.TargetCLIRepository{DB: db}
	multiplexRepo := &sqlite2.MultiplexRepository{DB: db}

	mutatorRegistry := map[string]domain.ConfigMutator{
		"claude-settings-json":   &mutators.ClaudeSettingsJSON{},
		"opencode-provider-json": &mutators.OpenCodeProviderJSON{},
		"codex-config-toml":      &mutators.CodexConfigTOML{},
		"copilot-shell-profile":  &mutators.CopilotShellProfile{},
		"pi-dual-json":           &mutators.PiDualJSON{},
	}

	switchUseCases := application.NewSwitchUseCases(providerRepo, cliRepo, multiplexRepo, mutatorRegistry)
	providerUseCases := application.NewProviderUseCases(providerRepo, multiplexRepo)

	if len(os.Args) > 1 {
		if err := runCLI(os.Args[1:], switchUseCases, db, mutatorRegistry); err != nil {
			os.Exit(1)
		}
		return
	}

	runTUI(providerUseCases, switchUseCases, mutatorRegistry)
}

func setupDB() (db *sql.DB, cleanup func(), err error) {
	dbPath, err := application.ResolveConfigPath()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve config path: %w", err)
	}

	configDir := filepath.Dir(dbPath)
	if configDir != "" {
		if err := os.MkdirAll(configDir, 0700); err != nil {
			return nil, nil, fmt.Errorf("create config directory: %w", err)
		}
	}

	db, err = sqlite2.Open(dbPath)
	if err != nil {
		return nil, nil, fmt.Errorf("open database: %w", err)
	}

	cleanup = func() { db.Close() }

	for _, step := range []func(*sql.DB) error{
		sqlite2.RunMigrations,
		sqlite2.MigrationAddMutatorColumns,
		sqlite2.MigrationDropApiTypeColumn,
		sqlite2.MigrationAddModelMetadataColumn,
		sqlite2.MigrationAddDiscoveryURLColumn,
		sqlite2.MigrationMultiProvider,
		sqlite2.MigrationRemoveOpenCodeNpm,
		sqlite2.MigrationAddDefaultContextWindow,
		sqlite2.MigrationAddLogoURL,
		sqlite2.MigrationAddCustomModelsColumn,
		sqlite2.CreateIndexes,
		sqlite2.SeedTargetCLIs,
		sqlite2.SeedDefaultProviders,
	} {
		if err := step(db); err != nil {
			db.Close()
			return nil, nil, fmt.Errorf("migration: %w", err)
		}
	}

	return db, cleanup, nil
}

func runTUI(providerUseCases *application.ProviderUseCases, switchUseCases *application.SwitchUseCases, mutatorRegistry map[string]domain.ConfigMutator) {
	for {
		model := tui.NewModel(providerUseCases, switchUseCases, version)
		program := tea.NewProgram(model, tea.WithAltScreen())
		if _, err := program.Run(); err != nil {
			log.Fatalf("Error running program: %v", err)
		}

		// Check for launch request
		launchPath := filepath.Join(os.Getenv("HOME"), ".config", "aimux", ".launch")
		data, err := os.ReadFile(launchPath)
		if err != nil {
			break
		}
		os.Remove(launchPath)

		var launchReq struct {
			CLI       string `json:"cli"`
			Provider  string `json:"provider"`
			Models    string `json:"models"`
			Reasoning string `json:"reasoning"`
		}
		if err := json.Unmarshal(data, &launchReq); err != nil || launchReq.CLI == "" {
			break
		}

		log.Printf("TUI requested launch: cli=%s provider=%s reasoning=%s", launchReq.CLI, launchReq.Provider, launchReq.Reasoning)

		db, cleanup, err := setupDB()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			break
		}
		if err := daemon.RunCLI(db, launchReq.CLI, launchReq.Provider, launchReq.Models, launchReq.Reasoning, mutatorRegistry); err != nil {
			fmt.Fprintf(os.Stderr, "\nError launching: %v\n", err)
			fmt.Println("Press Enter to return to aimux...")
			fmt.Scanln()
		}
		cleanup()
		// Loop: re-open TUI after agent finishes
	}
}

func runCLI(args []string, switchUseCases *application.SwitchUseCases, db *sql.DB, mutatorRegistry map[string]domain.ConfigMutator) error {
	if len(args) < 1 {
		fmt.Print(printHelp())
		return nil
	}

	switch args[0] {
	case "apply":
		if len(args) < 2 {
			return fmt.Errorf("usage: aimux apply <cli-name>")
		}
		cliName := args[1]
		cli, err := switchUseCases.FindCLIByName(cliName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return err
		}

		providerID, err := switchUseCases.GetProviderForCLI(cli.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: no active binding for '%s'. Use the TUI to set one up first.\n", cliName)
			return err
		}

		result, err := switchUseCases.Apply(cli.ID, providerID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return err
		}
		if result != nil && result.BackupPath != "" {
			fmt.Printf("Applied. Backup saved to: %s\n", result.BackupPath)
		} else {
			fmt.Println("Applied successfully.")
		}

	case "list":
		active, err := switchUseCases.ListActiveMultiplexes()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return err
		}
		if len(active) == 0 {
			fmt.Println("No active multiplexes.")
			clis, _ := switchUseCases.ListTargetCLIs()
			if len(clis) > 0 {
				fmt.Println("\nAvailable CLIs:")
				for _, c := range clis {
					path := c.ConfigPath
					if path == "" {
						if c.Mutator == "copilot-shell-profile" {
							path = "shell profile"
						} else {
							path = "auto-detect"
						}
					}
					fmt.Printf("  %s  (%s)\n", c.Name, path)
				}
			}
			return nil
		}
		fmt.Println("Active multiplexes:")
		for _, am := range active {
			fmt.Printf("  %-15s → %-15s  (%s)\n", am.CLIName, am.ProviderName, am.ActivatedAt)
		}

	case "backups":
		if len(args) < 2 {
			return fmt.Errorf("usage: aimux backups <cli-name>")
		}
		backups, err := switchUseCases.ListBackups(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return err
		}
		if len(backups) == 0 {
			fmt.Printf("No backups for '%s'.\n", args[1])
			return nil
		}
		fmt.Printf("Backups for '%s' (newest first):\n", args[1])
		for i, b := range backups {
			fmt.Printf("  [%d] %s\n", i, b.When)
		}

	case "restore":
		if len(args) < 2 {
			return fmt.Errorf("usage: aimux restore <cli-name>")
		}
		bp, err := switchUseCases.RestoreLatest(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return err
		}
		fmt.Printf("Restored latest backup: %s\n", bp)

	case "version":
		fmt.Printf("aimux %s\n", version)
		info := update.CheckForUpdate(version, &http.Client{Timeout: 5 * time.Second})
		if info.HasUpdate {
			fmt.Printf("Update available: v%s → v%s\n", version, info.LatestVersion)
		}

	case "update":
		execPath, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot resolve executable path: %v\n", err)
			return err
		}
		if update.IsHomebrewInstall(execPath) {
			os.Exit(update.HomebrewUpdate())
		}
		if err := update.SelfUpdate(version, execPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return err
		}

	case "run":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: aimux run <cli-name> [args...]")
			fmt.Fprintln(os.Stderr, "Examples:")
			fmt.Fprintln(os.Stderr, "  aimux run claude-code")
			fmt.Fprintln(os.Stderr, "  aimux run opencode --fast")
			return fmt.Errorf("missing CLI name")
		}
		if err := daemon.RunCLI(db, args[1], "", "", "", mutatorRegistry); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return err
		}

	case "exec":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: aimux exec <cli-name> -- <command> [args...]")
			fmt.Fprintln(os.Stderr, "Example: aimux exec claude-code -- claude")
			return fmt.Errorf("missing CLI name")
		}

		cliName := args[1]
		var cmdArgs []string
		if len(args) > 2 && args[2] == "--" {
			cmdArgs = args[3:]
		} else {
			cmdArgs = args[2:]
		}

		if len(cmdArgs) == 0 {
			return fmt.Errorf("no command specified after CLI name")
		}

		// Resolve env vars via direct DB
		result, err := daemon.ResolveViaDB(db, cliName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return err
		}

		log.Printf("exec %s → %s: %d env vars", cliName, result.ProviderName, len(result.Env))

		// Build env: our vars first, then inherit non-conflicting vars
		env := make([]string, 0, len(result.Env)+len(os.Environ()))
		for k, v := range result.Env {
			env = append(env, k+"="+v)
		}
		managed := make(map[string]bool, len(result.Env))
		for k := range result.Env {
			managed[k] = true
		}
		for _, e := range os.Environ() {
			eq := strings.IndexByte(e, '=')
			if eq > 0 {
				key := e[:eq]
				if !managed[key] {
					env = append(env, e)
				}
			}
		}
		if err := syscall.Exec(cmdArgs[0], cmdArgs, env); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			return err
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		fmt.Fprint(os.Stderr, printHelp())
		return fmt.Errorf("unknown command: %s", args[0])
	}
	return nil
}

func printHelp() string {
	return `aimux — AI provider multiplexer for dev CLIs

Usage:
  aimux                    Launch TUI (default)
  aimux apply <cli-name>   Apply active provider binding for a CLI
  aimux run <cli> [args]   Launch a CLI agent with resolved credentials (auto-detect binary)
  aimux exec <cli> -- ...  Run a command with resolved env vars (daemon or direct)
  aimux list               Show active multiplexes
  aimux backups <cli-name> List centralized backups for a CLI
  aimux restore <cli-name> Restore the latest backup for a CLI
  aimux version            Show version and check for updates
  aimux update             Update aimux to the latest release

Examples:
  aimux apply claude-code
  aimux run claude-code
  aimux run opencode --fast
  aimux exec claude-code -- claude
  aimux backups claude-code
  aimux restore claude-code
`
}
