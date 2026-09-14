package autoinit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// newComponent creates a minimal, fingerprint-able component directory (a single main.tf) and
// returns Inputs pointed at it, with DataDir explicit so tests don't depend on TF_DATA_DIR.
func newComponent(t *testing.T) (*Inputs, string) {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte("resource \"null_resource\" \"x\" {}"), 0o644))
	dataDir := filepath.Join(dir, ".terraform")
	require.NoError(t, os.MkdirAll(dataDir, 0o755))
	return &Inputs{ComponentPath: dir, DataDir: dataDir, Binary: testBinary()}, dataDir
}

func TestDecide_ModeNever(t *testing.T) {
	in, _ := newComponent(t)

	d := Decide(&Request{Mode: schema.TerraformInitModeNever, Inputs: in})

	assert.False(t, d.RunInit)
	assert.Equal(t, ReasonModeNever, d.Reason)
}

func TestDecide_ModeAlways(t *testing.T) {
	in, _ := newComponent(t)

	d := Decide(&Request{Mode: schema.TerraformInitModeAlways, Inputs: in})

	assert.True(t, d.RunInit)
	assert.Equal(t, ReasonModeAlways, d.Reason)
}

func TestDecide_Force(t *testing.T) {
	in, _ := newComponent(t)

	d := Decide(&Request{Force: true, Inputs: in})

	assert.True(t, d.RunInit)
	assert.Equal(t, ReasonForced, d.Reason)
	assert.True(t, d.Reconfigure, "auto reconfigure must be true when init is forced")
}

func TestDecide_NilInputs(t *testing.T) {
	d := Decide(&Request{})

	assert.True(t, d.RunInit)
	assert.Equal(t, ReasonNoInputs, d.Reason)
	assert.True(t, d.Reconfigure, "auto reconfigure must default true when it cannot be determined")
}

func TestDecide_NoMarker(t *testing.T) {
	in, _ := newComponent(t)

	d := Decide(&Request{Inputs: in})

	assert.True(t, d.RunInit)
	assert.Equal(t, ReasonNoMarker, d.Reason)
}

func TestDecide_SchemaVersionMismatch(t *testing.T) {
	in, dataDir := newComponent(t)
	fp, err := Compute(in)
	require.NoError(t, err)
	require.NoError(t, WriteMarker(MarkerPath(dataDir), &Marker{
		SchemaVersion:      MarkerSchemaVersion + 1,
		Fingerprint:        fp.Hash,
		BackendFingerprint: fp.BackendHash,
	}))

	d := Decide(&Request{Inputs: in})

	assert.True(t, d.RunInit)
	assert.Equal(t, ReasonSchemaVersion, d.Reason)
}

func TestDecide_ProvidersMissing(t *testing.T) {
	in, dataDir := newComponent(t)
	require.NoError(t, os.WriteFile(filepath.Join(in.ComponentPath, lockFileName()), []byte(validLockFile), 0o644))
	fp, err := Compute(in)
	require.NoError(t, err)
	require.NoError(t, WriteMarker(MarkerPath(dataDir), validMarker(fp)))

	d := Decide(&Request{Inputs: in})

	assert.True(t, d.RunInit)
	assert.Equal(t, ReasonProvidersMissing, d.Reason)
}

func TestDecide_ModulesMissing(t *testing.T) {
	in, dataDir := newComponent(t)
	require.NoError(t, os.WriteFile(filepath.Join(in.ComponentPath, "main.tf"), []byte(`module "vpc" { source = "./vpc" }`), 0o644))
	fp, err := Compute(in)
	require.NoError(t, err)
	require.NoError(t, WriteMarker(MarkerPath(dataDir), validMarker(fp)))

	d := Decide(&Request{Inputs: in})

	assert.True(t, d.RunInit)
	assert.Equal(t, ReasonModulesMissing, d.Reason)
}

func TestDecide_BackendStateMissing(t *testing.T) {
	in, dataDir := newComponent(t)
	require.NoError(t, os.WriteFile(filepath.Join(in.ComponentPath, "backend.tf.json"), []byte(`{"terraform":{"backend":{"s3":{}}}}`), 0o644))
	fp, err := Compute(in)
	require.NoError(t, err)
	require.NoError(t, WriteMarker(MarkerPath(dataDir), validMarker(fp)))

	d := Decide(&Request{Inputs: in})

	assert.True(t, d.RunInit)
	assert.Equal(t, ReasonBackendStateMissing, d.Reason)
}

func TestDecide_FingerprintChanged(t *testing.T) {
	in, dataDir := newComponent(t)
	fp, err := Compute(in)
	require.NoError(t, err)
	require.NoError(t, WriteMarker(MarkerPath(dataDir), validMarker(fp)))

	require.NoError(t, os.WriteFile(filepath.Join(in.ComponentPath, "main.tf"), []byte("resource \"null_resource\" \"changed\" {}"), 0o644))

	d := Decide(&Request{Inputs: in})

	assert.True(t, d.RunInit)
	assert.Equal(t, ReasonFingerprintChanged, d.Reason)
}

func TestDecide_UpToDate(t *testing.T) {
	in, dataDir := newComponent(t)
	fp, err := Compute(in)
	require.NoError(t, err)
	require.NoError(t, WriteMarker(MarkerPath(dataDir), validMarker(fp)))

	d := Decide(&Request{Inputs: in})

	assert.False(t, d.RunInit)
	assert.Equal(t, ReasonUpToDate, d.Reason)
	assert.False(t, d.Reconfigure, "backend fingerprint has not changed, so auto reconfigure must be false")
}

func TestDecide_FingerprintError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningfully enforceable on Windows in this way")
	}

	in, _ := newComponent(t)
	path := filepath.Join(in.ComponentPath, "main.tf")
	require.NoError(t, os.Chmod(path, 0o000))
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	d := Decide(&Request{Inputs: in})

	assert.True(t, d.RunInit)
	assert.Equal(t, ReasonFingerprintError, d.Reason)
}

func TestDecide_UpgradeAlways(t *testing.T) {
	in, dataDir := newComponent(t)
	fp, err := Compute(in)
	require.NoError(t, err)
	require.NoError(t, WriteMarker(MarkerPath(dataDir), validMarker(fp)))

	d := Decide(&Request{Inputs: in, Upgrade: schema.TerraformInitUpgradeAlways})

	assert.True(t, d.Upgrade)
}

func TestDecide_UpgradeAutoIsFalseByDefault(t *testing.T) {
	in, dataDir := newComponent(t)
	fp, err := Compute(in)
	require.NoError(t, err)
	require.NoError(t, WriteMarker(MarkerPath(dataDir), validMarker(fp)))

	d := Decide(&Request{Inputs: in, Upgrade: schema.TerraformInitUpgradeAuto})

	assert.False(t, d.Upgrade)
}

func TestDecide_ReconfigureAuto(t *testing.T) {
	t.Run("no marker turns reconfigure on", func(t *testing.T) {
		in, _ := newComponent(t)

		d := Decide(&Request{Inputs: in})

		assert.True(t, d.Reconfigure)
	})

	t.Run("backend fingerprint changed turns reconfigure on", func(t *testing.T) {
		in, dataDir := newComponent(t)
		fp, err := Compute(in)
		require.NoError(t, err)
		m := validMarker(fp)
		m.BackendFingerprint = "stale-backend-hash"
		require.NoError(t, WriteMarker(MarkerPath(dataDir), m))

		d := Decide(&Request{Inputs: in})

		assert.False(t, d.RunInit, "only the backend fingerprint changed, not the overall hash")
		assert.True(t, d.Reconfigure)
	})

	t.Run("only non-backend change leaves reconfigure off", func(t *testing.T) {
		in, dataDir := newComponent(t)
		fp, err := Compute(in)
		require.NoError(t, err)
		require.NoError(t, WriteMarker(MarkerPath(dataDir), validMarker(fp)))

		d := Decide(&Request{Inputs: in})

		assert.False(t, d.RunInit)
		assert.False(t, d.Reconfigure)
	})

	t.Run("reconfigure always forces true even when up to date", func(t *testing.T) {
		in, dataDir := newComponent(t)
		fp, err := Compute(in)
		require.NoError(t, err)
		require.NoError(t, WriteMarker(MarkerPath(dataDir), validMarker(fp)))

		d := Decide(&Request{Inputs: in, Reconfigure: schema.TerraformInitReconfigureAlways})

		assert.True(t, d.Reconfigure)
	})

	t.Run("reconfigure never forces false even without a marker", func(t *testing.T) {
		in, _ := newComponent(t)

		d := Decide(&Request{Inputs: in, Reconfigure: schema.TerraformInitReconfigureNever})

		assert.False(t, d.Reconfigure)
	})

	t.Run("mode always still computes auto reconfigure from the backend fingerprint", func(t *testing.T) {
		in, dataDir := newComponent(t)
		fp, err := Compute(in)
		require.NoError(t, err)
		require.NoError(t, WriteMarker(MarkerPath(dataDir), validMarker(fp)))

		d := Decide(&Request{Mode: schema.TerraformInitModeAlways, Inputs: in})

		assert.True(t, d.RunInit)
		assert.Equal(t, ReasonModeAlways, d.Reason)
		assert.False(t, d.Reconfigure, "backend fingerprint unchanged, even under mode=always")
	})
}

// lockFileName avoids re-declaring the lock file's conventional name in this test file.
func lockFileName() string {
	return ".terraform.lock.hcl"
}

// validMarker builds a Marker that will be treated as up to date for fp, so a test can focus on a
// single precondition it deliberately violates.
func validMarker(fp Fingerprint) *Marker {
	return &Marker{
		SchemaVersion:      MarkerSchemaVersion,
		Fingerprint:        fp.Hash,
		BackendFingerprint: fp.BackendHash,
	}
}
