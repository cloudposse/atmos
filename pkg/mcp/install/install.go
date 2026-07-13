// Package install writes configured MCP servers into supported client config files.
//
//nolint:gocritic,gosec,nestif,revive // Target/config rendering code favors explicit value objects and client-specific file formats.
package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"

	yaml "gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/config/homedir"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

const (
	ScopeProject = "project"
	ScopeUser    = "user"

	ClientClaudeCode    = "claude-code"
	ClientCursor        = "cursor"
	ClientVSCode        = "vscode"
	ClientCodex         = "codex"
	ClientGemini        = "gemini"
	ClientClaudeDesktop = "claude-desktop"
	ClientWindsurf      = "windsurf"
	ClientCline         = "cline"
	ClientClineCLI      = "cline-cli"
	ClientZed           = "zed"
	ClientOpenCode      = "opencode"
	ClientGoose         = "goose"
	// ClientCopilotCLI is the standalone `copilot` CLI product
	// (https://docs.github.com/en/copilot/how-tos/copilot-cli), distinct
	// from ClientVSCode's GitHub Copilot *extension*. Both historically
	// aliased the string "github-copilot-cli" onto ClientVSCode; see
	// normalizeClient for why that changed.
	ClientCopilotCLI  = "copilot-cli"
	ClientAntigravity = "antigravity"
	ClientMCPorter    = "mcporter"

	configFileMode = 0o600
	configDirMode  = 0o755

	mcpJSONFile = "mcp.json"
	newline     = "\n"
)

var SupportedClients = []string{
	ClientClaudeCode,
	ClientCursor,
	ClientVSCode,
	ClientCodex,
	ClientGemini,
	ClientClaudeDesktop,
	ClientWindsurf,
	ClientCline,
	ClientClineCLI,
	ClientZed,
	ClientOpenCode,
	ClientGoose,
	ClientCopilotCLI,
	ClientAntigravity,
	ClientMCPorter,
}

// globalOnlyClients lists clients that only support user-scope (global)
// installs: either they're a desktop/standalone app with no
// project-workspace concept (claude-desktop, windsurf, antigravity,
// copilot-cli), or their config lives in the editor's global extension
// storage rather than anywhere project-relative (cline, cline-cli).
// ResolveTarget rejects ScopeProject for these with errUnsupportedScope
// instead of silently resolving to a useless path.
var globalOnlyClients = map[string]bool{
	ClientClaudeDesktop: true,
	ClientWindsurf:      true,
	ClientCline:         true,
	ClientClineCLI:      true,
	ClientAntigravity:   true,
	ClientCopilotCLI:    true,
}

var (
	errUnsupportedManagedScope = errors.New("managed/system MCP configuration is client-specific and is not implemented")
	errUnsupportedScope        = errors.New("unsupported MCP install scope")
	errUnsupportedClient       = errors.New("unsupported MCP client")
	errNoMCPClientsSelected    = errors.New("no MCP clients selected")
	errNoMCPServersSelected    = errors.New("no MCP servers selected")
	errInvalidServersSection   = errors.New("invalid servers section")
)

type ConflictFunc func(target Target, serverName string) (bool, error)

type Options struct {
	BasePath   string
	HomeDir    string
	Scope      string
	Clients    []string
	AllClients bool
	Overwrite  bool
	DryRun     bool
	Gitignore  bool
	OnConflict ConflictFunc
}

type Option func(*Options)

func WithBasePath(path string) Option {
	return func(o *Options) { o.BasePath = path }
}

func WithHomeDir(path string) Option {
	return func(o *Options) { o.HomeDir = path }
}

func WithScope(scope string) Option {
	return func(o *Options) { o.Scope = scope }
}

func WithClients(clients []string) Option {
	return func(o *Options) { o.Clients = clients }
}

func WithAllClients(all bool) Option {
	return func(o *Options) { o.AllClients = all }
}

func WithOverwrite(overwrite bool) Option {
	return func(o *Options) { o.Overwrite = overwrite }
}

func WithDryRun(dryRun bool) Option {
	return func(o *Options) { o.DryRun = dryRun }
}

func WithGitignore(gitignore bool) Option {
	return func(o *Options) { o.Gitignore = gitignore }
}

func WithOnConflict(fn ConflictFunc) Option {
	return func(o *Options) { o.OnConflict = fn }
}

type Target struct {
	Client string
	Scope  string
	Path   string
	Root   string
	Format string
}

type Result struct {
	CreatedFiles    []string
	UpdatedFiles    []string
	SkippedServers  []string
	GitignoredFiles []string
	// AddedServers lists server entries that didn't previously exist in a
	// given target and were newly written, formatted "<client>:<name>".
	AddedServers []string
	// UpdatedServers lists server entries that existed with different
	// content and were overwritten (confirmed or --force), formatted
	// "<client>:<name>".
	UpdatedServers []string
	// UnchangedServers lists server entries that already existed with
	// identical content, so nothing was written and no overwrite
	// confirmation was asked for, formatted "<client>:<name>".
	UnchangedServers []string
	// RemovedServers lists servers removed (or, in DryRun mode, that would be
	// removed) by Uninstall, formatted "<client>:<name>".
	RemovedServers []string
	// NotFoundServers lists servers Uninstall was asked to remove that
	// weren't present in a given target, formatted "<client>:<name>". This is
	// distinct from SkippedServers, which means a declined overwrite
	// confirmation during Install.
	NotFoundServers []string
}

type Installer struct {
	opts Options
}

func New(options ...Option) (*Installer, error) {
	opts := Options{
		BasePath: ".",
		Scope:    ScopeProject,
	}
	for _, opt := range options {
		opt(&opts)
	}
	if opts.BasePath == "" {
		opts.BasePath = "."
	}
	if err := ValidateScope(opts.Scope); err != nil {
		return nil, err
	}
	clients, err := normalizeClients(opts.Clients, opts.AllClients)
	if err != nil {
		return nil, err
	}
	opts.Clients = clients
	if opts.HomeDir == "" {
		home, err := homedir.Dir()
		if err != nil {
			return nil, err
		}
		opts.HomeDir = home
	}
	return &Installer{opts: opts}, nil
}

func ValidateScope(scope string) error {
	switch scope {
	case ScopeProject, ScopeUser:
		return nil
	case "system", "managed":
		return fmt.Errorf("%w: %q: %w", errUnsupportedScope, scope, errUnsupportedManagedScope)
	default:
		return fmt.Errorf("%w: %q: expected %q or %q", errUnsupportedScope, scope, ScopeProject, ScopeUser)
	}
}

func normalizeClients(clients []string, all bool) ([]string, error) {
	if all {
		return append([]string(nil), SupportedClients...), nil
	}
	seen := make(map[string]bool, len(clients))
	result := make([]string, 0, len(clients))
	for _, client := range clients {
		normalized := normalizeClient(client)
		if normalized == "" {
			return nil, fmt.Errorf("%w: %q", errUnsupportedClient, client)
		}
		if !seen[normalized] {
			seen[normalized] = true
			result = append(result, normalized)
		}
	}
	sort.Strings(result)
	return result, nil
}

// clientAliases maps every recognized client identifier -- canonical name
// or alias -- to its canonical SupportedClients entry. A registry-style map
// keeps normalizeClient itself a simple O(1) lookup instead of a long
// switch, and keeps each client's aliases declared next to each other.
//
// "github-copilot" alone still means the VS Code extension (it shares VS
// Code's own .vscode/mcp.json). "github-copilot-cli" used to alias onto
// ClientVSCode too, but that collapsed two different products into one
// client; it now maps to ClientCopilotCLI, for the standalone `copilot`
// CLI, which has its own config file.
var clientAliases = map[string]string{
	ClientClaudeCode: ClientClaudeCode,
	"claude":         ClientClaudeCode,

	ClientCursor: ClientCursor,

	ClientVSCode:     ClientVSCode,
	"vs-code":        ClientVSCode,
	"code":           ClientVSCode,
	"github-copilot": ClientVSCode,

	ClientCodex: ClientCodex,
	"codex-cli": ClientCodex,

	ClientGemini: ClientGemini,
	"gemini-cli": ClientGemini,

	ClientClaudeDesktop: ClientClaudeDesktop,
	ClientWindsurf:      ClientWindsurf,
	ClientCline:         ClientCline,
	ClientClineCLI:      ClientClineCLI,
	ClientZed:           ClientZed,

	ClientOpenCode: ClientOpenCode,
	"open-code":    ClientOpenCode,

	ClientGoose: ClientGoose,

	ClientCopilotCLI:     ClientCopilotCLI,
	"github-copilot-cli": ClientCopilotCLI,
	"copilot":            ClientCopilotCLI,

	ClientAntigravity: ClientAntigravity,
	ClientMCPorter:    ClientMCPorter,
}

func normalizeClient(client string) string {
	return clientAliases[strings.ToLower(strings.TrimSpace(client))]
}

func DetectClients(basePath, homeDir, scope string) []string {
	if homeDir == "" {
		home, err := homedir.Dir()
		if err == nil {
			homeDir = home
		}
	}
	var detected []string
	for _, client := range SupportedClients {
		target, err := ResolveTarget(basePath, homeDir, scope, client)
		if err != nil {
			continue
		}
		signalDir := projectSignalDir(basePath, client)
		if scope == ScopeUser {
			signalDir = userSignalDir(homeDir, client)
		}
		if fileOrDirExists(target.Path) || fileOrDirExists(signalDir) {
			detected = append(detected, client)
		}
	}
	return detected
}

// projectSignalDir returns the directory whose existence indicates a client
// is used by this project, at project scope. For most clients this is the
// same directory their config file lives in (.cursor, .vscode, ...); Claude
// Code's config file (.mcp.json) lives at the project root itself, so it
// gets its own well-known directory (.claude) as the signal instead.
func projectSignalDir(basePath, client string) string {
	switch client {
	case ClientClaudeCode:
		return filepath.Join(basePath, ".claude")
	case ClientOpenCode, ClientMCPorter:
		// opencode.json sits directly at the project root and
		// config/mcporter.json's parent ("config/") is too generic a
		// directory name to use as a reliable signal (most repos have
		// unrelated config/ directories) -- unlike clients with a
		// dedicated dotfolder, detection here relies solely on the
		// config file itself existing (see DetectClients's target.Path
		// check), not a signal directory.
		return ""
	default:
		return filepath.Dir(projectPath(basePath, client))
	}
}

// userSignalDir is projectSignalDir's user-scope (global) counterpart.
func userSignalDir(homeDir, client string) string {
	if client == ClientClaudeCode {
		return filepath.Join(homeDir, ".claude")
	}
	return filepath.Dir(userPath(homeDir, client))
}

func fileOrDirExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func (i *Installer) Install(servers map[string]schema.MCPServerConfig) (*Result, error) {
	if len(i.opts.Clients) == 0 {
		return nil, errNoMCPClientsSelected
	}
	if len(servers) == 0 {
		return nil, errNoMCPServersSelected
	}
	result := &Result{}
	targets, err := i.targets()
	if err != nil {
		return result, err
	}
	for _, target := range targets {
		if err := i.installTarget(target, servers, result); err != nil {
			return result, err
		}
	}
	if i.opts.Gitignore && i.opts.Scope == ScopeProject && !i.opts.DryRun {
		if err := i.updateGitignore(targets, result); err != nil {
			return result, err
		}
	}
	return result, nil
}

// Uninstall removes the named servers from each resolved client target.
// Callers are responsible for resolving the server-name list before calling
// (e.g. defaulting to "all currently declared" when no names are given) --
// Uninstall itself just removes whatever names it's given, mirroring
// Install's contract of receiving an already-resolved server map.
func (i *Installer) Uninstall(names []string) (*Result, error) {
	if len(i.opts.Clients) == 0 {
		return nil, errNoMCPClientsSelected
	}
	if len(names) == 0 {
		return nil, errNoMCPServersSelected
	}
	result := &Result{}
	targets, err := i.targets()
	if err != nil {
		return result, err
	}
	for _, target := range targets {
		if err := i.uninstallTarget(target, names, result); err != nil {
			return result, err
		}
	}
	return result, nil
}

func (i *Installer) targets() ([]Target, error) {
	targets := make([]Target, 0, len(i.opts.Clients))
	for _, client := range i.opts.Clients {
		target, err := ResolveTarget(i.opts.BasePath, i.opts.HomeDir, i.opts.Scope, client)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func ResolveTarget(basePath, homeDir, scope, client string) (Target, error) {
	client = normalizeClient(client)
	if client == "" {
		return Target{}, errUnsupportedClient
	}
	if err := ValidateScope(scope); err != nil {
		return Target{}, err
	}
	if scope == ScopeProject && globalOnlyClients[client] {
		return Target{}, fmt.Errorf("%w: %q: %s only supports user-scope (global) installs; use --scope user",
			errUnsupportedScope, scope, client)
	}
	root, format := targetRootAndFormat(client)
	var path string
	if scope == ScopeProject {
		path = projectPath(basePath, client)
	} else {
		path = userPath(homeDir, client)
	}
	return Target{Client: client, Scope: scope, Path: path, Root: root, Format: format}, nil
}

// targetRootAndFormat returns the config root key and file format a client
// uses. Most clients share the "mcpServers"/JSON default; a client with a
// different root key or file format gets its own case here.
func targetRootAndFormat(client string) (root, format string) {
	switch client {
	case ClientVSCode:
		return "servers", "json"
	case ClientCodex:
		return "mcp_servers", "toml"
	case ClientZed:
		return "context_servers", "json"
	case ClientOpenCode:
		return "mcp", "json"
	case ClientGoose:
		return "extensions", "yaml"
	default:
		return "mcpServers", "json"
	}
}

func projectPath(basePath, client string) string {
	switch client {
	case ClientClaudeCode:
		return filepath.Join(basePath, ".mcp.json")
	case ClientCursor:
		return filepath.Join(basePath, ".cursor", mcpJSONFile)
	case ClientVSCode:
		return filepath.Join(basePath, ".vscode", mcpJSONFile)
	case ClientCodex:
		return filepath.Join(basePath, ".codex", "config.toml")
	case ClientGemini:
		return filepath.Join(basePath, ".gemini", "settings.json")
	case ClientZed:
		return filepath.Join(basePath, ".zed", "settings.json")
	case ClientOpenCode:
		return filepath.Join(basePath, "opencode.json")
	case ClientGoose:
		// TODO(verify): Goose's own docs (block.github.io/goose/docs/guides/config-file)
		// describe a single global ~/.config/goose/config.yaml (overridable
		// via the GOOSE_CONFIG env var); a project-local .goose/config.yaml
		// is not confirmed by official docs as something Goose
		// auto-discovers. Kept for parity with user-scope until confirmed
		// against a real Goose install.
		return filepath.Join(basePath, ".goose", "config.yaml")
	case ClientMCPorter:
		return filepath.Join(basePath, "config", "mcporter.json")
	default:
		return ""
	}
}

// userPathResolvers maps each supported client to the function that
// computes its user-scope (global) config path. A registry-style map keeps
// userPath itself a simple O(1) lookup instead of a long switch.
var userPathResolvers = map[string]func(homeDir string) string{
	ClientClaudeCode:    func(homeDir string) string { return filepath.Join(homeDir, ".claude.json") },
	ClientCursor:        func(homeDir string) string { return filepath.Join(homeDir, ".cursor", mcpJSONFile) },
	ClientVSCode:        vscodeUserPath,
	ClientCodex:         func(homeDir string) string { return filepath.Join(homeDir, ".codex", "config.toml") },
	ClientGemini:        func(homeDir string) string { return filepath.Join(homeDir, ".gemini", "settings.json") },
	ClientClaudeDesktop: claudeDesktopUserPath,
	ClientWindsurf:      func(homeDir string) string { return filepath.Join(homeDir, ".codeium", "windsurf", "mcp_config.json") },
	ClientCline:         clineUserPath,
	ClientClineCLI:      clineCLIUserPath,
	ClientZed:           zedUserPath,
	ClientOpenCode:      func(homeDir string) string { return filepath.Join(homeDir, ".config", "opencode", "opencode.json") },
	ClientGoose:         gooseUserPath,
	ClientCopilotCLI:    func(homeDir string) string { return filepath.Join(homeDir, ".copilot", "mcp-config.json") },
	ClientAntigravity: func(homeDir string) string {
		return filepath.Join(homeDir, ".gemini", "antigravity", "mcp_config.json")
	},
	ClientMCPorter: func(homeDir string) string { return filepath.Join(homeDir, ".mcporter", "mcporter.json") },
}

func userPath(homeDir, client string) string {
	resolver, ok := userPathResolvers[client]
	if !ok {
		return ""
	}
	return resolver(homeDir)
}

func clineUserPath(homeDir string) string {
	return filepath.Join(vscodeGlobalStorageDir(homeDir), "saoudrizwan.claude-dev", "settings", "cline_mcp_settings.json")
}

// clineCLIUserPath's path isn't documented in Cline's own docs as of this
// writing.
// TODO(verify): see the still-open, unanswered cline/cline#7249. This is
// the best candidate found during research; confirm against a real Cline
// CLI install before relying on it.
func clineCLIUserPath(homeDir string) string {
	return filepath.Join(homeDir, ".cline", "data", "settings", "cline_mcp_settings.json")
}

// vscodeGlobalStorageDir returns the platform-specific VS Code "User" data
// directory. It backs both VS Code's own user config (vscodeUserPath) and
// VS Code extensions that persist settings under
// User/globalStorage/<extension-id>/... (Cline's cline_mcp_settings.json).
func vscodeGlobalStorageDir(homeDir string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support", "Code", "User")
	case "windows":
		if appData, ok := os.LookupEnv("APPDATA"); ok && appData != "" {
			return filepath.Join(appData, "Code", "User")
		}
		return filepath.Join(homeDir, "AppData", "Roaming", "Code", "User")
	default:
		return filepath.Join(homeDir, ".config", "Code", "User")
	}
}

func vscodeUserPath(homeDir string) string {
	return filepath.Join(vscodeGlobalStorageDir(homeDir), mcpJSONFile)
}

// claudeDesktopUserPath mirrors vscodeUserPath's per-OS pattern for the
// Claude Desktop app's config file.
func claudeDesktopUserPath(homeDir string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	case "windows":
		if appData, ok := os.LookupEnv("APPDATA"); ok && appData != "" {
			return filepath.Join(appData, "Claude", "claude_desktop_config.json")
		}
		return filepath.Join(homeDir, "AppData", "Roaming", "Claude", "claude_desktop_config.json")
	default:
		return filepath.Join(homeDir, ".config", "Claude", "claude_desktop_config.json")
	}
}

// zedUserPath mirrors vscodeUserPath's per-OS pattern for Zed's global
// settings.json. Unlike VS Code and Claude Desktop, Zed uses the
// XDG-style ~/.config/zed path on macOS too, not ~/Library/Application
// Support -- confirmed against Zed's own docs
// (github.com/zed-industries/zed/blob/main/docs/src/configuring-zed.md).
func zedUserPath(homeDir string) string {
	switch runtime.GOOS {
	case "windows":
		if appData, ok := os.LookupEnv("APPDATA"); ok && appData != "" {
			return filepath.Join(appData, "Zed", "settings.json")
		}
		return filepath.Join(homeDir, "AppData", "Roaming", "Zed", "settings.json")
	default:
		return filepath.Join(homeDir, ".config", "zed", "settings.json")
	}
}

// gooseUserPath mirrors vscodeUserPath's per-OS pattern for Goose's global
// config.yaml, per block.github.io/goose/docs/guides/config-file.
func gooseUserPath(homeDir string) string {
	switch runtime.GOOS {
	case "windows":
		if appData, ok := os.LookupEnv("APPDATA"); ok && appData != "" {
			return filepath.Join(appData, "Block", "goose", "config", "config.yaml")
		}
		return filepath.Join(homeDir, "AppData", "Roaming", "Block", "goose", "config", "config.yaml")
	default:
		return filepath.Join(homeDir, ".config", "goose", "config.yaml")
	}
}

func (i *Installer) installTarget(target Target, servers map[string]schema.MCPServerConfig, result *Result) error {
	switch target.Format {
	case "toml":
		return i.installTOMLTarget(target, servers, result)
	case "yaml":
		return i.installYAMLTarget(target, servers, result)
	default:
		return i.installJSONTarget(target, servers, result)
	}
}

// jsonEntryRenderer converts a rendered MCP server entry into the map shape
// a specific target's config format expects, ready to be marshaled by
// mapConfigIO.write. Most clients share the standard .mcp.json shape
// (mcpclient.MCPJSONServer's own JSON/YAML-compatible field names, rendered
// via structToMap) and differ only in root key (handled by Target.Root); a
// client with a materially different per-entry schema gets a dedicated
// renderer, the same way Codex's TOML target already has its own bespoke
// renderTOMLServer.
type jsonEntryRenderer func(srv mcpclient.MCPJSONServer) (map[string]any, error)

// jsonEntryRendererFor returns the entry renderer for client.
func jsonEntryRendererFor(client string) jsonEntryRenderer {
	if client == ClientOpenCode {
		return renderOpenCodeEntry
	}
	return func(srv mcpclient.MCPJSONServer) (map[string]any, error) {
		return structToMap(srv)
	}
}

// mapConfigIO abstracts the read/write mechanics for a config format backed
// by a plain map[string]any root. JSON and YAML both qualify (installTarget
// dispatches to installJSONTarget/installYAMLTarget below, which just wire
// up the right mapConfigIO); TOML's text-block-based rendering is different
// enough that it keeps its own install/uninstallTOMLTarget implementation.
type mapConfigIO struct {
	read  func(path string) (map[string]any, error)
	write func(path string, data map[string]any) error
}

var jsonConfigIO = mapConfigIO{
	read: readJSONConfig,
	write: func(path string, data map[string]any) error {
		buf, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			return err
		}
		return writeConfigFile(path, append(buf, '\n'))
	},
}

var yamlConfigIO = mapConfigIO{
	read:  readYAMLConfig,
	write: writeYAMLConfigFile,
}

func (i *Installer) installJSONTarget(target Target, servers map[string]schema.MCPServerConfig, result *Result) error {
	return i.installMapTarget(target, servers, result, jsonConfigIO, jsonEntryRendererFor(target.Client))
}

func (i *Installer) installYAMLTarget(target Target, servers map[string]schema.MCPServerConfig, result *Result) error {
	return i.installMapTarget(target, servers, result, yamlConfigIO, renderGooseExtension)
}

// installMapTarget is the shared read-merge-write implementation for any
// map[string]any-backed config format (JSON, YAML): read the existing root
// document, merge in each configured server under target.Root via render,
// and write back only if something changed.
func (i *Installer) installMapTarget(
	target Target, servers map[string]schema.MCPServerConfig, result *Result, cfgIO mapConfigIO, render jsonEntryRenderer,
) error {
	existed := fileOrDirExists(target.Path)
	root, err := cfgIO.read(target.Path)
	if err != nil {
		return err
	}
	serverMap, err := extractServerMap(root, target.Root)
	if err != nil {
		return err
	}
	entries := mcpclient.GenerateMCPConfig(servers, "").MCPServers
	changed := false
	for _, name := range sortedEntryNames(entries) {
		entryMap, err := render(entries[name])
		if err != nil {
			return err
		}
		existing, exists := serverMap[name]
		write, err := i.applyConfigEntry(target, name, entryMap, existing, exists, result)
		if err != nil {
			return err
		}
		if !write {
			continue
		}
		serverMap[name] = entryMap
		changed = true
	}
	if !changed || i.opts.DryRun {
		return nil
	}
	root[target.Root] = serverMap
	if err := cfgIO.write(target.Path, root); err != nil {
		return err
	}
	recordFileResult(result, target.Path, existed)
	return nil
}

// applyConfigEntry decides the Added/Updated/Unchanged/Skipped outcome for a
// single server entry and records it on result, returning whether the
// caller should write entryMap into serverMap. Split out of
// installMapTarget to keep that function's cyclomatic complexity down;
// shared by both the JSON and YAML map-backed targets.
func (i *Installer) applyConfigEntry(
	target Target, name string, entryMap, existing any, exists bool, result *Result,
) (bool, error) {
	entryKey := fmt.Sprintf("%s:%s", target.Client, name)
	switch {
	case !exists:
		result.AddedServers = append(result.AddedServers, entryKey)
		return true, nil
	case reflect.DeepEqual(existing, entryMap):
		result.UnchangedServers = append(result.UnchangedServers, entryKey)
		return false, nil
	case i.opts.Overwrite || i.opts.DryRun:
		result.UpdatedServers = append(result.UpdatedServers, entryKey)
		return true, nil
	default:
		overwrite, err := i.confirmConflict(target, name)
		if err != nil {
			return false, err
		}
		if !overwrite {
			result.SkippedServers = append(result.SkippedServers, entryKey)
			return false, nil
		}
		result.UpdatedServers = append(result.UpdatedServers, entryKey)
		return true, nil
	}
}

// readConfigFile is the shared "read a map-backed config file" behavior for
// both JSON and YAML: a missing file yields an empty root (nothing to
// merge), an empty file is treated the same way, and any real parse error
// is wrapped with the file path for context.
func readConfigFile(path string, unmarshal func([]byte, any) error) (map[string]any, error) {
	root := map[string]any{}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return root, nil
	}
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return root, nil
	}
	if err := unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return root, nil
}

func readJSONConfig(path string) (map[string]any, error) {
	return readConfigFile(path, func(data []byte, v any) error { return json.Unmarshal(data, v) })
}

func readYAMLConfig(path string) (map[string]any, error) {
	return readConfigFile(path, func(data []byte, v any) error { return yaml.Unmarshal(data, v) })
}

// writeYAMLConfigFile mirrors writeConfigFile's create-dir + write +
// enforce-permissions behavior, built on utils.WriteToFileAsYAML rather
// than a raw os.WriteFile since the payload here is a Go value, not
// pre-marshaled bytes.
func writeYAMLConfigFile(path string, data map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), configDirMode); err != nil {
		return err
	}
	if err := utils.WriteToFileAsYAML(path, data, configFileMode); err != nil {
		return err
	}
	return os.Chmod(path, configFileMode)
}

// extractServerMap returns the map[string]any stored under key in root
// (empty if absent), used for both JSON and YAML roots -- gopkg.in/yaml.v3
// and encoding/json both decode nested objects into map[string]any here, so
// the same extraction logic applies to either format.
func extractServerMap(root map[string]any, key string) (map[string]any, error) {
	existing, ok := root[key]
	if !ok || existing == nil {
		return map[string]any{}, nil
	}
	servers, ok := existing.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: %q: expected object", errInvalidServersSection, key)
	}
	return servers, nil
}

func structToMap(v any) (map[string]any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// renderOpenCodeEntry converts a server entry into OpenCode's mcp.json
// shape, which differs from the shared .mcp.json format per OpenCode's own
// docs (opencode.ai/docs/mcp-servers, opencode.ai/docs/config): local
// servers use a single "command" array (executable and args combined)
// instead of separate command/args fields, env vars live under
// "environment" rather than "env", and every entry carries an explicit
// "type" ("local"/"remote") and "enabled" flag.
func renderOpenCodeEntry(srv mcpclient.MCPJSONServer) (map[string]any, error) {
	if srv.URL != "" {
		entry := map[string]any{
			"type":    "remote",
			"url":     srv.URL,
			"enabled": true,
		}
		if len(srv.Headers) > 0 {
			entry["headers"] = srv.Headers
		}
		return entry, nil
	}
	entry := map[string]any{
		"type":    "local",
		"command": append([]string{srv.Command}, srv.Args...),
		"enabled": true,
	}
	if len(srv.Env) > 0 {
		entry["environment"] = srv.Env
	}
	return entry, nil
}

// gooseExtension mirrors the entry shape documented for a manually
// configured extension under Goose's "extensions" key in config.yaml
// (block.github.io/goose/docs/guides/config-file). The stdio fields
// (cmd/args/envs) are corroborated by multiple independent sources; the
// remote-extension shape is not -- see the TODO(verify) on URI below.
type gooseExtension struct {
	Type    string            `yaml:"type"`
	Enabled bool              `yaml:"enabled"`
	Cmd     string            `yaml:"cmd,omitempty"`
	Args    []string          `yaml:"args,omitempty"`
	Envs    map[string]string `yaml:"envs,omitempty"`
	// TODO(verify): sources disagree on the remote-extension shape: some
	// describe `type: sse` with a "url" field, others (DeepWiki, more
	// detailed) describe `type: streamable_http` with "uri". Confirm
	// against a real Goose install before relying on remote (HTTP) MCP
	// servers here.
	URI     string            `yaml:"uri,omitempty"`
	Headers map[string]string `yaml:"headers,omitempty"`
}

func renderGooseExtension(srv mcpclient.MCPJSONServer) (map[string]any, error) {
	entry := gooseExtension{Enabled: true}
	if srv.URL != "" {
		entry.Type = "streamable_http"
		entry.URI = srv.URL
		entry.Headers = srv.Headers
	} else {
		entry.Type = "stdio"
		entry.Cmd = srv.Command
		entry.Args = srv.Args
		entry.Envs = srv.Env
	}
	data, err := yaml.Marshal(entry)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (i *Installer) installTOMLTarget(target Target, servers map[string]schema.MCPServerConfig, result *Result) error {
	existed := fileOrDirExists(target.Path)
	data, err := os.ReadFile(target.Path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	content := string(data)
	entries := mcpclient.GenerateMCPConfig(servers, "").MCPServers
	changed := false
	for _, name := range sortedEntryNames(entries) {
		entryKey := fmt.Sprintf("%s:%s", target.Client, name)
		existingBlock, exists := extractTOMLServerBlock(content, name)
		newBlock := strings.TrimSpace(renderTOMLServer(name, entries[name]))
		switch {
		case !exists:
			result.AddedServers = append(result.AddedServers, entryKey)
		case strings.TrimSpace(existingBlock) == newBlock:
			result.UnchangedServers = append(result.UnchangedServers, entryKey)
			continue
		case i.opts.Overwrite || i.opts.DryRun:
			result.UpdatedServers = append(result.UpdatedServers, entryKey)
		default:
			overwrite, err := i.confirmConflict(target, name)
			if err != nil {
				return err
			}
			if !overwrite {
				result.SkippedServers = append(result.SkippedServers, entryKey)
				continue
			}
			result.UpdatedServers = append(result.UpdatedServers, entryKey)
		}
		content = removeTOMLServer(content, name)
		content = strings.TrimRight(content, "\n") + "\n\n" + renderTOMLServer(name, entries[name])
		changed = true
	}
	if !changed || i.opts.DryRun {
		return nil
	}
	if err := writeConfigFile(target.Path, []byte(strings.TrimLeft(content, "\n"))); err != nil {
		return err
	}
	recordFileResult(result, target.Path, existed)
	return nil
}

func (i *Installer) uninstallTarget(target Target, names []string, result *Result) error {
	switch target.Format {
	case "toml":
		return i.uninstallTOMLTarget(target, names, result)
	case "yaml":
		return i.uninstallYAMLTarget(target, names, result)
	default:
		return i.uninstallJSONTarget(target, names, result)
	}
}

func (i *Installer) uninstallJSONTarget(target Target, names []string, result *Result) error {
	return i.uninstallMapTarget(target, names, result, jsonConfigIO)
}

func (i *Installer) uninstallYAMLTarget(target Target, names []string, result *Result) error {
	return i.uninstallMapTarget(target, names, result, yamlConfigIO)
}

// uninstallMapTarget is the shared read-remove-write implementation for any
// map[string]any-backed config format (JSON, YAML); mirrors
// installMapTarget's format-agnostic structure.
func (i *Installer) uninstallMapTarget(target Target, names []string, result *Result, cfgIO mapConfigIO) error {
	if !fileOrDirExists(target.Path) {
		recordNotFound(result, target.Client, names)
		return nil
	}
	root, err := cfgIO.read(target.Path)
	if err != nil {
		return err
	}
	serverMap, err := extractServerMap(root, target.Root)
	if err != nil {
		return err
	}
	changed := false
	for _, name := range names {
		if _, exists := serverMap[name]; !exists {
			result.NotFoundServers = append(result.NotFoundServers, fmt.Sprintf("%s:%s", target.Client, name))
			continue
		}
		delete(serverMap, name)
		result.RemovedServers = append(result.RemovedServers, fmt.Sprintf("%s:%s", target.Client, name))
		changed = true
	}
	if !changed || i.opts.DryRun {
		return nil
	}
	root[target.Root] = serverMap
	if err := cfgIO.write(target.Path, root); err != nil {
		return err
	}
	result.UpdatedFiles = append(result.UpdatedFiles, target.Path)
	return nil
}

func (i *Installer) uninstallTOMLTarget(target Target, names []string, result *Result) error {
	if !fileOrDirExists(target.Path) {
		recordNotFound(result, target.Client, names)
		return nil
	}
	data, err := os.ReadFile(target.Path)
	if err != nil {
		return err
	}
	content := string(data)
	changed := false
	for _, name := range names {
		if !tomlHasServer(content, name) {
			result.NotFoundServers = append(result.NotFoundServers, fmt.Sprintf("%s:%s", target.Client, name))
			continue
		}
		// removeTOMLServer already exists and is exercised by installTOMLTarget
		// when re-rendering an overwritten entry -- reused here unchanged.
		content = removeTOMLServer(content, name)
		result.RemovedServers = append(result.RemovedServers, fmt.Sprintf("%s:%s", target.Client, name))
		changed = true
	}
	if !changed || i.opts.DryRun {
		return nil
	}
	if err := writeConfigFile(target.Path, []byte(strings.TrimLeft(content, "\n"))); err != nil {
		return err
	}
	result.UpdatedFiles = append(result.UpdatedFiles, target.Path)
	return nil
}

func recordNotFound(result *Result, client string, names []string) {
	for _, name := range names {
		result.NotFoundServers = append(result.NotFoundServers, fmt.Sprintf("%s:%s", client, name))
	}
}

func sortedEntryNames(entries map[string]mcpclient.MCPJSONServer) []string {
	names := make([]string, 0, len(entries))
	for name := range entries {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func tomlHasServer(content, name string) bool {
	prefix := "[mcp_servers." + name
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "[mcp_servers."+name+"]" || strings.HasPrefix(trimmed, prefix+".") {
			return true
		}
	}
	return false
}

func removeTOMLServer(content, name string) string {
	lines := strings.Split(content, "\n")
	var kept []string
	skip := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if trimmed == "[mcp_servers."+name+"]" || strings.HasPrefix(trimmed, "[mcp_servers."+name+".") {
				skip = true
				continue
			}
			skip = false
		}
		if !skip {
			kept = append(kept, line)
		}
	}
	return strings.Join(kept, "\n")
}

// extractTOMLServerBlock returns the raw text of the [mcp_servers.<name>]
// block (including any nested [mcp_servers.<name>.*] sub-tables), and
// whether it was found. Used to detect an unchanged entry by comparing
// against a freshly rendered block, mirroring removeTOMLServer's block
// boundary logic.
func extractTOMLServerBlock(content, name string) (string, bool) {
	lines := strings.Split(content, "\n")
	var block []string
	collecting := false
	found := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if trimmed == "[mcp_servers."+name+"]" || strings.HasPrefix(trimmed, "[mcp_servers."+name+".") {
				collecting = true
				found = true
			} else {
				collecting = false
			}
		}
		if collecting {
			block = append(block, line)
		}
	}
	return strings.Join(block, "\n"), found
}

func renderTOMLServer(name string, srv mcpclient.MCPJSONServer) string {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "[mcp_servers.%s]\n", name)
	if srv.URL != "" {
		fmt.Fprintf(&buf, "url = %q\n", srv.URL)
	} else {
		fmt.Fprintf(&buf, "command = %q\n", srv.Command)
		if len(srv.Args) > 0 {
			fmt.Fprintf(&buf, "args = [")
			for i, arg := range srv.Args {
				if i > 0 {
					fmt.Fprint(&buf, ", ")
				}
				fmt.Fprintf(&buf, "%q", arg)
			}
			fmt.Fprint(&buf, "]\n")
		}
	}
	if len(srv.Env) > 0 {
		fmt.Fprintf(&buf, "\n[mcp_servers.%s.env]\n", name)
		writeTOMLMap(&buf, srv.Env)
	}
	if len(srv.Headers) > 0 {
		fmt.Fprintf(&buf, "\n[mcp_servers.%s.http_headers]\n", name)
		writeTOMLMap(&buf, srv.Headers)
	}
	fmt.Fprint(&buf, "\n")
	return buf.String()
}

func writeTOMLMap(buf *bytes.Buffer, values map[string]string) {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Fprintf(buf, "%q = %q\n", k, values[k])
	}
}

func (i *Installer) confirmConflict(target Target, serverName string) (bool, error) {
	if i.opts.OnConflict == nil {
		return false, nil
	}
	return i.opts.OnConflict(target, serverName)
}

func writeConfigFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), configDirMode); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, configFileMode); err != nil {
		return err
	}
	return os.Chmod(path, configFileMode)
}

func recordFileResult(result *Result, path string, existed bool) {
	if existed {
		result.UpdatedFiles = append(result.UpdatedFiles, path)
	} else {
		result.CreatedFiles = append(result.CreatedFiles, path)
	}
}

func (i *Installer) updateGitignore(targets []Target, result *Result) error {
	gitignorePath := filepath.Join(i.opts.BasePath, ".gitignore")
	existing := ""
	if data, err := os.ReadFile(gitignorePath); err == nil {
		existing = string(data)
	} else if !os.IsNotExist(err) {
		return err
	}
	var toAdd []string
	for _, target := range targets {
		rel, err := filepath.Rel(i.opts.BasePath, target.Path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if !gitignoreContains(existing, rel) {
			toAdd = append(toAdd, rel)
		}
	}
	if len(toAdd) == 0 {
		return nil
	}
	sort.Strings(toAdd)
	prefix := ""
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		prefix = "\n"
	}
	updated := existing + prefix + strings.Join(toAdd, "\n") + "\n"
	if err := os.WriteFile(gitignorePath, []byte(updated), 0o644); err != nil {
		return err
	}
	result.GitignoredFiles = append(result.GitignoredFiles, toAdd...)
	return nil
}

func gitignoreContains(content, rel string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == rel {
			return true
		}
	}
	return false
}
