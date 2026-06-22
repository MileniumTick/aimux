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
		runCLI(os.Args[1:], switchUseCases, db)
		return
	}

	runTUI(providerUseCases, switchUseCases)
}

func setupDB() (db *sql.DB, cleanup func(), err error) {
	dbPath, err := application.ResolveConfigPath()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve config path: %w", err)
	}

	configDir := ""
	for i := len(dbPath) - 1; i >= 0; i-- {
		if dbPath[i] == '/' {
			configDir = dbPath[:i]
			break
		}
	}
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
		sqlite2.CreateIndexes,
		sqlite2.SeedTargetCLIs,
	} {
		if err := step(db); err != nil {
			db.Close()
			return nil, nil, fmt.Errorf("migration: %w", err)
		}
	}

	return db, cleanup, nil
}

func runTUI(providerUseCases *application.ProviderUseCases, switchUseCases *application.SwitchUseCases) {
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
			CLI      string `json:"cli"`
			Provider string `json:"provider"`
			Models   string `json:"models"`
		}
		if err := json.Unmarshal(data, &launchReq); err != nil || launchReq.CLI == "" {
			break
		}

		log.Printf("TUI requested launch: cli=%s provider=%s models=%s", launchReq.CLI, launchReq.Provider, launchReq.Models)

		db, cleanup, err := setupDB()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			break
		}
		if err := daemon.RunCLI(db, launchReq.CLI, launchReq.Provider, launchReq.Models); err != nil {
			fmt.Fprintf(os.Stderr, "\nError launching: %v\n", err)
			fmt.Println("Press Enter to return to aimux...")
			fmt.Scanln()
		}
		cleanup()
		// Loop: re-open TUI after agent finishes
	}
}

func runCLI(args []string, switchUseCases *application.SwitchUseCases, db *sql.DB) {
	if len(args) < 1 {
		printHelp()
		return
	}

	switch args[0] {
	case "apply":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: aimux apply <cli-name>")
			os.Exit(1)
		}
		cliName := args[1]
		cli, err := switchUseCases.FindCLIByName(cliName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		providerID, err := switchUseCases.GetProviderForCLI(cli.ID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: no active binding for '%s'. Use the TUI to set one up first.\n", cliName)
			os.Exit(1)
		}

		result, err := switchUseCases.Apply(cli.ID, providerID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
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
			os.Exit(1)
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
			return
		}
		fmt.Println("Active multiplexes:")
		for _, am := range active {
			fmt.Printf("  %-15s → %-15s  (%s)\n", am.CLIName, am.ProviderName, am.ActivatedAt)
		}

	case "backups":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: aimux backups <cli-name>")
			os.Exit(1)
		}
		backups, err := switchUseCases.ListBackups(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if len(backups) == 0 {
			fmt.Printf("No backups for '%s'.\n", args[1])
			return
		}
		fmt.Printf("Backups for '%s' (newest first):\n", args[1])
		for i, b := range backups {
			fmt.Printf("  [%d] %s\n", i, b.When)
		}

	case "restore":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: aimux restore <cli-name>")
			os.Exit(1)
		}
		bp, err := switchUseCases.RestoreLatest(args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Restored latest backup: %s\n", bp)

	case "version":
		fmt.Printf("aimux %s\n", version)
		info := update.CheckForUpdate(version, db, &http.Client{Timeout: 5 * time.Second})
		if info.HasUpdate {
			fmt.Printf("Update available: v%s → v%s\n", version, info.LatestVersion)
		}

	case "update":
		execPath, err := os.Executable()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot resolve executable path: %v\n", err)
			os.Exit(1)
		}
		if update.IsHomebrewInstall(execPath) {
			os.Exit(update.HomebrewUpdate())
		}
		if err := update.SelfUpdate(version, execPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "daemon":
		fmt.Println("Starting aimux daemon...")
		socketPath, err := daemon.StartDaemon(db)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Daemon stopped. Socket: %s\n", socketPath)

	case "daemon-stop":
		if err := daemon.StopDaemon(); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Daemon stopped.")

	case "run":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: aimux run <cli-name> [args...]")
			fmt.Fprintln(os.Stderr, "Examples:")
			fmt.Fprintln(os.Stderr, "  aimux run claude-code")
			fmt.Fprintln(os.Stderr, "  aimux run opencode --fast")
			os.Exit(1)
		}
		if err := daemon.RunCLI(db, args[1], "", ""); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

	case "exec":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "Usage: aimux exec <cli-name> -- <command> [args...]")
			fmt.Fprintln(os.Stderr, "Example: aimux exec claude-code -- claude")
			os.Exit(1)
		}

		cliName := args[1]
		var cmdArgs []string
		if len(args) > 2 && args[2] == "--" {
			cmdArgs = args[3:]
		} else {
			cmdArgs = args[2:]
		}

		if len(cmdArgs) == 0 {
			fmt.Fprintf(os.Stderr, "Error: no command specified after CLI name\n")
			os.Exit(1)
		}

		// Try daemon first, fall back to direct DB
		result, err := daemon.ResolveViaDaemon(cliName)
		if err != nil {
			log.Printf("daemon not reachable, falling back to direct DB: %v", err)
			result, err = daemon.ResolveViaDB(db, cliName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
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
			os.Exit(1)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Print(`aimux — AI provider multiplexer for dev CLIs

Usage:
  aimux                    Launch TUI (default)
  aimux apply <cli-name>   Apply active provider binding for a CLI
  aimux run <cli> [args]   Launch a CLI agent with resolved credentials (auto-detect binary)
  aimux exec <cli> -- ...  Run a command with resolved env vars (daemon or direct)
  aimux list               Show active multiplexes
  aimux backups <cli-name> List centralized backups for a CLI
  aimux restore <cli-name> Restore the latest backup for a CLI
  aimux daemon             Start the credential daemon (Unix socket)
  aimux daemon-stop        Stop the running daemon
  aimux version            Show version and check for updates
  aimux update             Update aimux to the latest release

Examples:
  aimux apply claude-code
  aimux run claude-code
  aimux run opencode --fast
  aimux exec claude-code -- claude
  aimux daemon
  aimux daemon-stop
  aimux backups claude-code
  aimux restore claude-code
`)
}
