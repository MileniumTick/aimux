package daemon

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/MileniumTick/aimux/internal/application"
	"github.com/MileniumTick/aimux/internal/domain"
	"github.com/MileniumTick/aimux/internal/infrastructure/config"
	"github.com/MileniumTick/aimux/internal/infrastructure/mutators"
	"github.com/MileniumTick/aimux/internal/infrastructure/sqlite"
)

const socketName = "aimuxd.sock"

// Server is the aimux daemon that resolves credentials via a Unix socket.
type Server struct {
	db         *sql.DB
	socketPath string
	server     *http.Server
	mux        *http.ServeMux

	providerRepo  *sqlite.ProviderRepository
	cliRepo       *sqlite.TargetCLIRepository
	multiplexRepo *sqlite.MultiplexRepository
}

// StartDaemon opens the DB, starts the Unix socket listener, and blocks until
// SIGINT/SIGTERM or Stop is called. Returns the socket path on success.
func StartDaemon(db *sql.DB) (string, error) {
	socketPath, err := daemonSocketPath()
	if err != nil {
		return "", fmt.Errorf("resolve daemon socket path: %w", err)
	}

	// Remove stale socket file
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("remove stale socket: %w", err)
	}

	srv := &Server{
		db:            db,
		socketPath:    socketPath,
		providerRepo:  &sqlite.ProviderRepository{DB: db},
		cliRepo:       &sqlite.TargetCLIRepository{DB: db},
		multiplexRepo: &sqlite.MultiplexRepository{DB: db},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/resolve/", srv.handleResolve)
	mux.HandleFunc("/v1/health", srv.handleHealth)
	mux.HandleFunc("/v1/stop", srv.handleStop)
	srv.mux = mux

	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return "", fmt.Errorf("listen on unix socket: %w", err)
	}
	// Secure the socket — only owner can connect
	os.Chmod(socketPath, 0600)

	srv.server = &http.Server{
		Handler: mux,
	}

	// Graceful shutdown on signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("aimuxd: shutting down...")
		srv.server.Close()
	}()

	log.Printf("aimuxd: listening on %s", socketPath)
	if err := srv.server.Serve(listener); err != nil && err != http.ErrServerClosed {
		return "", fmt.Errorf("serve: %w", err)
	}

	return socketPath, nil
}

// handleResolve resolves env vars for the CLI named in the URL path.
// GET /v1/resolve/<cli-name>
func (s *Server) handleResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	cliName := strings.TrimPrefix(r.URL.Path, "/v1/resolve/")
	cliName = strings.TrimSpace(cliName)
	if cliName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cli name required"})
		return
	}

	result, err := s.resolve(cliName)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// handleHealth returns a simple health check.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleStop triggers a graceful shutdown.
func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopping"})
	go func() {
		s.server.Close()
	}()
}

// resolve looks up the CLI, its active binding, and resolves all env vars.
func (s *Server) resolve(cliName string) (*ResolveResult, error) {
	// Find CLI
	clis, err := s.cliRepo.List()
	if err != nil {
		return nil, fmt.Errorf("list CLIs: %w", err)
	}
	var cli *domain.TargetCLI
	for i := range clis {
		if strings.EqualFold(clis[i].Name, cliName) {
			cli = &clis[i]
			break
		}
	}
	if cli == nil {
		return nil, fmt.Errorf("CLI '%s' not found", cliName)
	}

	// Get active bindings
	bindings, err := s.multiplexRepo.ListForCLI(cli.ID)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	if len(bindings) == 0 {
		return nil, fmt.Errorf("no active binding for '%s'. Use the TUI to set one up first", cliName)
	}

	// Use the first binding
	binding := bindings[0]

	// Get provider
	provider, err := s.providerRepo.Get(binding.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	// Parse model mappings
	mappings := make(map[string]string)
	if err := json.Unmarshal([]byte(binding.ModelMappings), &mappings); err != nil {
		return nil, fmt.Errorf("parse model mappings: %w", err)
	}

	// Parse mutator config for extra_env
	mutatorCfg := make(map[string]any)
	if cli.MutatorConfig != "" && cli.MutatorConfig != "{}" {
		if err := json.Unmarshal([]byte(cli.MutatorConfig), &mutatorCfg); err != nil {
			return nil, fmt.Errorf("parse mutator config: %w", err)
		}
	}

	// Parse env var names from cli.EnvVars and inject into mutatorCfg
	var envVarNames []string
	if cli.EnvVars != "" {
		json.Unmarshal([]byte(cli.EnvVars), &envVarNames)
	}
	mutatorCfg["_env_var_names"] = envVarNames
	mutatorCfg["_mutator_name"] = cli.Mutator

	return resolveEnvVars(cliName, provider, mappings, mutatorCfg)
}

// daemonSocketPath returns the absolute path to the daemon Unix socket.
func daemonSocketPath() (string, error) {
	configDir, err := application.ResolveConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, socketName), nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// ── Client (aimux exec) ──────────────────────────────────────────────────────

// ResolveViaDaemon connects to the running daemon and resolves env vars for a CLI.
func ResolveViaDaemon(cliName string) (*ResolveResult, error) {
	socketPath, err := daemonSocketPath()
	if err != nil {
		return nil, fmt.Errorf("resolve socket path: %w", err)
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}

	url := fmt.Sprintf("http://unix/v1/resolve/%s", cliName)
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("daemon not reachable at %s (start with 'aimux daemon'): %w", socketPath, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error string `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)
		if errResp.Error != "" {
			return nil, fmt.Errorf("daemon: %s", errResp.Error)
		}
		return nil, fmt.Errorf("daemon: %s", resp.Status)
	}

	var result ResolveResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode daemon response: %w", err)
	}

	return &result, nil
}

// ResolveViaDB resolves env vars for a CLI by querying the database directly.
// Used as fallback when the daemon is not running.
func ResolveViaDB(db *sql.DB, cliName string) (*ResolveResult, error) {
	providerRepo := &sqlite.ProviderRepository{DB: db}
	cliRepo := &sqlite.TargetCLIRepository{DB: db}
	multiplexRepo := &sqlite.MultiplexRepository{DB: db}

	// Find CLI
	clis, err := cliRepo.List()
	if err != nil {
		return nil, fmt.Errorf("list CLIs: %w", err)
	}
	var cli *domain.TargetCLI
	for i := range clis {
		if strings.EqualFold(clis[i].Name, cliName) {
			cli = &clis[i]
			break
		}
	}
	if cli == nil {
		return nil, fmt.Errorf("CLI '%s' not found", cliName)
	}

	// Get active bindings
	bindings, err := multiplexRepo.ListForCLI(cli.ID)
	if err != nil {
		return nil, fmt.Errorf("list bindings: %w", err)
	}
	if len(bindings) == 0 {
		return nil, fmt.Errorf("no active binding for '%s'. Use the TUI to set one up first", cliName)
	}

	binding := bindings[0]

	provider, err := providerRepo.Get(binding.ProviderID)
	if err != nil {
		return nil, fmt.Errorf("get provider: %w", err)
	}

	mappings := make(map[string]string)
	if err := json.Unmarshal([]byte(binding.ModelMappings), &mappings); err != nil {
		return nil, fmt.Errorf("parse model mappings: %w", err)
	}

	mutatorCfg := make(map[string]any)
	if cli.MutatorConfig != "" && cli.MutatorConfig != "{}" {
		if err := json.Unmarshal([]byte(cli.MutatorConfig), &mutatorCfg); err != nil {
			return nil, fmt.Errorf("parse mutator config: %w", err)
		}
	}

	var envVarNames []string
	if cli.EnvVars != "" {
		json.Unmarshal([]byte(cli.EnvVars), &envVarNames)
	}
	mutatorCfg["_env_var_names"] = envVarNames
	mutatorCfg["_mutator_name"] = cli.Mutator

	return resolveEnvVars(cliName, provider, mappings, mutatorCfg)
}

// StopDaemon sends a stop signal to the running daemon.
func StopDaemon() error {
	socketPath, err := daemonSocketPath()
	if err != nil {
		return err
	}

	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", socketPath)
			},
		},
	}

	resp, err := client.Post("http://unix/v1/stop", "application/json", nil)
	if err != nil {
		return fmt.Errorf("daemon not reachable at %s: %w", socketPath, err)
	}
	resp.Body.Close()
	return nil
}

// cliBinaries maps known CLI names to their default binary names.
var cliBinaries = map[string]string{
	"claude-code":    "claude",
	"opencode":       "opencode",
	"codex":          "codex",
	"github-copilot": "github-copilot",
	"pi-ai":          "pi",
}

// mutatorRegistry provides mutators for writing config files during run/launch.
var mutatorRegistry = map[string]domain.ConfigMutator{
	"claude-settings-json":   &mutators.ClaudeSettingsJSON{},
	"opencode-provider-json": &mutators.OpenCodeProviderJSON{},
	"codex-config-toml":      &mutators.CodexConfigTOML{},
	"copilot-shell-profile":  &mutators.CopilotShellProfile{},
	"pi-dual-json":           &mutators.PiDualJSON{},
}

// ResolveBinary resolves the binary path for a CLI name.
// First checks if the CLI has a custom binary_path in its mutator_config,
// then falls back to the known binary name map, then tries PATH lookup.
func ResolveBinary(db *sql.DB, cliName string) (string, error) {
	cliRepo := &sqlite.TargetCLIRepository{DB: db}
	clis, err := cliRepo.List()
	if err != nil {
		return "", fmt.Errorf("list CLIs: %w", err)
	}
	var cli *domain.TargetCLI
	for i := range clis {
		if strings.EqualFold(clis[i].Name, cliName) {
			cli = &clis[i]
			break
		}
	}
	if cli == nil {
		return "", fmt.Errorf("CLI '%s' not found", cliName)
	}

	// Check custom binary path in mutator_config
	if cli.MutatorConfig != "" && cli.MutatorConfig != "{}" {
		var cfg map[string]any
		if err := json.Unmarshal([]byte(cli.MutatorConfig), &cfg); err == nil {
			if bp, ok := cfg["binary_path"].(string); ok && bp != "" {
				if expanded, err := application.ExpandTilde(bp); err == nil {
					if _, err := os.Stat(expanded); err == nil {
						return expanded, nil
					}
				}
			}
		}
	}

	// Try known binary name
	binName, known := cliBinaries[cliName]
	if !known {
		binName = cliName
	}

	// Look up in PATH
	binary, err := exec.LookPath(binName)
	if err != nil {
		return "", fmt.Errorf("binary '%s' not found in PATH for '%s'. Install it or set binary_path in CLI config", binName, cliName)
	}
	return binary, nil
}

// RunCLI resolves credentials for a CLI and launches its binary.
// If providerName is set, uses that provider instead of the bound one.
// models is JSON: either {"ENV_VAR":"model",...} (env mapping) or ["model1","model2",...] (registered).
// reasoning is the reasoning level: "off", "low", "medium", "high", "max".
func RunCLI(db *sql.DB, cliName, providerName, models, reasoning string) error {
	cliRepo := &sqlite.TargetCLIRepository{DB: db}
	providerRepo := &sqlite.ProviderRepository{DB: db}
	multiplexRepo := &sqlite.MultiplexRepository{DB: db}

	clis, err := cliRepo.List()
	if err != nil {
		return fmt.Errorf("list CLIs: %w", err)
	}
	var cli *domain.TargetCLI
	for i := range clis {
		if strings.EqualFold(clis[i].Name, cliName) {
			cli = &clis[i]
			break
		}
	}
	if cli == nil {
		return fmt.Errorf("CLI '%s' not found", cliName)
	}

	// Resolve provider: by name (launch) or from binding (run with binding)
	var provider domain.Provider
	mappings := make(map[string]string)

	if providerName != "" {
		// Look up provider by name (from Launch flow)
		allProviders, err := providerRepo.List()
		if err != nil {
			return fmt.Errorf("list providers: %w", err)
		}
		var found bool
		for _, p := range allProviders {
			if strings.EqualFold(p.Name, providerName) {
				provider = p
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("provider '%s' not found", providerName)
		}
		// Parse models JSON: if it's an object ({"ENV":"model",...}), use as mappings.
		// If it's an array (["m1","m2",...]), set _registered_models later.
		if models != "" {
			if err := json.Unmarshal([]byte(models), &mappings); err != nil {
				// Not an object, might be an array — handled later via _registered_models
				mappings = make(map[string]string)
			}
		}
	} else {
		// Use binding from DB
		bindings, err := multiplexRepo.ListForCLI(cli.ID)
		if err != nil {
			return fmt.Errorf("list bindings: %w", err)
		}
		if len(bindings) == 0 {
			return fmt.Errorf("no active binding for '%s'. Use the TUI to set one up first", cliName)
		}

		binding := bindings[0]
		provider, err = providerRepo.Get(binding.ProviderID)
		if err != nil {
			return fmt.Errorf("get provider: %w", err)
		}

		if err := json.Unmarshal([]byte(binding.ModelMappings), &mappings); err != nil {
			mappings = make(map[string]string)
		}
	}

	configPath := cli.ConfigPath
	if strings.HasPrefix(configPath, "~") {
		if hd, err := os.UserHomeDir(); err == nil {
			configPath = filepath.Join(hd, configPath[1:])
		}
	}

	// ── Backup config + write via mutator (non-env-var CLIs) ─────────
	// Claude Code supports env var auth — no file mutation at all.
	// For opencode/pi/codex/copilot, backup → write → launch → restore.
	var backupPath string
	var fileExisted bool
	if cli.Mutator != "" && configPath != "" {
		if fi, err := os.Stat(configPath); err == nil && fi.Mode().IsRegular() {
			fileExisted = true
			if bp, err := config.CreateBackup(configPath); err == nil {
				backupPath = bp
				log.Printf("backed up %s", filepath.Base(bp))
			}
		}
		defer func() {
			if backupPath != "" {
				if err := config.RestoreBackup(backupPath, configPath); err != nil {
					log.Printf("warning: restore failed: %v", err)
				} else {
					log.Printf("restored config from backup")
				}
			} else if !fileExisted {
				if err := os.Remove(configPath); err != nil && !os.IsNotExist(err) {
					log.Printf("warning: failed to remove created config: %v", err)
				} else {
					log.Printf("removed config file created by launch")
				}
			}
		}()
	
		if m, ok := mutatorRegistry[cli.Mutator]; ok {
			applyCfg := make(map[string]any)
			if cli.MutatorConfig != "" && cli.MutatorConfig != "{}" {
				json.Unmarshal([]byte(cli.MutatorConfig), &applyCfg)
			}
			applyCfg["_clear_providers"] = true
			applyCfg["provider_id"] = strings.ToLower(strings.ReplaceAll(provider.Name, " ", "-"))
			// Parse models JSON for registered_models (array format)
			if models != "" && strings.HasPrefix(models, "[") {
				var regModels []string
				if err := json.Unmarshal([]byte(models), &regModels); err == nil && len(regModels) > 0 {
					applyCfg["_registered_models"] = regModels
				}
			}
			models, _ := providerRepo.ListModels(provider.ID)
			if len(models) > 0 {
				modelMeta := make(map[string]any)
				for _, model := range models {
					if len(model.Metadata) > 0 {
						modelMeta[model.ModelName] = map[string]any(model.Metadata)
					}
				}
				if len(modelMeta) > 0 {
					applyCfg["_model_metadata"] = modelMeta
				}
			}
			if _, err := m.Mutate(configPath, mappings, provider, applyCfg); err != nil {
				log.Printf("warning: config write failed: %v", err)
			}
		}
	}

	// ── Inject env vars from .env file (for codex) ──────────────────
	// Some mutators (codex) write a .env alongside the config. The agent
	// expects those env vars in its environment.
	dotEnv := make(map[string]string)
	if configPath != "" {
		envDir := filepath.Dir(configPath)
		if data, err := os.ReadFile(filepath.Join(envDir, ".env")); err == nil {
			for _, line := range strings.Split(string(data), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				eq := strings.IndexByte(line, '=')
				if eq > 0 {
					if k, v := line[:eq], line[eq+1:]; k != "" && v != "" {
						dotEnv[k] = v
					}
				}
			}
		}
	}

	// Resolve env vars
	mutatorCfg := make(map[string]any)
	if cli.MutatorConfig != "" && cli.MutatorConfig != "{}" {
		json.Unmarshal([]byte(cli.MutatorConfig), &mutatorCfg)
	}
	var envVarNames []string
	if cli.EnvVars != "" {
		json.Unmarshal([]byte(cli.EnvVars), &envVarNames)
	}
	mutatorCfg["_env_var_names"] = envVarNames
	mutatorCfg["_mutator_name"] = cli.Mutator

	// Inject model metadata for context suffix resolution
	provModels, _ := providerRepo.ListModels(provider.ID)
	if len(provModels) > 0 {
		modelMeta := make(map[string]any)
		for _, m := range provModels {
			if len(m.Metadata) > 0 {
				modelMeta[m.ModelName] = map[string]any(m.Metadata)
			}
		}
		if len(modelMeta) > 0 {
			mutatorCfg["_model_metadata"] = modelMeta
		}
	}

	// ── Inject reasoning level ──────────────────────────────────────
	if reasoning != "" && cli.Mutator == "claude-settings-json" {
		mutatorCfg["_reasoning_level"] = reasoning
		mutatorCfg["extra_env_disabled"] = false
		extra := make(map[string]any)
		if e, ok := mutatorCfg["extra_env"].(map[string]any); ok {
			extra = e
		}
		extra["CLAUDE_CODE_EFFORT_LEVEL"] = reasoning
		mutatorCfg["extra_env"] = extra
	}

	result, err := resolveEnvVars(cliName, provider, mappings, mutatorCfg)
	if err != nil {
		return fmt.Errorf("resolve env: %w", err)
	}

	// ── Resolve binary ─────────────────────────────────────────────────
	binary, err := ResolveBinary(db, cliName)
	if err != nil {
		return fmt.Errorf("resolve binary: %w", err)
	}

	log.Printf("run %s → %s (%s): %d env vars", cliName, result.ProviderName, binary, len(result.Env))

	// ── Build env (our vars first, then non-conflicting inherited) ──────
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

	// ── Inject .env vars (from mutator-written .env files) ──────────
	for k, v := range dotEnv {
		if !managed[k] {
			env = append(env, k+"="+v)
			managed[k] = true
		}
	}

	// ── Launch agent as subprocess ──────────────────────────────────────
	cmd := exec.Command(binary)
	cmd.Env = env
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("run %s: %w", binary, err)
	}

	return nil
}

// isClaudeEnvVar returns true if the env var name is relevant for Claude Code's
// settings.json. Filters out vars from other providers (OPENAI_*, etc.) that
// don't belong in Claude's config.
func isClaudeEnvVar(name string) bool {
	upper := strings.ToUpper(name)
	// Anthropic/Claude-specific vars
	if strings.HasPrefix(upper, "ANTHROPIC_") {
		return true
	}
	if strings.HasPrefix(upper, "CLAUDE_") {
		return true
	}
	return false
}
