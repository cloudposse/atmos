package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Static errors returned by run so callers can check them with errors.Is.
var (
	errReplaceNotAllowed   = errors.New("replace directives not allowed")
	errExcludeNotAllowed   = errors.New("exclude directives not allowed")
	errForbiddenModuleUsed = errors.New("forbidden modules required")
)

// forbiddenModules maps module paths that must never appear in a require
// directive (directly or as `// indirect`) to the reason they are banned.
// Matching is on the exact module path token, so versioned major paths such
// as github.com/aws/aws-sdk-go-v2 never match github.com/aws/aws-sdk-go.
var forbiddenModules = map[string]string{
	"github.com/aws/aws-sdk-go": "AWS SDK for Go v1 is end-of-life and has unfixable OSV advisories (GO-2022-0635, GO-2022-0646). " +
		"It was deliberately removed from the dependency tree; see docs/fixes/2026-09-02-gomplate-v5-aws-sdk-v1-removal.md " +
		"for the import chain and the hashicorp/vault/api/auth/aws pin that keeps it out.",
}

func main() {
	goModPath := "go.mod"
	if len(os.Args) > 1 {
		goModPath = os.Args[1]
	}
	if err := run(goModPath); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// forbiddenHit records one require line that names a forbidden module.
type forbiddenHit struct {
	module string
	line   string
}

func run(goModPath string) error {
	file, err := os.Open(goModPath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", goModPath, err)
	}
	defer file.Close()

	var replaceDirectives []string
	var excludeDirectives []string
	var forbiddenHits []forbiddenHit
	inRequireBlock := false

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Check for replace directives (inline or block start).
		if strings.HasPrefix(trimmed, "replace ") {
			replaceDirectives = append(replaceDirectives, line)
		}

		// Check for exclude directives (inline or block start).
		if strings.HasPrefix(trimmed, "exclude ") {
			excludeDirectives = append(excludeDirectives, line)
		}

		// Track `require (` ... `)` blocks so bare module lines inside them are recognized.
		switch {
		case trimmed == "require (":
			inRequireBlock = true
			continue
		case inRequireBlock && trimmed == ")":
			inRequireBlock = false
			continue
		}

		if module := requireModulePath(trimmed, inRequireBlock); module != "" {
			if _, forbidden := forbiddenModules[module]; forbidden {
				forbiddenHits = append(forbiddenHits, forbiddenHit{module: module, line: line})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading %s: %w", goModPath, err)
	}

	// Report errors.
	if len(replaceDirectives) > 0 {
		fmt.Fprintf(os.Stderr, "ERROR: go.mod contains 'replace' directives which break 'go install'.\n\n")
		fmt.Fprintf(os.Stderr, "Replace directives found:\n")
		for _, directive := range replaceDirectives {
			fmt.Fprintf(os.Stderr, "  %s\n", directive)
		}
		fmt.Fprintf(os.Stderr, "\nThis breaks a documented installation method for Atmos.\n")
		fmt.Fprintf(os.Stderr, "Consider alternative approaches that don't break go install compatibility.\n")
		return errReplaceNotAllowed
	}

	if len(excludeDirectives) > 0 {
		fmt.Fprintf(os.Stderr, "ERROR: go.mod contains 'exclude' directives which break 'go install'.\n\n")
		fmt.Fprintf(os.Stderr, "Exclude directives found:\n")
		for _, directive := range excludeDirectives {
			fmt.Fprintf(os.Stderr, "  %s\n", directive)
		}
		fmt.Fprintf(os.Stderr, "\nThis breaks a documented installation method for Atmos.\n")
		fmt.Fprintf(os.Stderr, "Consider alternative approaches that don't break go install compatibility.\n")
		return errExcludeNotAllowed
	}

	if len(forbiddenHits) > 0 {
		for _, hit := range forbiddenHits {
			fmt.Fprintf(os.Stderr, "ERROR: go.mod requires forbidden module %s\n", hit.module)
			fmt.Fprintf(os.Stderr, "  %s\n", hit.line)
			fmt.Fprintf(os.Stderr, "  %s\n\n", forbiddenModules[hit.module])
		}
		return errForbiddenModuleUsed
	}

	fmt.Println("✓ go.mod is compatible with 'go install'")
	return nil
}

// requireModulePath returns the module path named by a require line, or ""
// when the line is not a require. Inside a `require (` block the module path
// is the first field of the line; for an inline `require example.com/m v1.2.3`
// it is the second field. Comment-only and blank lines never match.
func requireModulePath(trimmed string, inBlock bool) string {
	if trimmed == "" || strings.HasPrefix(trimmed, "//") {
		return ""
	}
	fields := strings.Fields(trimmed)
	switch {
	case inBlock:
		return fields[0]
	case fields[0] == "require" && len(fields) >= 2 && fields[1] != "(":
		return fields[1]
	default:
		return ""
	}
}
