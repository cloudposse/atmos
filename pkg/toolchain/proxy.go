package toolchain

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// ProxyConfigPathEnv preserves the configuration that declared a proxy for
	// child processes which change their working directory before invoking it.
	ProxyConfigPathEnv   = "ATMOS_TOOLCHAIN_PROXY_CONFIG_PATH"
	ProxyVersionsFileEnv = "ATMOS_TOOLCHAIN_PROXY_VERSIONS_FILE"
	ProxyInstallPathEnv  = "ATMOS_TOOLCHAIN_PROXY_INSTALL_PATH"
	proxyMetadataFile    = ".atmos-proxies.json"
	proxyFilePermissions = 0o600
	// ProxyDirPermissions is the file mode used when creating the directory
	// that holds generated command proxies.
	proxyDirPermissions = 0o755
	// Format string for wrapping a proxy-name sentinel error with the offending name.
	proxyNameErrorFormat = "%w: %q"
)

var (
	errProxyConfigurationRequired = errors.New("toolchain proxy requires configuration")
	errProxyAlreadyExists         = errors.New("toolchain proxy already exists and is not managed by Atmos")
	errProxyInvalidName           = errors.New("invalid toolchain proxy name")
	errProxyNameReserved          = errors.New("toolchain proxy name is reserved")
	errProxyRequiresTool          = errors.New("toolchain proxy requires a tool")
	errProxyNotConfigured         = errors.New("toolchain proxy is not configured")
	errProxyRequiresVersion       = errors.New("toolchain proxy requires a default version in the tool-versions file")
)

// proxyMetadata records proxy entries created by Atmos. It lets SyncProxies
// safely refresh stale links after the Atmos binary changes without replacing
// an unrelated file a user placed in the proxy directory.
type proxyMetadata struct {
	Proxies map[string]struct{} `json:"proxies"`
}

// proxyExecutable is replaceable in tests so proxy links never need to target
// the running test binary. Windows cannot remove a hard link to that binary.
var proxyExecutable = os.Executable

// ProxyEnvironment is the PATH entry and private context required by a proxy.
type ProxyEnvironment struct {
	Path         string
	ConfigPath   string
	VersionsFile string
	InstallPath  string
}

// ProxyDir returns the directory containing generated command proxies.
func ProxyDir(config *schema.AtmosConfiguration) string {
	defer perf.Track(nil, "toolchain.ProxyDir")()

	installPath := GetInstallPath()
	if config != nil && config.Toolchain.InstallPath != "" {
		installPath = config.Toolchain.InstallPath
	}
	return filepath.Join(installPath, "bin", "proxy")
}

// PrepareProxyEnvironment creates configured proxy links and returns their
// environment without mutating the current process.
func PrepareProxyEnvironment(config *schema.AtmosConfiguration) (ProxyEnvironment, error) {
	defer perf.Track(nil, "toolchain.PrepareProxyEnvironment")()

	if config == nil || len(config.Toolchain.Proxies) == 0 {
		return ProxyEnvironment{}, nil
	}
	if err := SyncProxies(config); err != nil {
		return ProxyEnvironment{}, err
	}
	proxyDir, err := resolveAbsPath(ProxyDir(config), "proxy directory")
	if err != nil {
		return ProxyEnvironment{}, err
	}
	versionsFile, err := resolveAbsPath(resolveVersionsFilePath(config), "tool versions file")
	if err != nil {
		return ProxyEnvironment{}, err
	}
	configPath, err := resolveProxyConfigPath(config)
	if err != nil {
		return ProxyEnvironment{}, err
	}
	installPath, err := resolveAbsPath(resolveInstallPath(config), "toolchain install path")
	if err != nil {
		return ProxyEnvironment{}, err
	}
	return ProxyEnvironment{Path: proxyDir, ConfigPath: configPath, VersionsFile: versionsFile, InstallPath: installPath}, nil
}

// resolveAbsPath resolves path to an absolute path, wrapping any error with
// context describing what the path represents.
func resolveAbsPath(path, context string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", context, err)
	}
	return abs, nil
}

// resolveVersionsFilePath returns the configured tool-versions file, falling
// back to the default path when unset.
func resolveVersionsFilePath(config *schema.AtmosConfiguration) string {
	if config.Toolchain.VersionsFile != "" {
		return config.Toolchain.VersionsFile
	}
	return DefaultToolVersionsFilePath
}

// resolveInstallPath returns the configured toolchain install path, falling
// back to the default install path when unset.
func resolveInstallPath(config *schema.AtmosConfiguration) string {
	if config.Toolchain.InstallPath != "" {
		return config.Toolchain.InstallPath
	}
	return GetInstallPath()
}

// resolveProxyConfigPath resolves the absolute CLI config path, returning an
// empty string (not an error) when none was set.
func resolveProxyConfigPath(config *schema.AtmosConfiguration) (string, error) {
	if config.CliConfigPath == "" {
		return "", nil
	}
	return resolveAbsPath(config.CliConfigPath, "proxy config path")
}

// ApplyProxyEnvironment installs proxy environment variables for child processes.
func ApplyProxyEnvironment(config *schema.AtmosConfiguration) error {
	defer perf.Track(nil, "toolchain.ApplyProxyEnvironment")()

	env, err := PrepareProxyEnvironment(config)
	if err != nil || env.Path == "" {
		return err
	}
	// PATH is a shell-managed environment variable reflecting the live
	// process environment, not Atmos configuration; os.LookupEnv reads it
	// directly rather than going through Viper's config/flag/env precedence.
	path, _ := os.LookupEnv("PATH")
	if !strings.HasPrefix(path, env.Path+string(os.PathListSeparator)) && path != env.Path {
		if err := os.Setenv("PATH", env.Path+string(os.PathListSeparator)+path); err != nil {
			return err
		}
	}
	for key, value := range env.Variables() {
		if value != "" {
			if err := os.Setenv(key, value); err != nil {
				return err
			}
		}
	}
	return nil
}

// Variables returns the private context exports for a prepared proxy environment.
func (e ProxyEnvironment) Variables() map[string]string {
	defer perf.Track(nil, "toolchain.ProxyEnvironment.Variables")()

	return map[string]string{
		ProxyConfigPathEnv:   e.ConfigPath,
		ProxyVersionsFileEnv: e.VersionsFile,
		ProxyInstallPathEnv:  e.InstallPath,
	}
}

// SyncProxies creates a link per configured proxy. Existing links are left
// untouched unless they already target this executable, preventing accidental
// replacement of a user-managed command.
func SyncProxies(config *schema.AtmosConfiguration) error {
	defer perf.Track(nil, "toolchain.SyncProxies")()

	if config == nil || len(config.Toolchain.Proxies) == 0 {
		return nil
	}
	target, err := resolveProxyExecutableTarget()
	if err != nil {
		return err
	}
	dir := ProxyDir(config)
	if err := os.MkdirAll(dir, proxyDirPermissions); err != nil {
		return fmt.Errorf("create proxy directory: %w", err)
	}
	metadata, err := loadProxyMetadata(dir)
	if err != nil {
		return err
	}

	metadataChanged, err := syncAllProxyLinks(dir, target, config.Toolchain.Proxies, &metadata)
	if err != nil {
		return err
	}
	if !metadataChanged {
		return nil
	}
	return writeProxyMetadata(dir, metadata)
}

// syncAllProxyLinks validates and syncs every configured proxy's link,
// returning whether the proxy metadata registry (the managed-proxy list) was
// updated and needs to be persisted.
func syncAllProxyLinks(dir, target string, proxies map[string]schema.ToolchainProxy, metadata *proxyMetadata) (bool, error) {
	metadataChanged := false
	for name, proxy := range proxies {
		if err := validateProxy(name, proxy); err != nil {
			return false, err
		}
		changed, err := syncProxyLink(dir, name, target, metadata)
		if err != nil {
			return false, err
		}
		if changed {
			metadataChanged = true
		}
	}
	return metadataChanged, nil
}

// resolveProxyExecutableTarget resolves the absolute path of the running
// Atmos executable, which every proxy link points at.
func resolveProxyExecutableTarget() (string, error) {
	target, err := proxyExecutable()
	if err != nil {
		return "", fmt.Errorf("resolve Atmos executable for proxies: %w", err)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve absolute Atmos executable path for proxies: %w", err)
	}
	return target, nil
}

// syncProxyLink ensures a single proxy link exists and targets the current
// executable, creating or refreshing it as needed. It returns whether the
// proxy metadata (the managed-proxy registry) was updated.
func syncProxyLink(dir, name, target string, metadata *proxyMetadata) (bool, error) {
	link := filepath.Join(dir, proxyFilename(name))
	info, statErr := os.Lstat(link)
	if statErr == nil {
		return refreshExistingProxyLink(name, link, target, info, metadata)
	}
	if !errors.Is(statErr, os.ErrNotExist) {
		return false, fmt.Errorf("inspect toolchain proxy %q: %w", name, statErr)
	}
	if err := createProxyLink(target, link); err != nil {
		return false, fmt.Errorf("create toolchain proxy %q: %w", name, err)
	}
	metadata.Proxies[name] = struct{}{}
	return true, nil
}

// refreshExistingProxyLink inspects an existing proxy link, replacing it when
// it is stale (pointing at an old Atmos executable) and Atmos-managed. It
// refuses to touch a link Atmos did not create.
func refreshExistingProxyLink(name, link, target string, info os.FileInfo, metadata *proxyMetadata) (bool, error) {
	_, managed := metadata.Proxies[name]
	current, err := proxyTargetsExecutable(info, link, target)
	if err != nil {
		return false, fmt.Errorf("inspect toolchain proxy %q: %w", name, err)
	}
	if current {
		return false, nil
	}
	if !managed {
		return false, fmt.Errorf(proxyNameErrorFormat, errProxyAlreadyExists, name)
	}
	if err := os.Remove(link); err != nil {
		return false, fmt.Errorf("remove stale toolchain proxy %q: %w", name, err)
	}
	if err := createProxyLink(target, link); err != nil {
		return false, fmt.Errorf("refresh toolchain proxy %q: %w", name, err)
	}
	return false, nil
}

func proxyMetadataPath(dir string) string {
	return filepath.Join(dir, proxyMetadataFile)
}

func loadProxyMetadata(dir string) (proxyMetadata, error) {
	metadata := proxyMetadata{Proxies: map[string]struct{}{}}
	data, err := os.ReadFile(proxyMetadataPath(dir))
	if errors.Is(err, os.ErrNotExist) {
		return metadata, nil
	}
	if err != nil {
		return proxyMetadata{}, fmt.Errorf("read toolchain proxy metadata: %w", err)
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		return proxyMetadata{}, fmt.Errorf("parse toolchain proxy metadata: %w", err)
	}
	if metadata.Proxies == nil {
		metadata.Proxies = map[string]struct{}{}
	}
	return metadata, nil
}

func writeProxyMetadata(dir string, metadata proxyMetadata) error {
	data, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal toolchain proxy metadata: %w", err)
	}
	path := proxyMetadataPath(dir)
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, proxyFilePermissions); err != nil {
		return fmt.Errorf("write toolchain proxy metadata: %w", err)
	}
	if err := os.Rename(temporary, path); err != nil {
		return fmt.Errorf("commit toolchain proxy metadata: %w", err)
	}
	return nil
}

func proxyTargetsExecutable(info os.FileInfo, link, target string) (bool, error) {
	if runtime.GOOS == "windows" {
		targetInfo, err := os.Stat(target)
		if err != nil {
			return false, err
		}
		return os.SameFile(info, targetInfo), nil
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return false, nil
	}
	existing, err := os.Readlink(link)
	if err != nil {
		return false, err
	}
	if !filepath.IsAbs(existing) {
		existing = filepath.Join(filepath.Dir(link), existing)
	}
	existing, err = filepath.Abs(existing)
	if err != nil {
		return false, err
	}
	return existing == target, nil
}

func proxyFilename(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

func createProxyLink(target, link string) error {
	if runtime.GOOS == "windows" {
		// A hard link is executable without the symlink privilege required by
		// many Windows developer environments, mirroring Aqua's proxy strategy.
		return os.Link(target, link)
	}
	return os.Symlink(target, link)
}

func validateProxy(name string, proxy schema.ToolchainProxy) error {
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `\\/`) {
		return fmt.Errorf(proxyNameErrorFormat, errProxyInvalidName, name)
	}
	if strings.EqualFold(name, "atmos") {
		return fmt.Errorf(proxyNameErrorFormat, errProxyNameReserved, name)
	}
	if strings.TrimSpace(proxy.Tool) == "" {
		return fmt.Errorf(proxyNameErrorFormat, errProxyRequiresTool, name)
	}
	return nil
}

// RunProxy executes a configured proxy. The caller must have initialized the
// toolchain configuration. A child exit status is returned silently.
func RunProxy(config *schema.AtmosConfiguration, name string, args []string) error {
	defer perf.Track(nil, "toolchain.RunProxy")()

	if config == nil {
		return fmt.Errorf("%w for %q", errProxyConfigurationRequired, name)
	}
	proxy, ok := config.Toolchain.Proxies[name]
	if !ok {
		return fmt.Errorf(proxyNameErrorFormat, errProxyNotConfigured, name)
	}
	binary, err := resolveProxyBinary(name, proxy)
	if err != nil {
		return err
	}
	return execProxyBinary(binary, proxy.Args, args)
}

// proxyVersionsFilePath resolves the .tool-versions file used to pick a
// proxy's pinned version. A child process launched through a proxy link
// inherits ProxyVersionsFileEnv from ApplyProxyEnvironment so it resolves
// tool versions consistently with the parent Atmos invocation; it is a
// private inter-process context variable, not user-facing configuration, so
// it is read directly rather than through Viper.
func proxyVersionsFilePath() string {
	if versionsPath, ok := os.LookupEnv(ProxyVersionsFileEnv); ok && versionsPath != "" {
		return versionsPath
	}
	return GetToolVersionsFilePath()
}

// resolveProxyBinary resolves (installing on demand if necessary) the binary
// path for a configured proxy's tool at its pinned .tool-versions version.
func resolveProxyBinary(name string, proxy schema.ToolchainProxy) (string, error) {
	versionsPath := proxyVersionsFilePath()
	versions, err := LoadToolVersions(versionsPath)
	if err != nil {
		return "", fmt.Errorf("load tool versions for proxy %q: %w", name, err)
	}
	installer := NewInstaller()
	owner, repo, err := installer.GetResolver().Resolve(proxy.Tool)
	if err != nil {
		return "", fmt.Errorf("resolve toolchain proxy %q: %w", name, err)
	}
	version, found := GetDefaultVersion(versions, proxy.Tool)
	if !found {
		version, found = GetDefaultVersion(versions, owner+"/"+repo)
	}
	if !found {
		return "", fmt.Errorf("%w: %q requires %q in %s", errProxyRequiresVersion, name, proxy.Tool, versionsPath)
	}
	binary, err := installer.FindBinaryPath(owner, repo, version)
	if err == nil {
		return binary, nil
	}
	if installErr := RunInstall(proxy.Tool+"@"+version, false, false, false, true); installErr != nil {
		return "", fmt.Errorf("install toolchain proxy %q: %w", name, installErr)
	}
	binary, err = installer.FindBinaryPath(owner, repo, version)
	if err != nil {
		return "", fmt.Errorf("find toolchain proxy %q binary: %w", name, err)
	}
	return binary, nil
}

// execProxyBinary execs the resolved tool binary with the proxy's configured
// args followed by the caller's args, forwarding stdio and translating a
// nonzero exit into a silent ExitCodeError so the parent process's exit code
// matches the proxied tool's.
func execProxyBinary(binary string, proxyArgs, args []string) error {
	// #nosec G204 -- binary is resolved internally via installer.FindBinaryPath
	// from the tool this proxy is configured for and the version pinned in the
	// .tool-versions file, never taken directly from untrusted input; the same
	// pattern is used for RunExecCommand in exec.go.
	command := exec.Command(binary, append(proxyArgs, args...)...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return errUtils.ExitCodeError{Code: exitErr.ExitCode(), Silent: true}
		}
		return err
	}
	return nil
}
