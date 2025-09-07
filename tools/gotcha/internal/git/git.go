package git

import (
	"bufio"
	"os/exec"
	"path/filepath"
	"strings"
)

// GetChangedFiles returns a list of changed Go files in the current PR/branch for PR-focused test reporting.
// This function enables gotcha to filter test results to only show failures from packages
// that have been modified in the current branch, reducing noise in PR reviews by hiding
// unrelated test failures from unchanged code.
// It compares the current branch against origin/main using git diff to identify modified files.
func GetChangedFiles() []string {
	cmd := exec.Command("git", "diff", "--name-only", "origin/main...HEAD")
	output, err := cmd.Output()
	if err != nil {
		// If git command fails, return empty slice (fallback to showing all).
		return []string{}
	}

	files := []string{}
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && strings.HasSuffix(line, ".go") {
			files = append(files, line)
		}
	}
	return files
}

// GetChangedPackages returns a list of packages that have been modified in the current PR/branch.
// This function supports PR-focused test reporting by converting the list of changed files
// from GetChangedFiles() into their corresponding Go package paths. This allows gotcha to
// run tests only on packages that have been modified, significantly speeding up CI/CD
// pipelines and reducing irrelevant test noise in pull request reviews.
func GetChangedPackages() []string {
	files := GetChangedFiles()
	packageSet := make(map[string]bool)

	for _, file := range files {
		// Convert file path to package path.
		// e.g., "tools/gotcha/main.go" -> "tools/gotcha".
		dir := filepath.Dir(file)
		if dir != "." {
			packageSet[dir] = true
		}
	}

	packages := []string{}
	for pkg := range packageSet {
		packages = append(packages, pkg)
	}
	return packages
}
