package release

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStageUsesGoReleaserPublicNames(t *testing.T) {
	source := t.TempDir()
	dist := filepath.Join(source, "dist")
	require.NoError(t, os.MkdirAll(filepath.Join(dist, "atmos_linux_amd64_v1"), 0o700))
	binaryPath := filepath.Join("dist", "atmos_linux_amd64_v1", "atmos")
	require.NoError(t, os.WriteFile(filepath.Join(source, binaryPath), []byte("fixture binary"), 0o600))
	for _, name := range []string{"metadata.json", "atmos-sbom.spdx.json", "atmos_1.233.0_SHA256SUMS"} {
		require.NoError(t, os.WriteFile(filepath.Join(dist, name), []byte("fixture"), 0o600))
	}
	artifacts := []goreleaserArtifact{
		{Name: "atmos", Path: binaryPath, Type: 4}, // raw build output is not a public asset.
		{Name: "atmos_1.233.0_linux_amd64", Path: binaryPath, Type: 2},
		{Name: "atmos_1.233.0_SHA256SUMS", Path: filepath.Join("dist", "atmos_1.233.0_SHA256SUMS"), Type: 12},
	}
	data, err := json.Marshal(artifacts)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dist, "artifacts.json"), data, 0o600))
	destination := filepath.Join(t.TempDir(), "upload")
	require.NoError(t, (Release{}).StageArtifacts(source, destination))
	got, err := os.ReadFile(filepath.Join(destination, "atmos_1.233.0_linux_amd64"))
	require.NoError(t, err)
	require.Equal(t, "fixture binary", string(got))
	_, err = os.Stat(filepath.Join(destination, "atmos"))
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Error(t, (Release{}).StageArtifacts(source, destination), "never mix builds in an existing staging directory")
}

func TestStageRejectsPathsOutsideDist(t *testing.T) {
	source := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(source, "dist"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(source, "secret"), []byte("must not upload"), 0o600))
	destination := t.TempDir()
	for _, artifact := range []goreleaserArtifact{
		{Name: "safe", Path: "secret"},
		{Name: "..", Path: "secret"},
		{Name: "../escape", Path: "secret"},
	} {
		require.Error(t, stageArtifact(source, destination, artifact))
	}
	entries, err := os.ReadDir(destination)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestStageRejectsIncompleteAndConflictingBuilds(t *testing.T) {
	for _, mode := range []string{"missing manifest", "invalid manifest", "missing binary", "duplicate public name", "missing metadata", "missing SBOM"} {
		t.Run(mode, func(t *testing.T) {
			source := t.TempDir()
			dist := filepath.Join(source, "dist")
			require.NoError(t, os.Mkdir(dist, 0o700))
			for _, name := range []string{"binary", "metadata.json", "atmos-sbom.spdx.json"} {
				require.NoError(t, os.WriteFile(filepath.Join(dist, name), []byte("build output"), 0o600))
			}
			artifact := goreleaserArtifact{Name: "atmos_1.233.0_linux_amd64", Path: "dist/binary", Type: artifactUploadBinary}
			artifacts := []goreleaserArtifact{artifact}
			if mode == "duplicate public name" {
				artifacts = append(artifacts, artifact)
			}
			data, err := json.Marshal(artifacts)
			require.NoError(t, err)
			if mode == "invalid manifest" {
				data = []byte("{")
			}
			if mode != "missing manifest" {
				require.NoError(t, os.WriteFile(filepath.Join(dist, "artifacts.json"), data, 0o600))
			}
			missing := map[string]string{"missing binary": "binary", "missing metadata": "metadata.json", "missing SBOM": "atmos-sbom.spdx.json"}
			if name := missing[mode]; name != "" {
				require.NoError(t, os.Remove(filepath.Join(dist, name)))
			}
			destination := filepath.Join(t.TempDir(), "upload")
			err = (Release{}).StageArtifacts(source, destination)
			require.Error(t, err)
			if mode == "duplicate public name" {
				require.ErrorIs(t, err, os.ErrExist, "never overwrite an asset with another artifact")
			} else if mode != "invalid manifest" {
				require.ErrorIs(t, err, os.ErrNotExist, "incomplete builds cannot proceed to publication")
			}
		})
	}
}

func TestStageRejectsSymlinkEscape(t *testing.T) {
	source := t.TempDir()
	dist := filepath.Join(source, "dist")
	require.NoError(t, os.Mkdir(dist, 0o700))
	outside := filepath.Join(source, "secret")
	require.NoError(t, os.WriteFile(outside, []byte("must not upload"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(dist, "binary")))
	destination := t.TempDir()
	err := stageArtifact(source, destination, goreleaserArtifact{Name: "atmos", Path: filepath.Join(dist, "binary")})
	require.ErrorIs(t, err, ErrReleaseVerification)
	entries, err := os.ReadDir(destination)
	require.NoError(t, err)
	require.Empty(t, entries, "a manifest cannot upload files outside the build directory via symlinks")
}
