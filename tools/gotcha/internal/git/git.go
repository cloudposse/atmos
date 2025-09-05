package git

import (
	"bufio"
	"os/exec"
	"path/filepath"
	"strings"
)

// getChangedFiles returns a list of changed .go files in the current PR.
// GetChangedFiles returns a list of changed files in the git repository.
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

// getChangedPackages returns a list of packages that have been changed.
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
