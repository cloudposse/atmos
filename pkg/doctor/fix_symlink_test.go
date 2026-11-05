package doctor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	symlinkLegacyHook = "hooks:\n  example:\n    events: [before-terraform-apply]\n    command: echo\n"
	symlinkReason     = "symlinked source requires manual review; doctor does not write through symlinks"
)

// requireSymlink creates link pointing at target, skipping the test where symlinks cannot be created.
func requireSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
}

// requireStillLinked asserts that link remains a symlink to target and that target is unchanged.
func requireStillLinked(t *testing.T, link, target, content string) {
	t.Helper()
	info, err := os.Lstat(link)
	require.NoError(t, err)
	require.NotZero(t, info.Mode()&os.ModeSymlink, "%s must remain a symlink", link)
	dest, err := os.Readlink(link)
	require.NoError(t, err)
	require.Equal(t, target, dest)
	b, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, content, string(b))
}

// symlinkFinding returns the repair finding of the hook-event rule reported for the file.
func symlinkFinding(r *Report, suffix string) *Finding {
	for i := range r.Findings {
		if r.Findings[i].RuleID == ruleHookEvent && strings.HasSuffix(filepath.ToSlash(r.Findings[i].File), suffix) {
			return &r.Findings[i]
		}
	}
	return nil
}

func TestFixSymlinkedStackDoesNotBlockOtherRepairs(t *testing.T) {
	root := newProject(t)
	atmosYAML := projectFile(t, root, "atmos.yaml", ecLegacyReconfigure)
	target := projectFile(t, root, "shared/prod.yaml", "vars:\n  stage: prod\n")
	link := filepath.Join(root, "stacks", "prod.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o700))
	requireSymlink(t, target, link)

	r, err := testEngine(t).Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
	require.NoError(t, err)
	require.Empty(t, r.OperationalError)
	require.NotEqual(t, 2, r.ExitCode("none"))
	require.Equal(t, []string{"atmos.yaml"}, r.ChangedFiles)
	content, err := os.ReadFile(atmosYAML)
	require.NoError(t, err)
	require.NotContains(t, string(content), "init_run_reconfigure")
	requireStillLinked(t, link, target, "vars:\n  stage: prod\n")
}

func TestFixSymlinkedStackWithOwnFindingIsReportedNotApplied(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", ecLegacyReconfigure)
	regular := projectFile(t, root, "stacks/dev.yaml", symlinkLegacyHook)
	target := projectFile(t, root, "shared/prod.yaml", symlinkLegacyHook)
	link := filepath.Join(root, "stacks", "prod.yaml")
	requireSymlink(t, target, link)

	r, err := testEngine(t).Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
	require.NoError(t, err)
	require.Empty(t, r.OperationalError)
	require.ElementsMatch(t, []string{"atmos.yaml", filepath.ToSlash(filepath.Join("stacks", "dev.yaml"))}, toSlashAll(r.ChangedFiles))

	// The regular stack was repaired.
	b, err := os.ReadFile(regular)
	require.NoError(t, err)
	require.Contains(t, string(b), "before.terraform.apply")

	// The symlinked stack was reported with the specific reason and left alone.
	f := symlinkFinding(&r, "stacks/prod.yaml")
	require.NotNil(t, f, "%+v", r.Findings)
	require.Equal(t, statusVerificationIncomplete, f.Status)
	require.False(t, f.Fixable)
	require.Contains(t, f.Reason, symlinkReason)
	require.NotEmpty(t, f.Remediation)
	requireStillLinked(t, link, target, symlinkLegacyHook)
}

func TestFixDryRunReportsSymlinkedStackAsManual(t *testing.T) {
	root := newProject(t)
	target := projectFile(t, root, "shared/prod.yaml", symlinkLegacyHook)
	projectFile(t, root, "atmos.yaml", "{}\n")
	link := filepath.Join(root, "stacks", "prod.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o700))
	requireSymlink(t, target, link)

	r, err := testEngine(t).Run(context.Background(), Options{Root: root, Fix: true, DryRun: true, Offline: true, Only: []string{"deprecations"}})
	require.NoError(t, err)
	f := symlinkFinding(&r, "stacks/prod.yaml")
	require.NotNil(t, f, "%+v", r.Findings)
	require.Equal(t, statusVerificationIncomplete, f.Status)
	require.Contains(t, f.Reason, symlinkReason)
}

func TestFixRefusesToWriteThroughALinkCreatedAfterStaging(t *testing.T) {
	original := symlinkLegacyHook
	for _, tt := range []struct {
		name         string
		replaceLink  bool
		wantErr      error
		wantChanged  int
		wantRepaired bool
	}{
		{name: "regular file stays regular and is repaired", wantChanged: 1, wantRepaired: true},
		{name: "regular file replaced by a symlink is refused", replaceLink: true, wantErr: ErrChanged},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			path := projectFile(t, root, "stacks/dev.yaml", original)
			target := filepath.Join(root, "elsewhere", "dev.yaml")
			e := testEngine(t)
			e.Observe = func(event Event) {
				if !tt.replaceLink || event.Phase != "Back up" || !event.Done {
					return
				}
				// The link appears after staging and backup with identical bytes and mode, so only a
				// write-time check can notice it.
				require.NoError(t, os.MkdirAll(filepath.Dir(target), 0o700))
				require.NoError(t, os.WriteFile(target, []byte(original), 0o640))
				require.NoError(t, os.Remove(path))
				requireSymlink(t, target, path)
			}
			r, err := e.Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				require.Empty(t, r.ChangedFiles)
				requireStillLinked(t, path, target, original)
				return
			}
			require.NoError(t, err)
			require.Len(t, r.ChangedFiles, tt.wantChanged)
			b, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			require.Contains(t, string(b), "before.terraform.apply")
		})
	}
}

func TestUnchangedDetectsDriftThroughASymlinkedDocument(t *testing.T) {
	for _, tt := range []struct {
		name    string
		mutate  func(t *testing.T, target string)
		wantErr error
	}{
		{"untouched target passes", func(*testing.T, string) {}, nil},
		{"content change of the target", func(t *testing.T, target string) {
			require.NoError(t, os.WriteFile(target, []byte("vars:\n  changed: true\n"), 0o640))
		}, ErrChanged},
		{"mode change of the target", func(t *testing.T, target string) {
			if runtime.GOOS == "windows" {
				t.Skip("Windows does not report POSIX permission bits.")
			}
			require.NoError(t, os.Chmod(target, 0o600))
		}, ErrChanged},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := newProject(t)
			projectFile(t, root, "atmos.yaml", "{}\n")
			target := projectFile(t, root, "shared/prod.yaml", "vars: {}\n")
			link := filepath.Join(root, "stacks", "prod.yaml")
			require.NoError(t, os.MkdirAll(filepath.Dir(link), 0o700))
			requireSymlink(t, target, link)
			e := testEngine(t)
			o := &Options{Root: root, Offline: true}
			p := ecDiscover(t, e, o)
			require.NotNil(t, p.byPath[link], "the symlinked document must be part of the captured project")
			tt.mutate(t, target)
			err := e.unchanged(context.Background(), p, o)
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tt.wantErr)
			require.Contains(t, filepath.ToSlash(err.Error()), "stacks/prod.yaml")
		})
	}
}

func toSlashAll(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.ToSlash(p)
	}
	return out
}

const readOnlyReason = "read-only source requires manual review; doctor does not overwrite read-only files"

// requireReadOnly makes path read-only for its owner, skipping where the platform ignores permission bits.
func requireReadOnly(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.Chmod(path, 0o444))
	info, err := os.Stat(path)
	require.NoError(t, err)
	if info.Mode().Perm()&0o222 != 0 {
		t.Skip("The platform does not report read-only permission bits.")
	}
}

func TestFixReadOnlyStackIsReportedNotApplied(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", ecLegacyReconfigure)
	regular := projectFile(t, root, "stacks/dev.yaml", symlinkLegacyHook)
	readOnly := projectFile(t, root, "stacks/prod.yaml", symlinkLegacyHook)
	requireReadOnly(t, readOnly)

	r, err := testEngine(t).Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
	require.NoError(t, err)
	require.Empty(t, r.OperationalError)
	require.ElementsMatch(t, []string{"atmos.yaml", filepath.ToSlash(filepath.Join("stacks", "dev.yaml"))}, toSlashAll(r.ChangedFiles))

	b, err := os.ReadFile(regular)
	require.NoError(t, err)
	require.Contains(t, string(b), "before.terraform.apply")

	f := symlinkFinding(&r, "stacks/prod.yaml")
	require.NotNil(t, f, "%+v", r.Findings)
	require.Equal(t, statusVerificationIncomplete, f.Status)
	require.False(t, f.Fixable)
	require.Contains(t, f.Reason, readOnlyReason)
	require.NotEmpty(t, f.Remediation)

	b, err = os.ReadFile(readOnly)
	require.NoError(t, err)
	require.Equal(t, symlinkLegacyHook, string(b), "the read-only file must keep its bytes")
	info, err := os.Stat(readOnly)
	require.NoError(t, err)
	require.Zero(t, info.Mode().Perm()&0o222, "the read-only file must keep its mode")
}

func TestFixDryRunReportsReadOnlyStackAsManual(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	readOnly := projectFile(t, root, "stacks/prod.yaml", symlinkLegacyHook)
	requireReadOnly(t, readOnly)

	r, err := testEngine(t).Run(context.Background(), Options{Root: root, Fix: true, DryRun: true, Offline: true, Only: []string{"deprecations"}})
	require.NoError(t, err)
	f := symlinkFinding(&r, "stacks/prod.yaml")
	require.NotNil(t, f, "%+v", r.Findings)
	require.Equal(t, statusVerificationIncomplete, f.Status)
	require.False(t, f.Fixable)
	require.Contains(t, f.Reason, readOnlyReason)
}

func TestFixWritableStackIsNotRejectedAsReadOnly(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	path := projectFile(t, root, "stacks/dev.yaml", symlinkLegacyHook)

	r, err := testEngine(t).Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
	require.NoError(t, err)
	require.Equal(t, []string{filepath.ToSlash(filepath.Join("stacks", "dev.yaml"))}, toSlashAll(r.ChangedFiles))
	f := symlinkFinding(&r, "stacks/dev.yaml")
	if f != nil {
		require.NotContains(t, f.Reason, readOnlyReason)
	}
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(b), "before.terraform.apply")
}

func TestFixRefusesToWriteAFileMadeReadOnlyAfterStaging(t *testing.T) {
	root := newProject(t)
	projectFile(t, root, "atmos.yaml", "{}\n")
	path := projectFile(t, root, "stacks/dev.yaml", symlinkLegacyHook)
	e := testEngine(t)
	e.Observe = func(event Event) {
		if event.Phase == "Back up" && event.Done {
			require.NoError(t, os.Chmod(path, 0o444))
		}
	}
	r, err := e.Run(context.Background(), Options{Root: root, Fix: true, Offline: true, Only: []string{"deprecations"}})
	if info, statErr := os.Stat(path); statErr == nil && info.Mode().Perm()&0o222 != 0 {
		t.Skip("The platform does not report read-only permission bits.")
	}
	require.ErrorIs(t, err, ErrChanged)
	require.Empty(t, r.ChangedFiles)
	b, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	require.Equal(t, symlinkLegacyHook, string(b))
}
