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

const mcpServersPathPrefix = "mcp.servers."

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

// ResolveFile picks the atmos.yaml file `add`/`remove` should edit, honoring
// an explicit --config override the same way `atmos config set`/`get` does.
func ResolveFile(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration) (string, error) {
	defer perf.Track(atmosConfig, "mcpconfig.ResolveFile")()

	override := ""
	if cfgFiles, _ := cmd.Flags().GetStringSlice("config"); len(cfgFiles) > 0 {
		override = cfgFiles[0]
	}
	file, err := pkgconfig.ResolveEditableConfigFile(atmosConfig, override)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrInvalidArgumentError).
			WithExplanation(err.Error()).
			WithHint("Run from a directory containing atmos.yaml, or pass --config <file>.").
			Err()
	}
	return file, nil
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

	return atmosyaml.DeleteFile(file, mcpServersPathPrefix+name)
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
