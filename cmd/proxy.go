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
	if len(argv) == 0 {
		return false, nil
	}
	name := filepath.Base(argv[0])
	if runtime.GOOS == "windows" {
		name = strings.TrimSuffix(strings.ToLower(name), ".exe")
	}
	if name == "atmos" || name == "" {
		return false, nil
	}
	configInfo := proxyConfigSelection()
	atmosConfig, err := cfg.InitCliConfig(configInfo, false)
	if err != nil {
		// A proxy context proves this executable was intentionally invoked
		// through a generated link, so surface its configuration error rather
		// than falling through to an unrelated Atmos subcommand error.
		return os.Getenv(toolchain.ProxyConfigPathEnv) != "", err
	}
	if _, ok := atmosConfig.Toolchain.Proxies[name]; !ok {
		return false, nil
	}
	if versionsFile := os.Getenv(toolchain.ProxyVersionsFileEnv); versionsFile != "" {
		atmosConfig.Toolchain.VersionsFile = versionsFile
	}
	if installPath := os.Getenv(toolchain.ProxyInstallPathEnv); installPath != "" {
		atmosConfig.Toolchain.InstallPath = installPath
	}
	toolchain.SetAtmosConfig(&atmosConfig)
	return true, toolchain.RunProxy(&atmosConfig, name, argv[1:])
}

func proxyConfigSelection() schema.ConfigAndStacksInfo {
	if configPath := os.Getenv(toolchain.ProxyConfigPathEnv); configPath != "" {
		return schema.ConfigAndStacksInfo{AtmosConfigDirsFromArg: []string{configPath}}
	}
	return cfg.EarlyConfigAndStacksInfoFromArgs(os.Args[1:])
}
