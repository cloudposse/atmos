package main

import (
	"fmt"
	"os"
	"strings"
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

	description, err := resolveDescription(outputPath, fetchDescription)
	if err != nil {
		return Summary{}, err
	}

	if err := os.WriteFile(outputPath, []byte(Render(entries, description)), 0o644); err != nil { //nolint:gosec // NOTICE is a plain-text, non-sensitive generated file.
		return Summary{}, fmt.Errorf("write %s: %w", outputPath, err)
	}

	return summarize(entries), nil
}

// resolveDescription returns the repository's live GitHub description. When the fetch fails it
// falls back to the tagline already committed in outputPath, with a warning: CI deliberately
// calls the GitHub API without a token (the job runs PR-controlled code), and on GitHub-hosted
// runners the unauthenticated rate limit is per IP and shared with every other job on the
// runner, so a 403 is routine. Reusing the committed tagline keeps that from failing generation
// or producing a spurious NOTICE diff; drift is still caught whenever the fetch succeeds. With no
// usable existing tagline, the fetch error is returned.
func resolveDescription(outputPath string, fetchDescription func() (string, error)) (string, error) {
	description, err := fetchDescription()
	if err == nil {
		return description, nil
	}
	existing, ok := existingDescription(outputPath)
	if !ok {
		return "", fmt.Errorf("fetch repo description: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Warning: fetch repo description: %v; keeping the description already in %s\n", err, outputPath)
	return existing, nil
}

// existingDescription extracts the tagline from a NOTICE file rendered by buildHeader: the line
// after the "NOTICE" title and blank line, followed by the copyright line. It reports false when
// the file is missing or does not have that shape.
func existingDescription(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	const minHeaderLines = 4
	if len(lines) < minHeaderLines || lines[0] != "NOTICE" || lines[1] != "" || !strings.HasPrefix(lines[3], "Copyright ") {
		return "", false
	}
	description := strings.TrimSpace(lines[2])
	if description == "" || !isSingleLinePrintable(description) {
		return "", false
	}
	return description, true
}
