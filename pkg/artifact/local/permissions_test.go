package local

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
)

func modeOf(t *testing.T, path string) fs.FileMode {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	return info.Mode().Perm()
}

// tgzMemberMode returns the mode recorded in the archive for one member.
func tgzMemberMode(t *testing.T, archivePath, member string) int64 {
	t.Helper()
	file, err := os.Open(archivePath)
	require.NoError(t, err)
	defer file.Close()
	gz, err := gzip.NewReader(file)
	require.NoError(t, err)
	reader := tar.NewReader(gz)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			t.Fatalf("member %q not found in %s", member, archivePath)
		}
		require.NoError(t, err)
		if header.Name == member {
			return header.Mode & 0o777
		}
	}
}

func TestArchivePermissions_DefaultIs0600OnTheArchiveAndSidecars(t *testing.T) {
	fixtures.RequirePOSIXModes(t)
	repo := newRepo(t, archiveSpec(artifact.PackagingTgz))
	result, _ := publishFilePackage(t, repo)

	assert.Equal(t, fs.FileMode(0o600), modeOf(t, result.Location), "default archive permissions")
	assert.Equal(t, fs.FileMode(0o600), modeOf(t, result.Location+".metadata.json"))
	assert.Equal(t, fs.FileMode(0o600), modeOf(t, filepath.Join(result.Location+".docs", "README.md")))
	assert.Equal(t, fs.FileMode(0o700), modeOf(t, result.Location+".docs"), "the sidecar directory keeps owner traversal")
	assert.False(t, repo.modeExplicit, "a default is not an explicit request")
}

func TestArchivePermissions_ExplicitModeIsDistinctFromMemberModes(t *testing.T) {
	fixtures.RequirePOSIXModes(t)
	spec := archiveSpec(artifact.PackagingTgz)
	spec = spec.withPermissions("0640")
	repo := newRepo(t, spec)
	require.True(t, repo.modeExplicit)
	result, _ := publishFilePackage(t, repo)

	assert.Equal(t, fs.FileMode(0o640), modeOf(t, result.Location), "the configured mode applies to the archive file")
	assert.Equal(t, fs.FileMode(0o640), modeOf(t, result.Location+".metadata.json"))
	assert.Equal(t, fs.FileMode(0o750), modeOf(t, result.Location+".docs"))
	assert.EqualValues(t, 0o755, tgzMemberMode(t, result.Location, "bin/helper.sh"), "member modes are independent of the archive file mode")
	assert.EqualValues(t, 0o644, tgzMemberMode(t, result.Location, "README.md"))
	assert.EqualValues(t, 0o640, tgzMemberMode(t, result.Location, fixtures.UnicodeName))
}

func TestArchivePermissions_AppliedBeforeAnythingIsVisible(t *testing.T) {
	fixtures.RequirePOSIXModes(t)
	spec := archiveSpec(artifact.PackagingTar)
	spec = spec.withPermissions("0400")
	spec = spec.withOpts(withFaults(crashAt(faultAfterMetadata)))
	repo := newRepo(t, spec)
	prepared := prepare(t, repo, &prepareSpec{})
	_, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
	require.ErrorIs(t, err, errCrash)
	archivePath := filepath.Join(repo.root, "team", "app", "1.0.0.tar")
	assert.Equal(t, fs.FileMode(0o400), modeOf(t, archivePath), "even an interrupted publication never exposes a wrongly-permissioned archive")
}

// An explicit permission request the host cannot enforce fails before anything is written.
func TestArchivePermissions_UnsupportedHostFailsExplicitly(t *testing.T) {
	spec := archiveSpec(artifact.PackagingTgz)
	spec = spec.withPermissions("0600")
	repo := newRepo(t, spec)
	repo.hostModes = false

	prepared := prepare(t, repo, &prepareSpec{})
	_, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
	require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability)
	capErr, ok := artifact.AsCapabilityError(err)
	require.True(t, ok)
	assert.Equal(t, "permissions/archive-file-mode", capErr.Capability)
	assert.Contains(t, capErr.Reason, "0600")
	_, statErr := os.Stat(repo.root)
	assert.True(t, os.IsNotExist(statErr), "nothing was created before the capability check")
}

// Without an explicit request, a host that cannot enforce modes publishes with the host default.
func TestArchivePermissions_DefaultWorksWhereModesCannotBeEnforced(t *testing.T) {
	repo := newRepo(t, archiveSpec(artifact.PackagingTgz))
	repo.hostModes = false
	result, _ := publishFilePackage(t, repo)
	assert.FileExists(t, result.Location)
	assertIntact(t, repo, defaultRef())
}

// On the real host the behavior matches its capability: Windows rejects an explicit request,
// POSIX hosts honor it.
func TestArchivePermissions_RealHostBehavior(t *testing.T) {
	spec := archiveSpec(artifact.PackagingTgz)
	spec = spec.withPermissions("0600")
	repo := newRepo(t, spec)
	prepared := prepare(t, repo, &prepareSpec{})
	_, err := repo.Publish(context.Background(), prepared, artifact.PublishOptions{})
	if runtime.GOOS == "windows" {
		require.ErrorIs(t, err, errUtils.ErrArtifactUnsupportedCapability, "Windows cannot enforce POSIX archive permissions")
		return
	}
	require.NoError(t, err)
	assert.Equal(t, fs.FileMode(0o600), modeOf(t, filepath.Join(repo.root, "team", "app", "1.0.0.tgz")))
}

func TestPermissions_InvalidConfigurationIsRejected(t *testing.T) {
	for _, bad := range []string{"preserve", "0o600", "9999", "4755", "6", "rw-------", "600.0"} {
		spec := archiveSpec(artifact.PackagingTgz).withPermissions(bad).withRoot(filepath.Join(t.TempDir(), "r"))
		_, err := newRepository("x", spec.kind, spec.config(t), nil)
		require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid, bad)
	}
	dir := dirSpec().withPermissions("0600").withRoot(filepath.Join(t.TempDir(), "r"))
	_, err := newRepository("x", dir.kind, dir.config(t), nil)
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid, "permissions apply only to local archive files")
}

func TestCapabilities_AreHonest(t *testing.T) {
	dir := newRepo(t, dirSpec())
	archive := newRepo(t, archiveSpec(artifact.PackagingTgz))

	dirCaps, archiveCaps := dir.Capabilities(), archive.Capabilities()
	assert.Equal(t, []artifact.Packaging{artifact.PackagingNone}, dirCaps.Packagings)
	assert.ElementsMatch(t, []artifact.Packaging{artifact.PackagingTar, artifact.PackagingTgz, artifact.PackagingZip}, archiveCaps.Packagings)
	for _, caps := range []artifact.Capabilities{dirCaps, archiveCaps} {
		assert.True(t, caps.Read && caps.Write && caps.EnumerateNamed && caps.EnumerateCollection && caps.ConditionalUpdate)
		assert.Equal(t, artifact.DeleteObject, caps.Delete)
		assert.Equal(t, []artifact.LockScope{artifact.LockScopeHost}, caps.LockScopes, "host scope only")
		assert.Equal(t, []artifact.SubjectKind{artifact.SubjectRecord}, caps.SubjectKinds)
		assert.Empty(t, caps.EvidenceFormats, "no verifier is configured, so no evidence format is advertised")
	}
	assert.Equal(t, hostEnforcesModes(), dirCaps.PreservesModes)
	assert.True(t, archiveCaps.PreservesModes, "member modes live inside the archive on every host")

	dir.hostModes = false
	assert.False(t, dir.Capabilities().PreservesModes)
	assert.Equal(t, dir.Capabilities(), dir.Capabilities(), "capabilities are stable")
}
