package main

import (
	"fmt"
	"os"
)

// Generate scans root's Go dependencies for licenses, applies the
// deterministic URL overrides, renders NOTICE, and writes it to
// outputPath.
func Generate(root, outputPath string) (Summary, error) {
	return generate(root, outputPath, func() (string, error) { return fetchRepoDescription(repoAPIURL) })
}

// generate is Generate's testable core. fetchDescription is injected so
// tests can supply a stub instead of making a real GitHub API call.
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
