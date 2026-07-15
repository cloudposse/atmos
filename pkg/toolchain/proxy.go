package toolchain

import (
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
)

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
	dir := ProxyDir(config)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create proxy directory: %w", err)
	}
	for name, proxy := range config.Toolchain.Proxies {
		if err := validateProxy(name, proxy); err != nil {
			return err
		}
		link := filepath.Join(dir, proxyFilename(name))
		info, statErr := os.Lstat(link)
		if statErr == nil {
			if runtime.GOOS == "windows" {
				targetInfo, targetErr := os.Stat(target)
				if targetErr == nil && os.SameFile(info, targetInfo) {
					continue
				}
				return fmt.Errorf("toolchain proxy %q already exists and is not managed by Atmos", name)
			}
			if info.Mode()&os.ModeSymlink == 0 {
				return fmt.Errorf("toolchain proxy %q already exists and is not managed by Atmos", name)
			}
			existing, readErr := os.Readlink(link)
			if readErr != nil {
				return fmt.Errorf("read toolchain proxy %q: %w", name, readErr)
			}
			if existing == target {
				continue
			}
			return fmt.Errorf("toolchain proxy %q already exists and points to %q", name, existing)
		}
		if !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("inspect toolchain proxy %q: %w", name, statErr)
		}
		if err := createProxyLink(target, link); err != nil {
			return fmt.Errorf("create toolchain proxy %q: %w", name, err)
		}
	}
	return nil
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
	if name == "atmos" {
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
