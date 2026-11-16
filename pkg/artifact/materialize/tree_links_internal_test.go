package materialize

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

func linkEntry(name, target string) *artifact.Entry {
	return &artifact.Entry{Path: name, Type: artifact.EntrySymlink, Target: target}
}

func linkPaths(links []*artifact.Entry) []string {
	paths := make([]string, 0, len(links))
	for _, link := range links {
		paths = append(paths, link.Path)
	}
	return paths
}

func TestOrderLinks_CreatesALinkAfterTheLinkItsTargetPassesThrough(t *testing.T) {
	tests := []struct {
		name  string
		links []*artifact.Entry
		want  []string
	}{
		{"independent links keep path order", []*artifact.Entry{linkEntry("b", "x"), linkEntry("a", "y")}, []string{"a", "b"}},
		{"a link to a directory that sorts after it", []*artifact.Entry{linkEntry("alias", "z-dir")}, []string{"alias"}},
		{"a chain is created from its end", []*artifact.Entry{linkEntry("a", "b"), linkEntry("b", "c"), linkEntry("c", "dir")}, []string{"c", "b", "a"}},
		{"a target below a link", []*artifact.Entry{linkEntry("x", "y/sub"), linkEntry("y", "dir")}, []string{"y", "x"}},
		{"a relative target in a subdirectory", []*artifact.Entry{linkEntry("d/a", "../e"), linkEntry("e", "dir")}, []string{"e", "d/a"}},
		// "m" reaches deep/c only after "z" is expanded, because the parent component applies to z's target.
		{"a parent component after a link", []*artifact.Entry{linkEntry("m", "z/../c"), linkEntry("z", "deep/b"), linkEntry("deep/c", "dir")}, []string{"deep/c", "z", "m"}},
		// "t" passes through "r" (a link to "s"), and "s" is a link that no prefix of t's target names.
		{"every link met while expanding the target", []*artifact.Entry{linkEntry("t", "r/f"), linkEntry("r", "s/sub"), linkEntry("s", "dir")}, []string{"s", "r", "t"}},
		{"a cycle keeps path order and terminates", []*artifact.Entry{linkEntry("q", "p"), linkEntry("p", "q")}, []string{"p", "q"}},
		{"a link to itself terminates", []*artifact.Entry{linkEntry("s", "s")}, []string{"s"}},
		{"no links", nil, []string{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			original := linkPaths(tc.links)
			got := linkPaths(orderLinks(tc.links))
			assert.Equal(t, tc.want, got)
			assert.Equal(t, original, linkPaths(tc.links), "the caller's slice is not reordered")
		})
	}
}

func TestTree_CreatesSymlinksOnlyAfterEveryOtherMemberExists(t *testing.T) {
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	alias := linkEntry("alias", "z-dir")
	zdir := &artifact.Entry{Path: "z-dir", Type: artifact.EntryDirectory, Mode: 0o755}
	target := newTree(root, &artifact.Record{Inventory: []artifact.Entry{*alias, *zdir}})

	require.NoError(t, target.symlink(alias))
	_, err = os.Lstat(filepath.Join(dir, "alias"))
	require.ErrorIs(t, err, os.ErrNotExist, "recording a link must not create it yet")

	require.NoError(t, target.mkdir(zdir))
	requireSymlinkSupport(t)
	require.NoError(t, target.createLinks(t.Context()))
	info, err := os.Lstat(filepath.Join(dir, "alias"))
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink)
	entries, err := os.ReadDir(filepath.Join(dir, "alias"))
	require.NoError(t, err, "the link must be traversable as a directory")
	assert.Empty(t, entries)
}

func TestTree_OwnerAccessKeepsATemporaryTreeReadable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skipf("POSIX permission bits are not enforced on %s", runtime.GOOS)
	}
	body := []byte("sealed")
	tests := []struct {
		name        string
		ownerAccess bool
		wantFile    os.FileMode
		wantDir     os.FileMode
	}{
		{"recorded modes are applied as declared", false, 0o000, 0o000},
		{"owner access is kept on top of the recorded modes", true, 0o400, 0o700},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			root, err := os.OpenRoot(dir)
			require.NoError(t, err)
			t.Cleanup(func() { _ = root.Close() })
			record := &artifact.Record{Inventory: []artifact.Entry{
				{Path: "d", Type: artifact.EntryDirectory, Mode: 0},
				{Path: "d/f", Type: artifact.EntryFile, Size: int64(len(body)), Mode: 0, Digest: digest.FromBytes(body)},
			}}
			target := newTree(root, record)
			target.ownerAccess = tc.ownerAccess
			for index := range record.Inventory {
				entry := &record.Inventory[index]
				_, err := target.entryFor(entry.Path)
				require.NoError(t, err)
				if entry.Type == artifact.EntryDirectory {
					require.NoError(t, target.mkdir(entry))
					continue
				}
				require.NoError(t, target.writeFile(entry, bytes.NewReader(body)))
			}

			require.NoError(t, target.complete())
			assert.Zero(t, record.Inventory[0].Mode, "the Record keeps the modes it declared")
			assert.Zero(t, record.Inventory[1].Mode, "the Record keeps the modes it declared")
			t.Cleanup(func() {
				_ = os.Chmod(filepath.Join(dir, "d"), 0o700)
				_ = os.Chmod(filepath.Join(dir, "d", "f"), 0o600)
			})

			dirInfo, err := os.Stat(filepath.Join(dir, "d"))
			require.NoError(t, err)
			assert.Equal(t, tc.wantDir, dirInfo.Mode().Perm())
			if tc.ownerAccess {
				fileInfo, err := os.Stat(filepath.Join(dir, "d", "f"))
				require.NoError(t, err)
				assert.Equal(t, tc.wantFile, fileInfo.Mode().Perm())
				data, err := os.ReadFile(filepath.Join(dir, "d", "f"))
				require.NoError(t, err)
				assert.Equal(t, body, data)
			}
		})
	}
}

// requireSymlinkSupport skips the test only when the platform cannot create symlinks at all. It runs
// its own probe so a failure of the operation under test fails the test instead of skipping it.
func requireSymlinkSupport(t *testing.T) {
	t.Helper()
	probe := t.TempDir()
	if err := os.WriteFile(filepath.Join(probe, "target.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write the symlink probe target: %v", err)
	}
	if err := os.Symlink("target.txt", filepath.Join(probe, "link")); err != nil {
		t.Skipf("this platform cannot create symlinks: %v", err)
	}
}

// A link whose target cannot be resolved for a reason other than a loop is rejected, because an unknown
// target cannot be shown to stay inside the package; a loop never leaves the root, so it is accepted.
func TestCheckLinkContainment_ResolutionFailures(t *testing.T) {
	requireSymlinkSupport(t)
	tests := []struct {
		name    string
		links   map[string]string
		wantErr bool
	}{
		// Capture accepts a link whose target runs through a regular file while its recorded path stays inside
		// the tree, so materialization accepts it too; one that leaves the tree is rejected.
		{name: "through a regular file, inside the tree", links: map[string]string{"dangling": "file.txt/child"}},
		{name: "through a regular file, leaving the tree", links: map[string]string{"bad": "file.txt/../../outside"}, wantErr: true},
		{name: "loop", links: map[string]string{"a": "b", "b": "a"}},
		{name: "inside the root", links: map[string]string{"ok": "file.txt"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stage := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(stage, "file.txt"), []byte("x"), 0o600))
			record := &artifact.Record{}
			for name, target := range tt.links {
				require.NoError(t, os.Symlink(target, filepath.Join(stage, name)))
				record.Inventory = append(record.Inventory, artifact.Entry{Path: name, Type: artifact.EntrySymlink, Target: target})
			}
			err := checkLinkContainment(stage, record)
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrArtifactUnsafePath)
				return
			}
			require.NoError(t, err)
		})
	}
}
