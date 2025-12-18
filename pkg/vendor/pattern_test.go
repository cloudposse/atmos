package vendor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShouldSkipBasedOnIncludedPaths(t *testing.T) {
	tempDir := t.TempDir()

	// Create test directory structure.
	examplesDir := filepath.Join(tempDir, "examples")
	srcDir := filepath.Join(tempDir, "src")
	err := os.MkdirAll(examplesDir, 0o755)
	assert.NoError(t, err)
	err = os.MkdirAll(srcDir, 0o755)
	assert.NoError(t, err)

	// Create test files.
	readmeFile := filepath.Join(tempDir, "README.md")
	mainTf := filepath.Join(srcDir, "main.tf")
	err = os.WriteFile(readmeFile, []byte("readme"), 0o644)
	assert.NoError(t, err)
	err = os.WriteFile(mainTf, []byte("main"), 0o644)
	assert.NoError(t, err)

	tests := []struct {
		name          string
		isDir         bool
		fullPath      string
		relativePath  string
		includedPaths []string
		shouldSkip    bool
		expectError   bool
	}{
		{
			name:          "file matches pattern - not skipped",
			isDir:         false,
			fullPath:      mainTf,
			relativePath:  "src/main.tf",
			includedPaths: []string{"**/*.tf"},
			shouldSkip:    false,
		},
		{
			name:          "file does not match pattern - skipped",
			isDir:         false,
			fullPath:      readmeFile,
			relativePath:  "README.md",
			includedPaths: []string{"**/*.tf"},
			shouldSkip:    true,
		},
		{
			name:          "directory could match nested pattern with concrete dir",
			isDir:         true,
			fullPath:      srcDir,
			relativePath:  "src",
			includedPaths: []string{"**/src/**/*.tf"},
			shouldSkip:    false,
		},
		{
			name:          "empty included paths - file skipped",
			isDir:         false,
			fullPath:      mainTf,
			relativePath:  "src/main.tf",
			includedPaths: []string{},
			shouldSkip:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, err := shouldSkipBasedOnIncludedPaths(tt.isDir, tt.fullPath, tt.relativePath, tt.includedPaths)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.shouldSkip, skip)
			}
		})
	}
}

func TestShouldSkipFile(t *testing.T) {
	tempDir := t.TempDir()

	// Create test files.
	mainTf := filepath.Join(tempDir, "main.tf")
	variablesTf := filepath.Join(tempDir, "variables.tf")
	readme := filepath.Join(tempDir, "README.md")
	err := os.WriteFile(mainTf, []byte("main"), 0o644)
	assert.NoError(t, err)
	err = os.WriteFile(variablesTf, []byte("variables"), 0o644)
	assert.NoError(t, err)
	err = os.WriteFile(readme, []byte("readme"), 0o644)
	assert.NoError(t, err)

	tests := []struct {
		name          string
		fullPath      string
		relativePath  string
		includedPaths []string
		shouldSkip    bool
	}{
		{
			name:          "file matches single pattern",
			fullPath:      mainTf,
			relativePath:  "main.tf",
			includedPaths: []string{"*.tf"},
			shouldSkip:    false,
		},
		{
			name:          "file matches one of multiple patterns",
			fullPath:      readme,
			relativePath:  "README.md",
			includedPaths: []string{"*.tf", "*.md"},
			shouldSkip:    false,
		},
		{
			name:          "file matches none of patterns",
			fullPath:      readme,
			relativePath:  "README.md",
			includedPaths: []string{"*.tf", "*.go"},
			shouldSkip:    true,
		},
		{
			name:          "empty patterns skips all files",
			fullPath:      mainTf,
			relativePath:  "main.tf",
			includedPaths: []string{},
			shouldSkip:    true,
		},
		{
			name:          "pattern with leading slash",
			fullPath:      mainTf,
			relativePath:  "main.tf",
			includedPaths: []string{"/main.tf"},
			shouldSkip:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, err := shouldSkipFile(tt.fullPath, tt.relativePath, tt.includedPaths)
			assert.NoError(t, err)
			assert.Equal(t, tt.shouldSkip, skip)
		})
	}
}

func TestMatchFileAgainstPattern(t *testing.T) {
	tempDir := t.TempDir()

	// Create nested directory structure.
	srcDir := filepath.Join(tempDir, "src", "modules")
	err := os.MkdirAll(srcDir, 0o755)
	assert.NoError(t, err)

	mainTf := filepath.Join(srcDir, "main.tf")
	err = os.WriteFile(mainTf, []byte("main"), 0o644)
	assert.NoError(t, err)

	tests := []struct {
		name         string
		pattern      string
		fullPath     string
		relativePath string
		shouldMatch  bool
	}{
		{
			name:         "simple glob matches against relative path",
			pattern:      "**/*.tf",
			fullPath:     mainTf,
			relativePath: "src/modules/main.tf",
			shouldMatch:  true,
		},
		{
			name:         "doublestar pattern matches nested",
			pattern:      "**/*.tf",
			fullPath:     mainTf,
			relativePath: "src/modules/main.tf",
			shouldMatch:  true,
		},
		{
			name:         "pattern with leading slash",
			pattern:      "/src/modules/main.tf",
			fullPath:     mainTf,
			relativePath: "src/modules/main.tf",
			shouldMatch:  true,
		},
		{
			name:         "non-matching pattern",
			pattern:      "*.go",
			fullPath:     mainTf,
			relativePath: "src/modules/main.tf",
			shouldMatch:  false,
		},
		{
			name:         "exact path match",
			pattern:      "src/modules/main.tf",
			fullPath:     mainTf,
			relativePath: "src/modules/main.tf",
			shouldMatch:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched, err := matchFileAgainstPattern(tt.pattern, tt.fullPath, tt.relativePath)
			assert.NoError(t, err)
			assert.Equal(t, tt.shouldMatch, matched)
		})
	}
}

func TestShouldSkipDirectory(t *testing.T) {
	tempDir := t.TempDir()

	// Create directory structure.
	modulesDir := filepath.Join(tempDir, "modules")
	examplesDir := filepath.Join(tempDir, "examples")
	testsDir := filepath.Join(tempDir, "tests")
	err := os.MkdirAll(modulesDir, 0o755)
	assert.NoError(t, err)
	err = os.MkdirAll(examplesDir, 0o755)
	assert.NoError(t, err)
	err = os.MkdirAll(testsDir, 0o755)
	assert.NoError(t, err)

	tests := []struct {
		name          string
		fullPath      string
		relativePath  string
		includedPaths []string
		shouldSkip    bool
	}{
		{
			name:          "directory matches pattern directly",
			fullPath:      modulesDir,
			relativePath:  "modules",
			includedPaths: []string{"modules/**"},
			shouldSkip:    false,
		},
		{
			name:          "directory could match nested files with concrete dir",
			fullPath:      modulesDir,
			relativePath:  "modules",
			includedPaths: []string{"**/modules/**/*.tf"},
			shouldSkip:    false,
		},
		{
			name:          "directory with no matching patterns",
			fullPath:      testsDir,
			relativePath:  "tests",
			includedPaths: []string{"modules/**/*.tf"},
			shouldSkip:    true,
		},
		{
			name:          "empty patterns skips directory",
			fullPath:      modulesDir,
			relativePath:  "modules",
			includedPaths: []string{},
			shouldSkip:    true,
		},
		{
			name:          "brace expansion pattern",
			fullPath:      modulesDir,
			relativePath:  "modules",
			includedPaths: []string{"{modules,examples}/**"},
			shouldSkip:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, err := shouldSkipDirectory(tt.fullPath, tt.relativePath, tt.includedPaths)
			assert.NoError(t, err)
			assert.Equal(t, tt.shouldSkip, skip)
		})
	}
}

func TestCouldMatchNestedPath(t *testing.T) {
	tests := []struct {
		name       string
		dirPath    string
		pattern    string
		couldMatch bool
	}{
		{
			name:       "doublestar prefix - any directory could match",
			dirPath:    "some/random/dir",
			pattern:    "**/modules/**",
			couldMatch: true,
		},
		{
			name:       "doublestar prefix with required directory - matches required",
			dirPath:    "modules",
			pattern:    "**/modules/**/*.tf",
			couldMatch: true,
		},
		{
			name:       "doublestar prefix with brace expansion",
			dirPath:    "demo-library",
			pattern:    "**/{demo-library,demo-stacks}/**/*.md",
			couldMatch: true,
		},
		{
			name:       "pattern without doublestar - dir is prefix of pattern",
			dirPath:    "src",
			pattern:    "src/modules/*.tf",
			couldMatch: true,
		},
		{
			name:       "pattern without doublestar - dir not prefix of pattern",
			dirPath:    "tests",
			pattern:    "src/modules/*.tf",
			couldMatch: false,
		},
		{
			name:       "doublestar in middle - directory matches part",
			dirPath:    "modules",
			pattern:    "src/**/modules/**",
			couldMatch: true,
		},
		{
			name:       "simple pattern with leading slash",
			dirPath:    "src",
			pattern:    "/src/file.tf",
			couldMatch: true,
		},
		{
			name:       "simple pattern - exact match",
			dirPath:    "src",
			pattern:    "src/*",
			couldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := couldMatchNestedPath(tt.dirPath, tt.pattern)
			assert.Equal(t, tt.couldMatch, result)
		})
	}
}

func TestCheckDoublestarPrefixPattern(t *testing.T) {
	tests := []struct {
		name       string
		dirPath    string
		pattern    string
		shouldPass bool
	}{
		{
			name:       "directory matches required dir in pattern",
			dirPath:    "demo-library",
			pattern:    "**/demo-library/**/*.md",
			shouldPass: true,
		},
		{
			name:       "directory matches one of brace expansion",
			dirPath:    "demo-stacks",
			pattern:    "**/{demo-library,demo-stacks}/**/*.md",
			shouldPass: true,
		},
		{
			name:       "wildcard patterns with no concrete dirs returns true",
			dirPath:    "random-dir",
			pattern:    "**/random-dir/**/*.tf",
			shouldPass: true,
		},
		{
			name:       "nested directory with matching segment",
			dirPath:    "path/to/demo-library",
			pattern:    "**/demo-library/**",
			shouldPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkDoublestarPrefixPattern(tt.dirPath, tt.pattern)
			assert.Equal(t, tt.shouldPass, result)
		})
	}
}

func TestCheckDoublestarPattern(t *testing.T) {
	tests := []struct {
		name       string
		dirPath    string
		pattern    string
		shouldPass bool
	}{
		{
			name:       "directory name matches pattern part",
			dirPath:    "modules",
			pattern:    "src/**/modules/**/*.tf",
			shouldPass: true,
		},
		{
			name:       "directory is prefix of pattern prefix",
			dirPath:    "src",
			pattern:    "src/**/modules/**/*.tf",
			shouldPass: true,
		},
		{
			name:       "directory name in nested path matches",
			dirPath:    "path/to/modules",
			pattern:    "src/**/modules/**/*.tf",
			shouldPass: true,
		},
		{
			name:       "directory name does not match any pattern part",
			dirPath:    "random",
			pattern:    "src/**/modules/**/*.tf",
			shouldPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkDoublestarPattern(tt.dirPath, tt.pattern)
			assert.Equal(t, tt.shouldPass, result)
		})
	}
}

func TestCheckSimplePattern(t *testing.T) {
	tests := []struct {
		name       string
		dirPath    string
		pattern    string
		shouldPass bool
	}{
		{
			name:       "directory is prefix of pattern path",
			dirPath:    "src",
			pattern:    "src/modules/main.tf",
			shouldPass: true,
		},
		{
			name:       "directory matches pattern with wildcard",
			dirPath:    "src",
			pattern:    "src/*",
			shouldPass: true,
		},
		{
			name:       "directory with leading slash",
			dirPath:    "/src",
			pattern:    "/src/modules/main.tf",
			shouldPass: true,
		},
		{
			name:       "directory not prefix of pattern",
			dirPath:    "tests",
			pattern:    "src/modules/main.tf",
			shouldPass: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkSimplePattern(tt.dirPath, tt.pattern)
			assert.Equal(t, tt.shouldPass, result)
		})
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		name     string
		parts    []string
		targets  []string
		expected bool
	}{
		{
			name:     "match found",
			parts:    []string{"a", "b", "c"},
			targets:  []string{"b", "d"},
			expected: true,
		},
		{
			name:     "no match",
			parts:    []string{"a", "b", "c"},
			targets:  []string{"d", "e"},
			expected: false,
		},
		{
			name:     "empty parts",
			parts:    []string{},
			targets:  []string{"a"},
			expected: false,
		},
		{
			name:     "empty targets",
			parts:    []string{"a"},
			targets:  []string{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := containsAny(tt.parts, tt.targets)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchesPatternPart(t *testing.T) {
	tests := []struct {
		name         string
		dirBase      string
		patternParts []string
		expected     bool
	}{
		{
			name:         "exact match",
			dirBase:      "modules",
			patternParts: []string{"src", "modules", "*.tf"},
			expected:     true,
		},
		{
			name:         "brace expansion match",
			dirBase:      "demo-library",
			patternParts: []string{"{demo-library,demo-stacks}", "*.md"},
			expected:     true,
		},
		{
			name:         "dir matches concrete part after wildcards",
			dirBase:      "modules",
			patternParts: []string{"**", "modules", "*.tf"},
			expected:     true,
		},
		{
			name:         "no match",
			dirBase:      "random",
			patternParts: []string{"src", "modules"},
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesPatternPart(tt.dirBase, tt.patternParts)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchesBraceOrConcrete(t *testing.T) {
	tests := []struct {
		name     string
		dirBase  string
		part     string
		expected bool
	}{
		{
			name:     "exact concrete match",
			dirBase:  "modules",
			part:     "modules",
			expected: true,
		},
		{
			name:     "brace expansion - first option",
			dirBase:  "demo-library",
			part:     "{demo-library,demo-stacks}",
			expected: true,
		},
		{
			name:     "brace expansion - second option",
			dirBase:  "demo-stacks",
			part:     "{demo-library,demo-stacks}",
			expected: true,
		},
		{
			name:     "brace expansion - no match",
			dirBase:  "other",
			part:     "{demo-library,demo-stacks}",
			expected: false,
		},
		{
			name:     "concrete no match",
			dirBase:  "tests",
			part:     "modules",
			expected: false,
		},
		{
			name:     "part with wildcard - not concrete",
			dirBase:  "modules",
			part:     "*.tf",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchesBraceOrConcrete(tt.dirBase, tt.part)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCheckPatternPrefix(t *testing.T) {
	tests := []struct {
		name     string
		dirPath  string
		pattern  string
		expected bool
	}{
		{
			name:     "directory is prefix of pattern prefix",
			dirPath:  "src",
			pattern:  "src/modules/**/file.tf",
			expected: true,
		},
		{
			name:     "directory equals pattern prefix",
			dirPath:  "src/modules",
			pattern:  "src/modules/**/file.tf",
			expected: true,
		},
		{
			name:     "pattern prefix is prefix of directory",
			dirPath:  "src/modules/vpc",
			pattern:  "src/**/file.tf",
			expected: true,
		},
		{
			name:     "no match",
			dirPath:  "tests",
			pattern:  "src/modules/**/file.tf",
			expected: false,
		},
		{
			name:     "pattern starts with doublestar - empty prefix",
			dirPath:  "src",
			pattern:  "**/modules/file.tf",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checkPatternPrefix(tt.dirPath, tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetPatternPrefixBeforeDoublestar(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		expected string
	}{
		{
			name:     "pattern with doublestar in middle",
			pattern:  "src/modules/**/file.tf",
			expected: "src/modules",
		},
		{
			name:     "pattern starting with doublestar",
			pattern:  "**/modules/file.tf",
			expected: "",
		},
		{
			name:     "pattern with wildcard before doublestar",
			pattern:  "src/*/modules/**/file.tf",
			expected: "src",
		},
		{
			name:     "pattern without doublestar",
			pattern:  "src/modules/file.tf",
			expected: "src/modules/file.tf",
		},
		{
			name:     "pattern with brace expansion",
			pattern:  "src/{a,b}/**/file.tf",
			expected: "src",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getPatternPrefixBeforeDoublestar(tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractRequiredDirs(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		expected []string
	}{
		{
			name:     "simple directory in pattern",
			pattern:  "**/modules/**/*.tf",
			expected: []string{"modules"},
		},
		{
			name:     "brace expansion",
			pattern:  "**/{demo-library,demo-stacks}/**/*.md",
			expected: []string{"demo-library", "demo-stacks"},
		},
		{
			name:     "multiple directories",
			pattern:  "src/modules/**/*.tf",
			expected: []string{"src", "modules"},
		},
		{
			name:     "only wildcards",
			pattern:  "**/*",
			expected: nil,
		},
		{
			name:     "pattern with file extension",
			pattern:  "**/modules/main.tf",
			expected: []string{"modules"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRequiredDirs(tt.pattern)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractDirsFromPart(t *testing.T) {
	tests := []struct {
		name     string
		part     string
		expected []string
	}{
		{
			name:     "concrete directory name",
			part:     "modules",
			expected: []string{"modules"},
		},
		{
			name:     "brace expansion",
			part:     "{a,b,c}",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "doublestar",
			part:     "**",
			expected: nil,
		},
		{
			name:     "single star",
			part:     "*",
			expected: nil,
		},
		{
			name:     "empty string",
			part:     "",
			expected: nil,
		},
		{
			name:     "wildcard in name",
			part:     "*.tf",
			expected: nil,
		},
		{
			name:     "file with extension",
			part:     "main.tf",
			expected: nil,
		},
		{
			name:     "brace with wildcard option",
			part:     "{a,*.tf}",
			expected: []string{"a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDirsFromPart(tt.part)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractDirsFromBraceExpansion(t *testing.T) {
	tests := []struct {
		name     string
		part     string
		expected []string
	}{
		{
			name:     "simple brace expansion",
			part:     "{a,b,c}",
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "brace expansion with wildcards filtered",
			part:     "{demo-library,*.tf,demo-stacks}",
			expected: []string{"demo-library", "demo-stacks"},
		},
		{
			name:     "single option",
			part:     "{modules}",
			expected: []string{"modules"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractDirsFromBraceExpansion(tt.part)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractConcreteDirName(t *testing.T) {
	tests := []struct {
		name     string
		part     string
		expected []string
	}{
		{
			name:     "directory name without extension",
			part:     "modules",
			expected: []string{"modules"},
		},
		{
			name:     "file with extension",
			part:     "main.tf",
			expected: nil,
		},
		{
			name:     "directory ending with slash",
			part:     "modules/",
			expected: []string{"modules/"},
		},
		{
			name:     "directory with numbers",
			part:     "module1",
			expected: []string{"module1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractConcreteDirName(tt.part)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDirectoryPatternMatching(t *testing.T) {
	tempDir := t.TempDir()

	modulesDir := filepath.Join(tempDir, "modules")
	err := os.MkdirAll(modulesDir, 0o755)
	assert.NoError(t, err)

	// Test checkDirectoryAgainstPattern.
	t.Run("checkDirectoryAgainstPattern", func(t *testing.T) {
		tests := []struct {
			name          string
			pattern       string
			shouldInclude bool
		}{
			{"pattern could match direct children", "modules/*", true},
			{"pattern could match nested files", "**/modules/**/*.tf", true},
			{"no match possible", "other/**", false},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := checkDirectoryAgainstPattern(tt.pattern, modulesDir, "modules")
				assert.NoError(t, err)
				assert.Equal(t, tt.shouldInclude, result)
			})
		}
	})

	// Test matchDirectoryDirectly.
	t.Run("matchDirectoryDirectly", func(t *testing.T) {
		tests := []struct {
			name        string
			pattern     string
			shouldMatch bool
		}{
			{"pattern with leading slash", "/modules", true},
			{"no match", "other", false},
			{"doublestar pattern matches", "**/modules", true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := matchDirectoryDirectly(tt.pattern, modulesDir, "modules")
				assert.NoError(t, err)
				assert.Equal(t, tt.shouldMatch, result)
			})
		}
	})

	// Test matchDirectoryChildren.
	t.Run("matchDirectoryChildren", func(t *testing.T) {
		tests := []struct {
			name        string
			pattern     string
			shouldMatch bool
		}{
			{"pattern with leading slash", "/modules/*", true},
			{"pattern does not match children", "other/*", false},
			{"doublestar matches children", "**/modules/*", true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result, err := matchDirectoryChildren(tt.pattern, modulesDir, "modules")
				assert.NoError(t, err)
				assert.Equal(t, tt.shouldMatch, result)
			})
		}
	})
}
