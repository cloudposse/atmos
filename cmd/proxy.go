package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// TryRunToolchainProxy recognizes an invocation through a generated proxy
// link. It returns false when argv[0] is the normal Atmos executable or no
// matching proxy is configured.
func TryRunToolchainProxy(argv []string) (bool, error) {
	name, ok := proxyNameFromArgv(argv)
	if !ok {
		return false, nil
	}
	configInfo := proxyConfigSelection()
	atmosConfig, err := cfg.InitCliConfig(configInfo, false)
	if err != nil {
		// A proxy context proves this executable was intentionally invoked
		// through a generated link, so surface its configuration error rather
		// than falling through to an unrelated Atmos subcommand error.
		return proxyConfigContextDetected(), err
	}
	if _, ok := atmosConfig.Toolchain.Proxies[name]; !ok {
		return false, nil
	}
	applyProxyEnvOverrides(&atmosConfig)
	toolchain.SetAtmosConfig(&atmosConfig)
	return true, toolchain.RunProxy(&atmosConfig, name, argv[1:])
}

// proxyNameFromArgv extracts the invoked command name from argv[0] (stripping
// a Windows .exe suffix), reporting false when the invocation doesn't look
// like a proxy call: no args, the Atmos executable itself, or an empty name.
func proxyNameFromArgv(argv []string) (string, bool) {
	if len(argv) == 0 {
		return "", false
	}
	name := filepath.Base(argv[0])
	if runtime.GOOS == "windows" {
		name = strings.TrimSuffix(strings.ToLower(name), ".exe")
	}
	if name == "atmos" || name == "" {
		return "", false
	}
	return name, true
}

// proxyConfigContextDetected reports whether ProxyConfigPathEnv is set,
// which proves the failed config load happened inside a genuine proxy
// invocation (so the caller should surface the error) rather than a normal
// Atmos invocation that merely shares this bootstrap path.
//
// ProxyConfigPathEnv is a private inter-process context variable set by
// ApplyProxyEnvironment for child processes, not user-facing configuration,
// so it is read directly via os.LookupEnv rather than through Viper (which
// isn't initialized yet at this bootstrap point).
func proxyConfigContextDetected() bool {
	configPath, ok := os.LookupEnv(toolchain.ProxyConfigPathEnv)
	return ok && configPath != ""
}

// applyProxyEnvOverrides applies the tool-versions file and install path
// overrides a parent Atmos process passed down via ApplyProxyEnvironment, if
// present.
func applyProxyEnvOverrides(atmosConfig *schema.AtmosConfiguration) {
	if versionsFile, ok := os.LookupEnv(toolchain.ProxyVersionsFileEnv); ok && versionsFile != "" {
		atmosConfig.Toolchain.VersionsFile = versionsFile
	}
	if installPath, ok := os.LookupEnv(toolchain.ProxyInstallPathEnv); ok && installPath != "" {
		atmosConfig.Toolchain.InstallPath = installPath
	}
}

// proxyConfigSelection determines which atmos.yaml to load for a proxy
// invocation. It runs before Cobra/Viper have parsed anything (main() calls
// TryRunToolchainProxy before general flag handling), so it reads
// ProxyConfigPathEnv directly via os.LookupEnv rather than through Viper.
func proxyConfigSelection() schema.ConfigAndStacksInfo {
	if configPath, ok := os.LookupEnv(toolchain.ProxyConfigPathEnv); ok && configPath != "" {
		return schema.ConfigAndStacksInfo{AtmosConfigDirsFromArg: []string{configPath}}
	}
	return cfg.EarlyConfigAndStacksInfoFromArgs(os.Args[1:])
}
