package tests

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/oci/ocitest"
)

const scaffoldOCITestTemplate = `apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: oci-template
spec:
  fields:
    - name: project_name
      type: input
      default: demo
`

// TestScaffoldGenerate_OCISource proves `atmos scaffold generate` can pull a
// template directly from an oci:// registry reference, the same way `atmos
// vendor pull` and JIT terraform source provisioning (see
// TestJITSource_OCIScheme) already do -- this is the CLI-level counterpart
// to pkg/generator/source's unit-level TestResolve_OCISuccess/
// TestHydrate_OCIStub, driving the real command end-to-end against an
// in-process fake registry (no real network or registry dependency).
//
// Runs the real atmos binary as a subprocess (via ensureAtmosRunner/
// runAtmosForScaffoldTest, the same helpers tests/scaffold_render_test.go
// uses), not an in-process cmd.Execute() call: cobra/viper flag state on
// cmd.RootCmd persists across Execute() calls within one test binary (no
// cmd.NewTestKit equivalent exists for this external tests package), so an
// in-process call here could silently inherit --git/--no-git state from a
// different test's earlier invocation.
func TestScaffoldGenerate_OCISource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows: subprocess CLI invocation can stall while reading from the loopback OCI test registry")
	}
	ensureAtmosRunner(t)

	root := t.TempDir()
	imageRef := ocitest.NewRegistry(t, "test/scaffold:v1", map[string]string{
		"scaffold.yaml": scaffoldOCITestTemplate,
		"README.md":     "# OCI_SOURCE_MARKER\n",
	})

	target := filepath.Join(root, "output")
	runAtmosForScaffoldTest(t, root, map[string]string{
		"XDG_CACHE_HOME": filepath.Join(t.TempDir(), ".cache"),
	}, 2*time.Minute,
		"scaffold", "generate", "oci://"+imageRef, target,
		"--defaults",
		"--no-git",
	)

	content, err := os.ReadFile(filepath.Join(target, "README.md"))
	require.NoError(t, err, "README.md should exist in the generated target (pulled from the OCI source)")
	require.Contains(t, string(content), "OCI_SOURCE_MARKER")
}

// TestScaffoldGenerate_OCISourceUpdate proves `--update`'s 3-way merge works
// identically for an OCI-sourced project as it already does for git/https/
// local sources -- expected per this feature's design, since the merge base
// is read from the *generated project's own* git history
// (pkg/generator/engine/merge_update.go's SetupGitStorage opens a git repo
// at the target directory, never at a clone of the template source), never
// from re-fetching/re-checking-out the template at a ref. "Theirs" is
// whatever Resolve fetches fresh on the --update run -- from OCI, now that
// it's wired up, same as any other source type. See TestScaffoldGenerate_OCISource's
// doc comment for why this drives a real atmos subprocess rather than
// cmd.Execute() -- doubly so here, since this test invokes the CLI twice.
func TestScaffoldGenerate_OCISourceUpdate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows: subprocess CLI invocation can stall while reading from the loopback OCI test registry")
	}
	ensureAtmosRunner(t)

	root := t.TempDir()
	imageRef := ocitest.NewRegistry(t, "test/update:v1", map[string]string{
		"scaffold.yaml": scaffoldOCITestTemplate,
		"file.txt":      "original content\n",
	})

	target := filepath.Join(root, "output")
	env := map[string]string{
		"XDG_CACHE_HOME": filepath.Join(t.TempDir(), ".cache"),
	}
	runAtmosForScaffoldTest(t, root, env, 2*time.Minute,
		"scaffold", "generate", "oci://"+imageRef, target,
		"--defaults",
		"--git",
	)

	before, err := os.ReadFile(filepath.Join(target, "file.txt"))
	require.NoError(t, err)
	require.Contains(t, string(before), "original content")

	// Republish the same repo:tag with changed content -- simulating a
	// template author pushing an update -- and confirm --update picks it
	// up. No local edits are made to the generated file, so the merge base
	// (from the target's own git history) equals "ours" (the unmodified
	// working tree); the merge should cleanly fast-forward to "theirs" (the
	// freshly re-fetched, changed template) with no conflict.
	pushOCIUpdate(t, imageRef, map[string]string{
		"scaffold.yaml": scaffoldOCITestTemplate,
		"file.txt":      "updated content\n",
	})

	runAtmosForScaffoldTest(t, root, env, 2*time.Minute,
		"scaffold", "generate", "oci://"+imageRef, target,
		"--defaults",
		"--update",
	)

	after, err := os.ReadFile(filepath.Join(target, "file.txt"))
	require.NoError(t, err)
	require.Contains(t, string(after), "updated content", "--update must pick up the republished OCI template content")
}

// TestInit_OCISource proves `atmos init` can pull a template directly from an
// oci:// registry reference, exercising the same executeInit -> selectTemplate
// -> source.Hydrate flow (cmd/init/init.go) that `atmos scaffold generate`
// exercises for its own OCI sources in TestScaffoldGenerate_OCISource above --
// that test alone was scaffold-generate-only CLI coverage; this is the `atmos
// init` counterpart.
//
// atmos init has no --defaults flag (unlike scaffold generate): the
// non-interactive equivalent here is --interactive=false, which also makes
// executeInit fall back to the scaffold.yaml fields' declared defaults (see
// ExecuteWithBaseRef's useDefaults parameter in pkg/generator/ui/ui.go, wired
// from !opts.interactive in cmd/init/init.go's RunE). --no-git is passed
// explicitly because atmos init defaults --git to true (scaffold generate
// defaults it to false); nothing OCI-specific is exercised by letting init
// create a git repo, so it's skipped to keep the test fast and hermetic.
//
// See TestScaffoldGenerate_OCISource's doc comment for why this drives a real
// atmos subprocess rather than cmd.Execute() in-process.
func TestInit_OCISource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on Windows: subprocess CLI invocation can stall while reading from the loopback OCI test registry")
	}
	ensureAtmosRunner(t)

	root := t.TempDir()
	imageRef := ocitest.NewRegistry(t, "test/init:v1", map[string]string{
		"scaffold.yaml": scaffoldOCITestTemplate,
		"README.md":     "# OCI_SOURCE_MARKER\n",
	})

	target := filepath.Join(root, "output")
	runAtmosForScaffoldTest(t, root, map[string]string{
		"XDG_CACHE_HOME": filepath.Join(t.TempDir(), ".cache"),
	}, 2*time.Minute,
		"init", "oci://"+imageRef, target,
		"--interactive=false",
		"--no-git",
	)

	content, err := os.ReadFile(filepath.Join(target, "README.md"))
	require.NoError(t, err, "README.md should exist in the generated target (pulled from the OCI source via atmos init)")
	require.Contains(t, string(content), "OCI_SOURCE_MARKER")
}

// pushOCIUpdate re-pushes imageRef (a "host:port/repo:tag" string, as
// returned by ocitest.NewRegistry) against the SAME already-running
// in-process registry server, overwriting its manifest with a new
// single-layer image built from files -- simulating a template author
// publishing a changed artifact under the same tag. No exported "push
// again" helper exists in ocitest (NewRegistry always starts a new
// server), so this mirrors its internal pushImage/buildTar logic directly
// against the registry host embedded in imageRef.
func pushOCIUpdate(t *testing.T, imageRef string, files map[string]string) {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for path, content := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: path,
			Mode: 0o644,
			Size: int64(len(content)),
		}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	tarBytes := buf.Bytes()

	layer, err := tarball.LayerFromOpener(func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(tarBytes)), nil
	})
	require.NoError(t, err)

	img, err := mutate.AppendLayers(empty.Image, layer)
	require.NoError(t, err)

	ref, err := name.ParseReference(imageRef)
	require.NoError(t, err)
	require.NoError(t, remote.Write(ref, img))
}
