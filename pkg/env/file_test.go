package env

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadEnvFiles(t *testing.T) {
	t.Parallel()

	t.Run("loads single .env file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Create .env file.
		envContent := "FOO=bar\nBAZ=qux\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0o644))

		envVars, loadedFiles, err := LoadEnvFiles(dir, []string{".env"})
		require.NoError(t, err)
		assert.Equal(t, "bar", envVars["FOO"])
		assert.Equal(t, "qux", envVars["BAZ"])
		assert.Len(t, loadedFiles, 1)
		assert.Contains(t, loadedFiles[0], ".env")
	})

	t.Run("loads multiple .env files with glob", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Create multiple .env files.
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("BASE=value\nOVERRIDE=base\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env.local"), []byte("LOCAL=localval\nOVERRIDE=local\n"), 0o644))

		envVars, loadedFiles, err := LoadEnvFiles(dir, []string{".env", ".env.*"})
		require.NoError(t, err)
		assert.Equal(t, "value", envVars["BASE"])
		assert.Equal(t, "localval", envVars["LOCAL"])
		// .env.local comes after .env alphabetically, so it overrides.
		assert.Equal(t, "local", envVars["OVERRIDE"])
		assert.Len(t, loadedFiles, 2)
	})

	t.Run("silently ignores missing files", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		envVars, loadedFiles, err := LoadEnvFiles(dir, []string{".env", ".env.nonexistent"})
		require.NoError(t, err)
		assert.Empty(t, envVars)
		assert.Empty(t, loadedFiles)
	})

	t.Run("preserves case of environment variable names", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		envContent := "GITHUB_TOKEN=secret123\nAWS_PROFILE=dev\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0o644))

		envVars, _, err := LoadEnvFiles(dir, []string{".env"})
		require.NoError(t, err)
		assert.Equal(t, "secret123", envVars["GITHUB_TOKEN"])
		assert.Equal(t, "dev", envVars["AWS_PROFILE"])
		// Ensure lowercase versions don't exist.
		_, hasLower := envVars["github_token"]
		assert.False(t, hasLower)
	})

	t.Run("handles empty patterns", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		envVars, loadedFiles, err := LoadEnvFiles(dir, []string{})
		require.NoError(t, err)
		assert.Empty(t, envVars)
		assert.Empty(t, loadedFiles)
	})

	t.Run("handles values with equals signs", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		envContent := "CONNECTION_STRING=host=localhost;port=5432\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0o644))

		envVars, _, err := LoadEnvFiles(dir, []string{".env"})
		require.NoError(t, err)
		assert.Equal(t, "host=localhost;port=5432", envVars["CONNECTION_STRING"])
	})

	t.Run("handles quoted values", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		envContent := `QUOTED="hello world"
SINGLE='single quoted'
`
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte(envContent), 0o644))

		envVars, _, err := LoadEnvFiles(dir, []string{".env"})
		require.NoError(t, err)
		assert.Equal(t, "hello world", envVars["QUOTED"])
		assert.Equal(t, "single quoted", envVars["SINGLE"])
	})

	t.Run("ignores directories matching pattern", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()

		// Create a directory that matches the pattern.
		require.NoError(t, os.Mkdir(filepath.Join(dir, ".env.d"), 0o755))
		// Create a regular file.
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("FOO=bar\n"), 0o644))

		envVars, loadedFiles, err := LoadEnvFiles(dir, []string{".env", ".env.*"})
		require.NoError(t, err)
		assert.Len(t, loadedFiles, 1)
		assert.Equal(t, "bar", envVars["FOO"])
	})
}

func TestLoadFromDirectory(t *testing.T) {
	t.Parallel()

	t.Run("loads from single directory without parents", func(t *testing.T) {
		t.Parallel()
		repoRoot := t.TempDir()
		workDir := filepath.Join(repoRoot, "components", "vpc")
		require.NoError(t, os.MkdirAll(workDir, 0o755))

		// Create .env only in work dir.
		require.NoError(t, os.WriteFile(filepath.Join(workDir, ".env"), []byte("COMPONENT=vpc\n"), 0o644))

		envVars, loadedFiles, err := LoadFromDirectory(workDir, []string{".env"}, false, repoRoot)
		require.NoError(t, err)
		assert.Equal(t, "vpc", envVars["COMPONENT"])
		assert.Len(t, loadedFiles, 1)
	})

	t.Run("loads from parents when enabled", func(t *testing.T) {
		t.Parallel()
		repoRoot := t.TempDir()
		componentsDir := filepath.Join(repoRoot, "components")
		workDir := filepath.Join(componentsDir, "vpc")
		require.NoError(t, os.MkdirAll(workDir, 0o755))

		// Create .env files at multiple levels.
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".env"), []byte("ROOT=rootval\nOVERRIDE=root\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(componentsDir, ".env"), []byte("COMPONENTS=compval\nOVERRIDE=components\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(workDir, ".env"), []byte("WORKDIR=workval\nOVERRIDE=workdir\n"), 0o644))

		envVars, loadedFiles, err := LoadFromDirectory(workDir, []string{".env"}, true, repoRoot)
		require.NoError(t, err)

		// All values should be present.
		assert.Equal(t, "rootval", envVars["ROOT"])
		assert.Equal(t, "compval", envVars["COMPONENTS"])
		assert.Equal(t, "workval", envVars["WORKDIR"])

		// OVERRIDE should be from workdir (closest).
		assert.Equal(t, "workdir", envVars["OVERRIDE"])

		// Should have loaded 3 files.
		assert.Len(t, loadedFiles, 3)
	})

	t.Run("stops at repo root and does not go beyond", func(t *testing.T) {
		t.Parallel()
		// Create a structure: /tmp/outer/.env (outside repo) /tmp/outer/repo/.env (repo root) /tmp/outer/repo/work/.env.
		outer := t.TempDir()
		repoRoot := filepath.Join(outer, "repo")
		workDir := filepath.Join(repoRoot, "work")
		require.NoError(t, os.MkdirAll(workDir, 0o755))

		// Create .env outside repo root - should NOT be loaded.
		require.NoError(t, os.WriteFile(filepath.Join(outer, ".env"), []byte("OUTSIDE=shouldnotload\n"), 0o644))
		// Create .env in repo root - should be loaded.
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".env"), []byte("REPO=reporoot\n"), 0o644))
		// Create .env in work dir.
		require.NoError(t, os.WriteFile(filepath.Join(workDir, ".env"), []byte("WORK=workdir\n"), 0o644))

		envVars, loadedFiles, err := LoadFromDirectory(workDir, []string{".env"}, true, repoRoot)
		require.NoError(t, err)

		// Should have values from repo root and work dir.
		assert.Equal(t, "reporoot", envVars["REPO"])
		assert.Equal(t, "workdir", envVars["WORK"])

		// Should NOT have value from outside repo root.
		_, hasOutside := envVars["OUTSIDE"]
		assert.False(t, hasOutside, "Should not load .env files from outside repo root")

		// Should have loaded exactly 2 files.
		assert.Len(t, loadedFiles, 2)
	})

	t.Run("handles work dir same as repo root", func(t *testing.T) {
		t.Parallel()
		repoRoot := t.TempDir()

		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".env"), []byte("ROOT=value\n"), 0o644))

		envVars, loadedFiles, err := LoadFromDirectory(repoRoot, []string{".env"}, true, repoRoot)
		require.NoError(t, err)
		assert.Equal(t, "value", envVars["ROOT"])
		assert.Len(t, loadedFiles, 1)
	})

	t.Run("handles missing intermediate directories gracefully", func(t *testing.T) {
		t.Parallel()
		repoRoot := t.TempDir()
		workDir := filepath.Join(repoRoot, "a", "b", "c")
		require.NoError(t, os.MkdirAll(workDir, 0o755))

		// Only create .env in repo root and final work dir.
		require.NoError(t, os.WriteFile(filepath.Join(repoRoot, ".env"), []byte("ROOT=value\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(workDir, ".env"), []byte("WORK=value\n"), 0o644))

		envVars, loadedFiles, err := LoadFromDirectory(workDir, []string{".env"}, true, repoRoot)
		require.NoError(t, err)
		assert.Equal(t, "value", envVars["ROOT"])
		assert.Equal(t, "value", envVars["WORK"])
		assert.Len(t, loadedFiles, 2)
	})
}

func TestMergeEnvMaps(t *testing.T) {
	t.Parallel()

	t.Run("merges multiple maps with later taking precedence", func(t *testing.T) {
		t.Parallel()
		map1 := map[string]string{"A": "1", "B": "1"}
		map2 := map[string]string{"B": "2", "C": "2"}
		map3 := map[string]string{"C": "3", "D": "3"}

		result := MergeEnvMaps(map1, map2, map3)

		assert.Equal(t, "1", result["A"])
		assert.Equal(t, "2", result["B"]) // from map2.
		assert.Equal(t, "3", result["C"]) // from map3.
		assert.Equal(t, "3", result["D"])
	})

	t.Run("handles nil maps", func(t *testing.T) {
		t.Parallel()
		map1 := map[string]string{"A": "1"}
		var nilMap map[string]string

		result := MergeEnvMaps(nilMap, map1, nilMap)

		assert.Equal(t, "1", result["A"])
		assert.Len(t, result, 1)
	})

	t.Run("handles empty input", func(t *testing.T) {
		t.Parallel()
		result := MergeEnvMaps()
		assert.Empty(t, result)
	})
}

func TestMergeEnvSlices(t *testing.T) {
	t.Parallel()

	t.Run("merges slices with later taking precedence", func(t *testing.T) {
		t.Parallel()
		slice1 := []string{"A=1", "B=1"}
		slice2 := []string{"B=2", "C=2"}

		result := MergeEnvSlices(slice1, slice2)

		resultMap := make(map[string]string)
		for _, entry := range result {
			parts := splitStringAtFirstOccurrence(entry, "=")
			resultMap[parts[0]] = parts[1]
		}

		assert.Equal(t, "1", resultMap["A"])
		assert.Equal(t, "2", resultMap["B"]) // from slice2.
		assert.Equal(t, "2", resultMap["C"])
	})

	t.Run("handles values with equals signs", func(t *testing.T) {
		t.Parallel()
		slice := []string{"CONN=host=localhost;port=5432"}

		result := MergeEnvSlices(slice)

		resultMap := make(map[string]string)
		for _, entry := range result {
			parts := splitStringAtFirstOccurrence(entry, "=")
			resultMap[parts[0]] = parts[1]
		}

		assert.Equal(t, "host=localhost;port=5432", resultMap["CONN"])
	})
}

func TestCollectParentDirs(t *testing.T) {
	t.Parallel()

	t.Run("collects directories from work dir to repo root", func(t *testing.T) {
		t.Parallel()
		repoRoot := t.TempDir()
		workDir := filepath.Join(repoRoot, "a", "b", "c")
		require.NoError(t, os.MkdirAll(workDir, 0o755))

		dirs, err := collectParentDirs(workDir, repoRoot)
		require.NoError(t, err)

		// Should have 4 directories: repoRoot, a, a/b, a/b/c.
		assert.Len(t, dirs, 4)
		// First should be repo root (lowest priority).
		assert.Equal(t, repoRoot, dirs[0])
		// Last should be work dir (highest priority).
		assert.Equal(t, workDir, dirs[len(dirs)-1])
	})

	t.Run("returns single dir when work dir equals repo root", func(t *testing.T) {
		t.Parallel()
		repoRoot := t.TempDir()

		dirs, err := collectParentDirs(repoRoot, repoRoot)
		require.NoError(t, err)

		assert.Len(t, dirs, 1)
		assert.Equal(t, repoRoot, dirs[0])
	})

	t.Run("does not go beyond repo root", func(t *testing.T) {
		t.Parallel()
		outer := t.TempDir()
		repoRoot := filepath.Join(outer, "repo")
		workDir := filepath.Join(repoRoot, "work")
		require.NoError(t, os.MkdirAll(workDir, 0o755))

		dirs, err := collectParentDirs(workDir, repoRoot)
		require.NoError(t, err)

		// Should only have repoRoot and workDir, not outer.
		assert.Len(t, dirs, 2)
		for _, d := range dirs {
			assert.True(t, isWithinOrEqual(d, repoRoot), "directory %s should be within repo root", d)
		}
	})
}

func TestIsWithinOrEqual(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		path     string
		root     string
		expected bool
	}{
		{"equal paths", "/a/b/c", "/a/b/c", true},
		{"path within root", "/a/b/c/d", "/a/b/c", true},
		{"path outside root", "/a/b", "/a/b/c", false},
		{"sibling paths", "/a/b/d", "/a/b/c", false},
		{"root with trailing slash", "/a/b/c", "/a/b/c/", true},
		{"similar prefix but different path", "/a/b/c-other", "/a/b/c", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isWithinOrEqual(tt.path, tt.root)
			assert.Equal(t, tt.expected, result)
		})
	}
}
