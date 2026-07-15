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
)

var errProxyConfigurationRequired = errors.New("toolchain proxy requires configuration")

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
	installPath := GetInstallPath()
	if config != nil && config.Toolchain.InstallPath != "" {
		installPath = config.Toolchain.InstallPath
	}
	return filepath.Join(installPath, "bin", "proxy")
}

// PrepareProxyEnvironment creates configured proxy links and returns their
// environment without mutating the current process.
func PrepareProxyEnvironment(config *schema.AtmosConfiguration) (ProxyEnvironment, error) {
	if config == nil || len(config.Toolchain.Proxies) == 0 {
		return ProxyEnvironment{}, nil
	}
	if err := SyncProxies(config); err != nil {
		return ProxyEnvironment{}, err
	}
	proxyDir, err := filepath.Abs(ProxyDir(config))
	if err != nil {
		return ProxyEnvironment{}, fmt.Errorf("resolve proxy directory: %w", err)
	}
	versionsFile := config.Toolchain.VersionsFile
	if versionsFile == "" {
		versionsFile = DefaultToolVersionsFilePath
	}
	versionsFile, err = filepath.Abs(versionsFile)
	if err != nil {
		return ProxyEnvironment{}, fmt.Errorf("resolve tool versions file: %w", err)
	}
	configPath := config.CliConfigPath
	if configPath != "" {
		configPath, err = filepath.Abs(configPath)
		if err != nil {
			return ProxyEnvironment{}, fmt.Errorf("resolve proxy config path: %w", err)
		}
	}
	installPath := GetInstallPath()
	if config.Toolchain.InstallPath != "" {
		installPath = config.Toolchain.InstallPath
	}
	installPath, err = filepath.Abs(installPath)
	if err != nil {
		return ProxyEnvironment{}, fmt.Errorf("resolve toolchain install path: %w", err)
	}
	return ProxyEnvironment{Path: proxyDir, ConfigPath: configPath, VersionsFile: versionsFile, InstallPath: installPath}, nil
}

// ApplyProxyEnvironment installs proxy environment variables for child processes.
func ApplyProxyEnvironment(config *schema.AtmosConfiguration) error {
	env, err := PrepareProxyEnvironment(config)
	if err != nil || env.Path == "" {
		return err
	}
	path := os.Getenv("PATH")
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
	if config == nil || len(config.Toolchain.Proxies) == 0 {
		return nil
	}
	target, err := proxyExecutable()
	if err != nil {
		return fmt.Errorf("resolve Atmos executable for proxies: %w", err)
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return fmt.Errorf("resolve absolute Atmos executable path for proxies: %w", err)
	}
	dir := ProxyDir(config)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create proxy directory: %w", err)
	}
	metadata, err := loadProxyMetadata(dir)
	if err != nil {
		return err
	}
	metadataChanged := false
	for name, proxy := range config.Toolchain.Proxies {
		if err := validateProxy(name, proxy); err != nil {
			return err
		}
		link := filepath.Join(dir, proxyFilename(name))
		info, statErr := os.Lstat(link)
		if statErr == nil {
			managed := false
			if _, ok := metadata.Proxies[name]; ok {
				managed = true
			}
			current, currentErr := proxyTargetsExecutable(info, link, target)
			if currentErr != nil {
				return fmt.Errorf("inspect toolchain proxy %q: %w", name, currentErr)
			}
			if current {
				continue
			}
			if !managed {
				return fmt.Errorf("toolchain proxy %q already exists and is not managed by Atmos", name)
			}
			if err := os.Remove(link); err != nil {
				return fmt.Errorf("remove stale toolchain proxy %q: %w", name, err)
			}
			if err := createProxyLink(target, link); err != nil {
				return fmt.Errorf("refresh toolchain proxy %q: %w", name, err)
			}
			continue
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("inspect toolchain proxy %q: %w", name, statErr)
		}
		if err := createProxyLink(target, link); err != nil {
			return fmt.Errorf("create toolchain proxy %q: %w", name, err)
		}
		metadata.Proxies[name] = struct{}{}
		metadataChanged = true
	}
	if metadataChanged {
		if err := writeProxyMetadata(dir, metadata); err != nil {
			return err
		}
	}
	return nil
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
		return fmt.Errorf("invalid toolchain proxy name %q", name)
	}
	if strings.EqualFold(name, "atmos") {
		return fmt.Errorf("toolchain proxy name %q is reserved", name)
	}
	if strings.TrimSpace(proxy.Tool) == "" {
		return fmt.Errorf("toolchain proxy %q requires a tool", name)
	}
	return nil
}

// RunProxy executes a configured proxy. The caller must have initialized the
// toolchain configuration. A child exit status is returned silently.
func RunProxy(config *schema.AtmosConfiguration, name string, args []string) error {
	if config == nil {
		return fmt.Errorf("%w for %q", errProxyConfigurationRequired, name)
	}
	proxy, ok := config.Toolchain.Proxies[name]
	if !ok {
		return fmt.Errorf("toolchain proxy %q is not configured", name)
	}
	versionsPath := os.Getenv(ProxyVersionsFileEnv)
	if versionsPath == "" {
		versionsPath = GetToolVersionsFilePath()
	}
	versions, err := LoadToolVersions(versionsPath)
	if err != nil {
		return fmt.Errorf("load tool versions for proxy %q: %w", name, err)
	}
	installer := NewInstaller()
	owner, repo, err := installer.GetResolver().Resolve(proxy.Tool)
	if err != nil {
		return fmt.Errorf("resolve toolchain proxy %q: %w", name, err)
	}
	version, found := GetDefaultVersion(versions, proxy.Tool)
	if !found {
		version, found = GetDefaultVersion(versions, owner+"/"+repo)
	}
	if !found {
		return fmt.Errorf("toolchain proxy %q requires %q in %s", name, proxy.Tool, versionsPath)
	}
	binary, err := installer.FindBinaryPath(owner, repo, version)
	if err != nil {
		if installErr := RunInstall(proxy.Tool+"@"+version, false, false, false, true); installErr != nil {
			return fmt.Errorf("install toolchain proxy %q: %w", name, installErr)
		}
		binary, err = installer.FindBinaryPath(owner, repo, version)
		if err != nil {
			return fmt.Errorf("find toolchain proxy %q binary: %w", name, err)
		}
	}
	command := exec.Command(binary, append(proxy.Args, args...)...)
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
