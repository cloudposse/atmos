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

// TestCompute_ExplicitVarFileDoesNotCollideWithComponentLocalVarFile guards against a regression
// where an external, explicit -var-file (in.VarFile) and a component-local terraform.tfvars with
// the same base name would collide on the same fingerprint record key (filepath.Base), silently
// dropping one of them -- so an edit to the dropped file would leave the fingerprint unchanged.
// Both files must independently affect the fingerprint.
func TestCompute_ExplicitVarFileDoesNotCollideWithComponentLocalVarFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.tf", "v1")
	// Component-local var file, same base name ("terraform.tfvars") as the explicit one below.
	writeFile(t, dir, "terraform.tfvars", "local-v1")

	// Explicit var file lives outside the component directory but shares the same base name.
	externalDir := t.TempDir()
	explicitVarFile := writeFile(t, externalDir, "terraform.tfvars", "explicit-v1")

	in := baseInputs(dir)
	in.PassVars = true
	in.VarFile = explicitVarFile

	fp1, err := Compute(in)
	require.NoError(t, err)

	// Editing the explicit var file alone must change the fingerprint.
	writeFile(t, externalDir, "terraform.tfvars", "explicit-v2")
	fp2, err := Compute(in)
	require.NoError(t, err)
	assert.NotEqual(t, fp1.Hash, fp2.Hash, "editing the explicit var file must change the fingerprint")

	// Editing the component-local var file alone must also change the fingerprint.
	writeFile(t, dir, "terraform.tfvars", "local-v2")
	fp3, err := Compute(in)
	require.NoError(t, err)
	assert.NotEqual(t, fp2.Hash, fp3.Hash, "editing the component-local var file must change the fingerprint")
}

// TestCompute_CLIConfigFileHashesBothTFAndTofuVars guards against a regression where
// cliConfigRecord only ever hashed TF_CLI_CONFIG_FILE, even when TOFU_CLI_CONFIG_FILE (the
// variable OpenTofu actually prefers) points at a different, readable file -- a change to the
// TOFU_CLI_CONFIG_FILE content would then leave the fingerprint unchanged.
func TestCompute_CLIConfigFileHashesBothTFAndTofuVars(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.tf", "v1")

	tfConfigDir := t.TempDir()
	tofuConfigDir := t.TempDir()
	tfConfigPath := writeFile(t, tfConfigDir, "tf.tfrc", "tf-content-v1")
	tofuConfigPath := writeFile(t, tofuConfigDir, "tofu.tfrc", "tofu-content-v1")

	in := baseInputs(dir)
	in.EnvLookup = func(key string) (string, bool) {
		switch key {
		case envTFCLIConfigFile:
			return tfConfigPath, true
		case envTofuCLIConfigFile:
			return tofuConfigPath, true
		default:
			return "", false
		}
	}

	fp1, err := Compute(in)
	require.NoError(t, err)

	// Changing only the TOFU_CLI_CONFIG_FILE content must change the fingerprint, even though
	// TF_CLI_CONFIG_FILE is also set and unchanged.
	writeFile(t, tofuConfigDir, "tofu.tfrc", "tofu-content-v2")
	fp2, err := Compute(in)
	require.NoError(t, err)
	assert.NotEqual(t, fp1.Hash, fp2.Hash, "a TOFU_CLI_CONFIG_FILE content change must change the fingerprint even when TF_CLI_CONFIG_FILE is also set")
}

func TestCompute_CLIConfigFileContentNotPath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.tf", "v1")

	path1 := writeFile(t, t.TempDir(), "cli-config-1.tfrc", "same content")
	path2 := writeFile(t, t.TempDir(), "cli-config-2.tfrc", "same content")

	in1 := baseInputs(dir)
	in1.EnvLookup = func(key string) (string, bool) {
		if key == "TF_CLI_CONFIG_FILE" {
			return path1, true
		}
		return "", false
	}
	in2 := baseInputs(dir)
	in2.EnvLookup = func(key string) (string, bool) {
		if key == "TF_CLI_CONFIG_FILE" {
			return path2, true
		}
		return "", false
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
			in.EnvLookup = func(k string) (string, bool) {
				if k == key {
					return "v1", true
				}
				return "", false
			}
			fp1, err := Compute(in)
			require.NoError(t, err)

			in.EnvLookup = func(k string) (string, bool) {
				if k == key {
					return "v2", true
				}
				return "", false
			}
			fp2, err := Compute(in)
			require.NoError(t, err)

			assert.NotEqual(t, fp1.Hash, fp2.Hash, "changing %s must change the fingerprint", key)
		})
	}
}

// TestCompute_EnvLookupExplicitEmptyValueIsHonored guards against a regression where an
// EnvLookup reporting a key present with an explicit empty value ("", true) was treated the same
// as the key being absent, silently falling back to this process's own os.Getenv value instead of
// the explicit override the subprocess will actually see. A change to the *ambient* process env
// var must NOT affect the fingerprint once EnvLookup explicitly reports the key as overridden to
// "".
func TestCompute_EnvLookupExplicitEmptyValueIsHonored(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "main.tf", "v1")

	t.Setenv("TF_CLI_ARGS", "ambient-value-v1")

	in := baseInputs(dir)
	in.EnvLookup = func(k string) (string, bool) {
		if k == "TF_CLI_ARGS" {
			return "", true
		}
		return "", false
	}

	fp1, err := Compute(in)
	require.NoError(t, err)

	// Changing only the ambient process env var (which EnvLookup explicitly overrides to "")
	// must NOT change the fingerprint: the explicit override always wins.
	t.Setenv("TF_CLI_ARGS", "ambient-value-v2")
	fp2, err := Compute(in)
	require.NoError(t, err)
	assert.Equal(t, fp1.Hash, fp2.Hash, "an explicit empty EnvLookup override must not fall back to the ambient os.Getenv value")
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
