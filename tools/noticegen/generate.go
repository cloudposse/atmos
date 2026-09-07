package main

import (
	"fmt"
	"os"
)

// generate scans root's Go dependencies for licenses, applies the
// deterministic URL overrides, renders NOTICE, and writes it to outputPath.
// fetchDescription is injected (run()'s production closure calls
// fetchRepoDescription against the real GitHub API; tests supply a stub)
// so tests can avoid a real network call.
func generate(root, outputPath string, fetchDescription func() (string, error)) (Summary, error) {
	env := defaultLicenseEnv()

	binPath, err := ensureGoLicenses(goLicensesVersion())
	if err != nil {
		return Summary{}, err
	}

	entries := runLicenseReport(binPath, root, env)

	applyOverrides(entries, goListModuleVersion(root, env))
	if err := applyGoogleCloudGoOverrides(entries, goListAllGoogleCloudGoModules(root, env)); err != nil {
		return Summary{}, err
	}

	description, err := fetchDescription()
	if err != nil {
		return Summary{}, fmt.Errorf("fetch repo description: %w", err)
	}

	if err := os.WriteFile(outputPath, []byte(Render(entries, description)), 0o644); err != nil { //nolint:gosec // NOTICE is a plain-text, non-sensitive generated file.
		return Summary{}, fmt.Errorf("write %s: %w", outputPath, err)
	}

	return summarize(entries), nil
}
