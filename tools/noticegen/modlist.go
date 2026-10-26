package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// goListModuleVersion returns a function that resolves a single module's
// version in root's build list via `go list -m`, for use as the
// moduleVersion callback in applyOverrides.
func goListModuleVersion(root string, env licenseEnv) func(module string) (string, error) {
	return func(module string) (string, error) {
		cmd := exec.Command("go", "list", "-m", "-f", "{{.Version}}", module) // #nosec G204 -- module comes from the fixed repoOverrides table, not user input.
		cmd.Dir = root
		cmd.Env = env.environ()

		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("go list -m %s: %w", module, err)
		}
		return strings.TrimSpace(string(out)), nil
	}
}

// goListAllGoogleCloudGoModules returns a function that lists every
// cloud.google.com/go[/*] module in root's build list, for use as the
// listModules callback in applyGoogleCloudGoOverrides.
func goListAllGoogleCloudGoModules(root string, env licenseEnv) func() ([]ModuleVersion, error) {
	return func() ([]ModuleVersion, error) {
		cmd := exec.Command("go", "list", "-m", "-f", "{{.Path}} {{.Version}}", "all")
		cmd.Dir = root
		cmd.Env = env.environ()

		out, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("go list -m all: %w", err)
		}

		var modules []ModuleVersion
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) != 2 || !isGoogleCloudGoModule(parts[0]) {
				continue
			}
			modules = append(modules, ModuleVersion{Path: parts[0], Version: parts[1]})
		}
		return modules, nil
	}
}
