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

	"github.com/cloudposse/atmos/pkg/config/homedir"
	mcpclient "github.com/cloudposse/atmos/pkg/mcp/client"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	ScopeProject = "project"
	ScopeUser    = "user"

	ClientClaudeCode = "claude-code"
	ClientCursor     = "cursor"
	ClientVSCode     = "vscode"
	ClientCodex      = "codex"
	ClientGemini     = "gemini"

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

func normalizeClient(client string) string {
	switch strings.ToLower(strings.TrimSpace(client)) {
	case ClientClaudeCode, "claude":
		return ClientClaudeCode
	case ClientCursor:
		return ClientCursor
	case ClientVSCode, "vs-code", "code", "github-copilot", "github-copilot-cli":
		return ClientVSCode
	case ClientCodex, "codex-cli":
		return ClientCodex
	case ClientGemini, "gemini-cli":
		return ClientGemini
	default:
		return ""
	}
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
	if client == ClientClaudeCode {
		return filepath.Join(basePath, ".claude")
	}
	return filepath.Dir(projectPath(basePath, client))
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
	root := "mcpServers"
	format := "json"
	var path string
	if scope == ScopeProject {
		path = projectPath(basePath, client)
	} else {
		path = userPath(homeDir, client)
	}
	if client == ClientVSCode {
		root = "servers"
	}
	if client == ClientCodex {
		root = "mcp_servers"
		format = "toml"
	}
	return Target{Client: client, Scope: scope, Path: path, Root: root, Format: format}, nil
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
	default:
		return ""
	}
}

func userPath(homeDir, client string) string {
	switch client {
	case ClientClaudeCode:
		return filepath.Join(homeDir, ".claude.json")
	case ClientCursor:
		return filepath.Join(homeDir, ".cursor", mcpJSONFile)
	case ClientVSCode:
		return vscodeUserPath(homeDir)
	case ClientCodex:
		return filepath.Join(homeDir, ".codex", "config.toml")
	case ClientGemini:
		return filepath.Join(homeDir, ".gemini", "settings.json")
	default:
		return ""
	}
}

func vscodeUserPath(homeDir string) string {
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(homeDir, "Library", "Application Support", "Code", "User", mcpJSONFile)
	case "windows":
		if appData, ok := os.LookupEnv("APPDATA"); ok && appData != "" {
			return filepath.Join(appData, "Code", "User", mcpJSONFile)
		}
		return filepath.Join(homeDir, "AppData", "Roaming", "Code", "User", mcpJSONFile)
	default:
		return filepath.Join(homeDir, ".config", "Code", "User", mcpJSONFile)
	}
}

func (i *Installer) installTarget(target Target, servers map[string]schema.MCPServerConfig, result *Result) error {
	if target.Format == "toml" {
		return i.installTOMLTarget(target, servers, result)
	}
	return i.installJSONTarget(target, servers, result)
}

func (i *Installer) installJSONTarget(target Target, servers map[string]schema.MCPServerConfig, result *Result) error {
	existed := fileOrDirExists(target.Path)
	root, err := readJSONConfig(target.Path)
	if err != nil {
		return err
	}
	serverMap, err := jsonServerMap(root, target.Root)
	if err != nil {
		return err
	}
	entries := mcpclient.GenerateMCPConfig(servers, "").MCPServers
	changed := false
	for _, name := range sortedEntryNames(entries) {
		entryMap, err := structToMap(entries[name])
		if err != nil {
			return err
		}
		existing, exists := serverMap[name]
		write, err := i.applyJSONEntry(target, name, entryMap, existing, exists, result)
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
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := writeConfigFile(target.Path, append(data, '\n')); err != nil {
		return err
	}
	recordFileResult(result, target.Path, existed)
	return nil
}

// applyJSONEntry decides the Added/Updated/Unchanged/Skipped outcome for a
// single server entry and records it on result, returning whether the
// caller should write entryMap into serverMap. Split out of
// installJSONTarget to keep that function's cyclomatic complexity down.
func (i *Installer) applyJSONEntry(
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

func readJSONConfig(path string) (map[string]any, error) {
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
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return root, nil
}

func jsonServerMap(root map[string]any, key string) (map[string]any, error) {
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
	if target.Format == "toml" {
		return i.uninstallTOMLTarget(target, names, result)
	}
	return i.uninstallJSONTarget(target, names, result)
}

func (i *Installer) uninstallJSONTarget(target Target, names []string, result *Result) error {
	if !fileOrDirExists(target.Path) {
		recordNotFound(result, target.Client, names)
		return nil
	}
	root, err := readJSONConfig(target.Path)
	if err != nil {
		return err
	}
	serverMap, err := jsonServerMap(root, target.Root)
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
	data, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := writeConfigFile(target.Path, append(data, '\n')); err != nil {
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
