package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jchavarriam/aimux/internal/application"
	"github.com/jchavarriam/aimux/internal/domain"
	"github.com/jchavarriam/aimux/internal/infrastructure/mutators"
	sqlite2 "github.com/jchavarriam/aimux/internal/infrastructure/sqlite"
	"github.com/jchavarriam/aimux/internal/tui"
)

func main() {
	db, cleanup, err := setupDB()
	if err != nil {
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
		"copilot-env-file":       &mutators.CopilotEnvFile{},
		"pi-dual-json":           &mutators.PiDualJSON{},
	}

	switchUseCases := application.NewSwitchUseCases(providerRepo, cliRepo, multiplexRepo, mutatorRegistry)
	providerUseCases := application.NewProviderUseCases(providerRepo, multiplexRepo)

	if len(os.Args) > 1 {
		runCLI(os.Args[1:], switchUseCases, providerUseCases)
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
		sqlite2.MigrationAddApiTypeColumn,
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
	model := tui.NewModel(providerUseCases, switchUseCases)
	program := tea.NewProgram(model, tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		log.Fatalf("Error running program: %v", err)
	}
}

func runCLI(args []string, switchUseCases *application.SwitchUseCases, providerUseCases *application.ProviderUseCases) {
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
		cli, err := findCLIByName(cliName, switchUseCases)
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
					fmt.Printf("  %s  (%s)\n", c.Name, c.ConfigPath)
				}
			}
			return
		}
		fmt.Println("Active multiplexes:")
		for _, am := range active {
			fmt.Printf("  %-15s → %-15s  (%s)\n", am.CLIName, am.ProviderName, am.ActivatedAt)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", args[0])
		printHelp()
		os.Exit(1)
	}
}

func findCLIByName(name string, uc *application.SwitchUseCases) (*domain.TargetCLI, error) {
	clis, err := uc.ListTargetCLIs()
	if err != nil {
		return nil, fmt.Errorf("list CLIs: %w", err)
	}
	for _, c := range clis {
		if strings.EqualFold(c.Name, name) {
			return &c, nil
		}
	}
	return nil, fmt.Errorf("CLI '%s' not found", name)
}

func printHelp() {
	fmt.Print(`aimux — AI provider multiplexer for dev CLIs

Usage:
  aimux                  Launch TUI (default)
  aimux apply <cli-name> Apply active provider binding for a CLI
  aimux list             Show active multiplexes

Examples:
  aimux apply claude-code
  aimux apply codex
  aimux list
`)
}
