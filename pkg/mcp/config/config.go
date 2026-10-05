// Package config parses `atmos mcp add` inputs into schema.MCPServerConfig
// values and reads/writes them under mcp.servers in atmos.yaml, backing the
// `atmos mcp add`/`remove` commands.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	pkgconfig "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

const (
	mcpServersPathPrefix = "mcp.servers."
	// The mcpServersPath and mcpEnabledPath constants are the dotted config paths
	// used to locate which file owns the effective MCP config so edits land where
	// they take effect rather than in a shadowed file (cloudposse/atmos#3269).
	mcpServersPath = "mcp.servers"
	mcpEnabledPath = "mcp.enabled"
)

var (
	errEmptyTarget          = errors.New("target must not be empty")
	errUnsupportedTransport = errors.New("unsupported MCP transport")
	errInvalidKeyValuePair  = errors.New("invalid KEY=VALUE pair")
	errInvalidHeaderPair    = errors.New(`invalid header, expected "Key: Value"`)
	errInvalidServerName    = errors.New("invalid server name")
)

// ServerSpec holds the `atmos mcp add` flag inputs used to build a
// schema.MCPServerConfig via ParseServerSpec. Grouped into a struct per the
// Options pattern rather than passed as positional parameters, since the
// full flag surface (name, transport, env, headers, description, identity,
// timeout, auto-start) is more than a handful of arguments.
type ServerSpec struct {
	Target      string
	Name        string
	Transport   string
	Description string
	Identity    string
	Timeout     string
	Env         []string
	Headers     []string
	AutoStart   bool
}

// ParseServerSpec builds a schema.MCPServerConfig from `atmos mcp add`
// inputs, inferring a name when spec.Name is empty. The spec.Target field may
// be a known preset name (see ResolvePreset), an http(s) URL, or a stdio
// command (optionally with arguments, e.g. "npx -y @org/mcp-server --flag
// value").
func ParseServerSpec(atmosConfig *schema.AtmosConfiguration, spec ServerSpec) (string, schema.MCPServerConfig, error) { //nolint:gocritic // hugeParam: spec is a read-only options struct.
	defer perf.Track(atmosConfig, "mcpconfig.ParseServerSpec")()

	if name, cfg, ok := resolvePresetSpec(atmosConfig, spec); ok {
		return name, cfg, nil
	}

	if strings.TrimSpace(spec.Target) == "" {
		return "", schema.MCPServerConfig{}, errEmptyTarget
	}
	if err := validateTransport(spec.Transport); err != nil {
		return "", schema.MCPServerConfig{}, err
	}

	envMap, err := ParseKeyValuePairs(spec.Env)
	if err != nil {
		return "", schema.MCPServerConfig{}, err
	}
	headerMap, err := ParseHeaderPairs(spec.Headers)
	if err != nil {
		return "", schema.MCPServerConfig{}, err
	}

	cfg := schema.MCPServerConfig{
		Env:         envMap,
		Headers:     headerMap,
		Description: spec.Description,
		Identity:    spec.Identity,
		Timeout:     spec.Timeout,
		AutoStart:   spec.AutoStart,
	}

	if isURL(spec.Target) {
		cfg.Type = schema.MCPTransportHTTP
		cfg.URL = spec.Target
	} else {
		fields := strings.Fields(spec.Target)
		cfg.Command = fields[0]
		if len(fields) > 1 {
			cfg.Args = fields[1:]
		}
	}

	name := spec.Name
	if name == "" {
		name = InferName(spec.Target)
	}
	if !isValidServerName(name) {
		return "", schema.MCPServerConfig{}, fmt.Errorf("%w: %q: pass --name explicitly", errInvalidServerName, name)
	}

	return name, cfg, nil
}

// resolvePresetSpec resolves spec.Target against the built-in preset
// registry, applying a --name override if given, and reports false when
// spec.Target isn't a known preset name -- signaling ParseServerSpec to fall
// through to URL/command parsing instead.
func resolvePresetSpec(atmosConfig *schema.AtmosConfiguration, spec ServerSpec) (name string, cfg schema.MCPServerConfig, ok bool) { //nolint:gocritic // hugeParam: spec is a read-only options struct.
	preset, found := ResolvePreset(spec.Target)
	if !found {
		return "", schema.MCPServerConfig{}, false
	}
	name = preset.DefaultServerName
	if spec.Name != "" {
		name = spec.Name
	}
	return name, preset.Resolve(atmosConfig), true
}

// validateTransport rejects transports Atmos's MCP schema doesn't support yet:
// only "stdio" and "http" are recognized by MCPServerConfig.TransportType(),
// so passing e.g. "sse" through unchecked would silently produce a broken
// client entry with an empty Command.
func validateTransport(transport string) error {
	switch transport {
	case "", schema.MCPTransportHTTP, schema.MCPTransportStdio:
		return nil
	default:
		return fmt.Errorf("%w: %q: not yet supported, use %q", errUnsupportedTransport, transport, schema.MCPTransportHTTP)
	}
}

// InferName derives a server name from a URL or command string, sanitized to
// the [A-Za-z0-9_-]+ charset atmos.yaml keys and MCP client configs expect.
func InferName(target string) string {
	if isURL(target) {
		return inferNameFromURL(target)
	}
	return inferNameFromCommand(target)
}

func inferNameFromURL(target string) string {
	parsed, err := url.Parse(target)
	if err != nil {
		return sanitizeName(target)
	}
	path := strings.Trim(parsed.Path, "/")
	if path == "" {
		return sanitizeName(parsed.Host)
	}
	segments := strings.Split(path, "/")
	return sanitizeName(segments[len(segments)-1])
}

// inferNameFromCommand prefers the first non-flag argument after the command
// (e.g. the package name in "npx -y @org/mcp-server" or
// "uvx awslabs.aws-docs@latest"), falling back to the command itself for bare
// commands. Only the first match is taken -- anything after it may be a flag's
// value (e.g. "--flag value"), not a second package identifier.
func inferNameFromCommand(target string) string {
	fields := strings.Fields(target)
	if len(fields) == 0 {
		return ""
	}
	candidate := fields[0]
	for _, field := range fields[1:] {
		if strings.HasPrefix(field, "-") {
			continue
		}
		candidate = field
		break
	}
	candidate = strings.TrimPrefix(candidate, "@")
	if idx := strings.LastIndex(candidate, "/"); idx >= 0 {
		candidate = candidate[idx+1:]
	}
	if idx := strings.LastIndex(candidate, "@"); idx > 0 {
		candidate = candidate[:idx]
	}
	return sanitizeName(candidate)
}

func sanitizeName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func isValidServerName(name string) bool {
	return name != "" && sanitizeName(name) == name
}

func isURL(target string) bool {
	return strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://")
}

// ParseKeyValuePairs parses repeatable KEY=VALUE flags (env) into a map.
func ParseKeyValuePairs(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	result := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		key, value, ok := strings.Cut(pair, "=")
		if !ok || key == "" {
			return nil, fmt.Errorf("%w: %q, expected KEY=VALUE", errInvalidKeyValuePair, pair)
		}
		result[key] = value
	}
	return result, nil
}

// ParseHeaderPairs parses repeatable "Key: Value" flags into a map.
func ParseHeaderPairs(pairs []string) (map[string]string, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	result := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		key, value, ok := strings.Cut(pair, ":")
		if !ok || strings.TrimSpace(key) == "" {
			return nil, fmt.Errorf("%w: %q", errInvalidHeaderPair, pair)
		}
		result[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return result, nil
}

// ResolveServerFile picks the file that `add`/`remove` should edit for
// mcp.servers.<name>, honoring an explicit --config override the same way
// `atmos config set` does. Without an override it selects by the server's
// effective provenance so an edit is never silently shadowed by a
// higher-precedence file (cloudposse/atmos#3269). Precedence:
//
//  1. an explicit --config override;
//  2. the highest-precedence effective file that already declares the server
//     (so an overwrite or remove targets the value that is actually in effect);
//  3. for a new server, the highest-precedence effective file that already
//     declares mcp.servers (joining existing servers, typically a .atmos.d
//     fragment);
//  4. the root atmos.yaml.
//
// The declared return reports whether the server already exists in the chosen
// file (case 1 or 2), which `remove` uses to distinguish "edit here" from "not
// configured".
func ResolveServerFile(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration, name string) (file string, declared bool, err error) {
	defer perf.Track(atmosConfig, "mcpconfig.ResolveServerFile")()

	override, err := resolveConfigOverride(cmd)
	if err != nil {
		return "", false, err
	}
	if override != "" {
		resolved, rerr := resolveOverrideFile(atmosConfig, override)
		if rerr != nil {
			return "", false, rerr
		}
		return resolved, fileDeclaresKey(resolved, serverPath(name)), nil
	}

	candidates := pkgconfig.EffectiveConfigFilesAscending(atmosConfig)
	if owner := highestPrecedenceDeclaring(candidates, serverPath(name)); owner != "" {
		return owner, true, nil
	}
	if holder := highestPrecedenceDeclaring(candidates, mcpServersPath); holder != "" {
		return holder, false, nil
	}
	root, rerr := resolveRootFile(atmosConfig)
	if rerr != nil {
		return "", false, rerr
	}
	return root, false, nil
}

// ResolveEnableFile picks the file to write mcp.enabled to. Because the root
// atmos.yaml is reapplied after its imports, an mcp.enabled written to a fragment
// is shadowed by an explicit root value; so this targets the highest-precedence
// file that already declares mcp.enabled, else the root atmos.yaml, which always
// takes effect (cloudposse/atmos#3269).
func ResolveEnableFile(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration) (string, error) {
	defer perf.Track(atmosConfig, "mcpconfig.ResolveEnableFile")()

	override, err := resolveConfigOverride(cmd)
	if err != nil {
		return "", err
	}
	if override != "" {
		return resolveOverrideFile(atmosConfig, override)
	}
	candidates := pkgconfig.EffectiveConfigFilesAscending(atmosConfig)
	if owner := highestPrecedenceDeclaring(candidates, mcpEnabledPath); owner != "" {
		return owner, nil
	}
	return resolveRootFile(atmosConfig)
}

// resolveConfigOverride extracts and validates a single --config override.
func resolveConfigOverride(cmd *cobra.Command) (string, error) {
	cfgFiles, _ := cmd.Flags().GetStringSlice("config")
	override, err := pkgconfig.ResolveConfigOverride(cfgFiles)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrInvalidArgumentError).
			WithExplanation(err.Error()).
			WithHint("Pass a single --config file, or edit the target file directly.").
			Err()
	}
	return override, nil
}

func resolveOverrideFile(atmosConfig *schema.AtmosConfiguration, override string) (string, error) {
	return wrapResolve(pkgconfig.ResolveEditableConfigFile(atmosConfig, override))
}

func resolveRootFile(atmosConfig *schema.AtmosConfiguration) (string, error) {
	return wrapResolve(pkgconfig.ResolveEditableConfigFile(atmosConfig, ""))
}

func wrapResolve(file string, err error) (string, error) {
	if err != nil {
		return "", errUtils.Build(errUtils.ErrInvalidArgumentError).
			WithExplanation(err.Error()).
			WithHint("Run from a directory containing atmos.yaml, or pass --config <file>.").
			Err()
	}
	return file, nil
}

// serverPath returns the dotted config path for a server entry.
func serverPath(name string) string {
	return mcpServersPathPrefix + name
}

// fileDeclaresKey reports whether file declares the dotted keyPath.
func fileDeclaresKey(file, keyPath string) bool {
	_, err := atmosyaml.GetFile(file, keyPath)
	return err == nil
}

// highestPrecedenceDeclaring returns the last (highest-precedence) file in
// candidates that declares keyPath, or "" when none do. The candidates must be
// in ascending precedence order (see config.EffectiveConfigFilesAscending).
func highestPrecedenceDeclaring(candidates []string, keyPath string) string {
	target := ""
	for _, f := range candidates {
		if fileDeclaresKey(f, keyPath) {
			target = f
		}
	}
	return target
}

// Write serializes cfg to a compact JSON literal (valid YAML flow syntax) and
// writes it at mcp.servers.<name> in file as a single atomic subtree replace
// -- avoids leaving stale fields behind if a server's shape changes (e.g.
// stdio to http) on overwrite, and preserves comments/formatting.
func Write(file, name string, cfg schema.MCPServerConfig) error { //nolint:gocritic // hugeParam: cfg is read-only config value.
	defer perf.Track(nil, "mcpconfig.Write")()

	data, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	return atmosyaml.SetFileRaw(file, mcpServersPathPrefix+name, string(data))
}

// Remove deletes mcp.servers.<name> from file.
func Remove(file, name string) error {
	defer perf.Track(nil, "mcpconfig.Remove")()

	_, err := atmosyaml.DeleteFile(file, mcpServersPathPrefix+name)
	return err
}

// Exists reports whether mcp.servers.<name> is already present in file.
func Exists(file, name string) (bool, error) {
	defer perf.Track(nil, "mcpconfig.Exists")()

	_, err := atmosyaml.GetFile(file, mcpServersPathPrefix+name)
	if err != nil {
		if errors.Is(err, atmosyaml.ErrYAMLPathNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// HasServerWithURL reports whether any server in servers resolves to url --
// used by the Atmos Pro nudge, matched by URL value rather than conventional
// key name so a renamed entry (via --name) doesn't trigger a false nudge.
func HasServerWithURL(servers map[string]schema.MCPServerConfig, url string) bool {
	for _, server := range servers { //nolint:gocritic // rangeValCopy: map values are read-only here, not worth restructuring.
		if server.URL == url {
			return true
		}
	}
	return false
}
