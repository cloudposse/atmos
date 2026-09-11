package autoinit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFile is a small test helper that writes content to name inside dir, creating dir as
// needed, and fails the test on error.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// baseInputs returns a minimal, valid Inputs for componentPath with a resolvable binary.
func baseInputs(componentPath string) *Inputs {
	return &Inputs{
		ComponentPath: componentPath,
		Binary:        testBinary(),
	}
}

// testBinary returns an absolute path to an executable guaranteed to exist and be stable across
// a single test run, so binaryRecord's os.Stat succeeds deterministically without depending on
// PATH contents.
func testBinary() string {
	exe, err := os.Executable()
	if err != nil {
		return "atmos-test-binary"
	}
	return exe
}

func TestCompute_Deterministic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.tf", "resource \"null_resource\" \"x\" {}")

	in := baseInputs(dir)

	fp1, err := Compute(in)
	require.NoError(t, err)
	fp2, err := Compute(in)
	require.NoError(t, err)

	assert.Equal(t, fp1.Hash, fp2.Hash)
	assert.Equal(t, fp1.BackendHash, fp2.BackendHash)
	assert.NotEmpty(t, fp1.Hash)
}

func TestCompute_IndependentOfFileCreationOrder(t *testing.T) {
	dirA := t.TempDir()
	writeFile(t, dirA, "a.tf", "a")
	writeFile(t, dirA, "b.tf", "b")

	dirB := t.TempDir()
	writeFile(t, dirB, "b.tf", "b")
	writeFile(t, dirB, "a.tf", "a")

	inA := baseInputs(dirA)
	inB := baseInputs(dirB)
	inB.Binary = inA.Binary

	fpA, err := Compute(inA)
	require.NoError(t, err)
	fpB, err := Compute(inB)
	require.NoError(t, err)

	assert.Equal(t, fpA.Hash, fpB.Hash, "creation order must not affect the fingerprint")
}

func TestCompute_ChangesOnRootConfigFileEdit(t *testing.T) {
	tests := []string{"main.tf", "main.tf.json", "main.tofu", "main.tofu.json"}

	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, name, "v1")
			in := baseInputs(dir)

			fp1, err := Compute(in)
			require.NoError(t, err)

			writeFile(t, dir, name, "v2")
			fp2, err := Compute(in)
			require.NoError(t, err)

			assert.NotEqual(t, fp1.Hash, fp2.Hash, "editing %s must change the fingerprint", name)
			assert.Contains(t, fp2.Files, name)
		})
	}
}

func TestCompute_ChangesWhenRootConfigFileAdded(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.tf", "v1")
	in := baseInputs(dir)

	fp1, err := Compute(in)
	require.NoError(t, err)

	writeFile(t, dir, "extra.tf", "extra")
	fp2, err := Compute(in)
	require.NoError(t, err)

	assert.NotEqual(t, fp1.Hash, fp2.Hash)
}

func TestCompute_ChangesOnLockFileEdit(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.tf", "v1")
	writeFile(t, dir, ".terraform.lock.hcl", "v1")
	in := baseInputs(dir)

	fp1, err := Compute(in)
	require.NoError(t, err)

	writeFile(t, dir, ".terraform.lock.hcl", "v2")
	fp2, err := Compute(in)
	require.NoError(t, err)

	assert.NotEqual(t, fp1.Hash, fp2.Hash)
	assert.Contains(t, fp2.Files, ".terraform.lock.hcl")
}

func TestCompute_VarFileOnlyHashedWhenPassVars(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.tf", "v1")
	varFile := writeFile(t, dir, "vars.tfvars", "v1")

	inNoPassVars := baseInputs(dir)
	inNoPassVars.VarFile = varFile

	fp1, err := Compute(inNoPassVars)
	require.NoError(t, err)

	writeFile(t, dir, "vars.tfvars", "v2")
	fp2, err := Compute(inNoPassVars)
	require.NoError(t, err)
	assert.Equal(t, fp1.Hash, fp2.Hash, "varfile edits must not affect the fingerprint when PassVars is false")

	inPassVars := baseInputs(dir)
	inPassVars.VarFile = varFile
	inPassVars.PassVars = true

	fp3, err := Compute(inPassVars)
	require.NoError(t, err)

	writeFile(t, dir, "vars.tfvars", "v3")
	fp4, err := Compute(inPassVars)
	require.NoError(t, err)
	assert.NotEqual(t, fp3.Hash, fp4.Hash, "varfile edits must affect the fingerprint when PassVars is true")
}

func TestCompute_CLIConfigFileContentNotPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.tf", "v1")

	path1 := writeFile(t, t.TempDir(), "cli-config-1.tfrc", "same content")
	path2 := writeFile(t, t.TempDir(), "cli-config-2.tfrc", "same content")

	in1 := baseInputs(dir)
	in1.EnvLookup = func(key string) string {
		if key == "TF_CLI_CONFIG_FILE" {
			return path1
		}
		return ""
	}
	in2 := baseInputs(dir)
	in2.EnvLookup = func(key string) string {
		if key == "TF_CLI_CONFIG_FILE" {
			return path2
		}
		return ""
	}

	fp1, err := Compute(in1)
	require.NoError(t, err)
	fp2, err := Compute(in2)
	require.NoError(t, err)
	assert.Equal(t, fp1.Hash, fp2.Hash, "identical CLI config content at different paths must hash identically")

	writeFile(t, filepath.Dir(path2), "cli-config-2.tfrc", "different content")
	fp3, err := Compute(in2)
	require.NoError(t, err)
	assert.NotEqual(t, fp1.Hash, fp3.Hash, "CLI config content changes must change the fingerprint")
}

func TestCompute_ChangesOnBinarySizeOrMtime(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.tf", "v1")

	binDir := t.TempDir()
	binPath := writeFile(t, binDir, "terraform", "v1")

	in := baseInputs(dir)
	in.Binary = binPath

	fp1, err := Compute(in)
	require.NoError(t, err)

	// Change size (and, incidentally, mtime).
	require.NoError(t, os.WriteFile(binPath, []byte("a longer binary content"), 0o644))
	fp2, err := Compute(in)
	require.NoError(t, err)
	assert.NotEqual(t, fp1.Hash, fp2.Hash, "binary size change must change the fingerprint")

	// Change only mtime, keeping size identical.
	newer := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(binPath, newer, newer))
	fp3, err := Compute(in)
	require.NoError(t, err)
	assert.NotEqual(t, fp2.Hash, fp3.Hash, "binary mtime change must change the fingerprint")
}

func TestCompute_EnvVarsChangeFingerprint(t *testing.T) {
	envKeys := []string{"TF_CLI_ARGS", "TF_CLI_ARGS_init", "TF_PLUGIN_CACHE_DIR", "TF_DATA_DIR"}

	for _, key := range envKeys {
		t.Run(key, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "main.tf", "v1")

			in := baseInputs(dir)
			in.EnvLookup = func(k string) string {
				if k == key {
					return "v1"
				}
				return ""
			}
			fp1, err := Compute(in)
			require.NoError(t, err)

			in.EnvLookup = func(k string) string {
				if k == key {
					return "v2"
				}
				return ""
			}
			fp2, err := Compute(in)
			require.NoError(t, err)

			assert.NotEqual(t, fp1.Hash, fp2.Hash, "changing %s must change the fingerprint", key)
		})
	}
}

func TestCompute_ExtraChangesFingerprint(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.tf", "v1")

	in := baseInputs(dir)
	in.Extra = map[string]string{"TF_VAR_foo": "v1"}
	fp1, err := Compute(in)
	require.NoError(t, err)

	in.Extra = map[string]string{"TF_VAR_foo": "v2"}
	fp2, err := Compute(in)
	require.NoError(t, err)

	assert.NotEqual(t, fp1.Hash, fp2.Hash)
}

func TestCompute_IgnoresSubdirectoriesAndUnrelatedFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.tf", "v1")
	in := baseInputs(dir)

	fp1, err := Compute(in)
	require.NoError(t, err)

	writeFile(t, dir, "README.md", "docs")
	writeFile(t, dir, filepath.Join("modules", "nested", "nested.tf"), "nested content")

	fp2, err := Compute(in)
	require.NoError(t, err)

	assert.Equal(t, fp1.Hash, fp2.Hash, "unrelated root files and nested subdirectories must not affect the fingerprint")
}

func TestCompute_BackendHashOnlyChangesForBackendRelevantFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.tf", "resource \"null_resource\" \"x\" {}")
	in := baseInputs(dir)

	fp1, err := Compute(in)
	require.NoError(t, err)

	// An unrelated edit to main.tf must not change BackendHash.
	writeFile(t, dir, "main.tf", "resource \"null_resource\" \"y\" {}")
	fp2, err := Compute(in)
	require.NoError(t, err)
	assert.Equal(t, fp1.BackendHash, fp2.BackendHash, "a non-backend edit must not change BackendHash")
	assert.NotEqual(t, fp1.Hash, fp2.Hash, "a non-backend edit must still change Hash")

	// Adding backend.tf.json must change BackendHash.
	writeFile(t, dir, "backend.tf.json", `{"terraform":{"backend":{"s3":{}}}}`)
	fp3, err := Compute(in)
	require.NoError(t, err)
	assert.NotEqual(t, fp2.BackendHash, fp3.BackendHash, "adding backend.tf.json must change BackendHash")

	// Adding an HCL backend block in a root file must also change BackendHash.
	dir2 := t.TempDir()
	writeFile(t, dir2, "main.tf", "resource \"null_resource\" \"x\" {}")
	in2 := baseInputs(dir2)
	in2.Binary = in.Binary
	fpBefore, err := Compute(in2)
	require.NoError(t, err)

	writeFile(t, dir2, "backend.tf", `terraform {
  backend "s3" {}
}`)
	fpAfter, err := Compute(in2)
	require.NoError(t, err)
	assert.NotEqual(t, fpBefore.BackendHash, fpAfter.BackendHash)
}

func TestCompute_RelativePathIndependence(t *testing.T) {
	dir1 := filepath.Join(t.TempDir(), "component")
	dir2 := filepath.Join(t.TempDir(), "component")
	for _, d := range []string{dir1, dir2} {
		writeFile(t, d, "main.tf", "resource \"null_resource\" \"x\" {}")
		writeFile(t, d, ".terraform.lock.hcl", "lockcontent")
	}

	binary := testBinary()
	in1 := &Inputs{ComponentPath: dir1, Binary: binary}
	in2 := &Inputs{ComponentPath: dir2, Binary: binary}

	fp1, err := Compute(in1)
	require.NoError(t, err)
	fp2, err := Compute(in2)
	require.NoError(t, err)

	assert.Equal(t, fp1.Hash, fp2.Hash, "two identical component directories at different absolute paths must produce the same Hash")
}

func TestCompute_BinaryRecordFallsBackToNameOnLookupFailure(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.tf", "v1")

	in := baseInputs(dir)
	in.Binary = "atmos-test-binary-that-does-not-exist-anywhere"

	fp, err := Compute(in)
	require.NoError(t, err, "an unresolvable bare binary name must not fail fingerprint computation")
	assert.NotEmpty(t, fp.Hash)
}

func TestCompute_FingerprintErrorOnUnreadableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningfully enforceable on Windows in this way")
	}

	dir := t.TempDir()
	path := writeFile(t, dir, "main.tf", "v1")
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	in := baseInputs(dir)

	_, err := Compute(in)
	require.Error(t, err)
}
