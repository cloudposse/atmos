package workdir

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// TestBuildPath covers BuildPath's instance-name resolution: it must prefer
// componentConfig's "atmos_component" (the per-instance name, used to keep
// inherited components isolated in distinct workdirs) over the shared
// component/metadata name, and fall back to component when no instance name
// is present.
func TestBuildPath(t *testing.T) {
	tests := []struct {
		name            string
		basePath        string
		componentType   string
		component       string
		stack           string
		componentConfig map[string]any
		want            []string
	}{
		{
			name:            "falls back to component when atmos_component is absent",
			basePath:        "/base",
			componentType:   "terraform",
			component:       "vpc",
			stack:           "dev",
			componentConfig: map[string]any{},
			want:            []string{"terraform", expectedWorkdirName("dev", "vpc")},
		},
		{
			name:          "prefers atmos_component instance name over shared component name",
			basePath:      "/base",
			componentType: "terraform",
			component:     "vpc",
			stack:         "dev",
			componentConfig: map[string]any{
				"atmos_component": "vpc-inherited-instance",
			},
			want: []string{"terraform", expectedWorkdirName("dev", "vpc-inherited-instance")},
		},
		{
			name:          "falls back to component when atmos_component is empty string",
			basePath:      "/base",
			componentType: "terraform",
			component:     "vpc",
			stack:         "dev",
			componentConfig: map[string]any{
				"atmos_component": "",
			},
			want: []string{"terraform", expectedWorkdirName("dev", "vpc")},
		},
		{
			name:          "falls back to component when atmos_component is not a string",
			basePath:      "/base",
			componentType: "terraform",
			component:     "vpc",
			stack:         "dev",
			componentConfig: map[string]any{
				"atmos_component": 123,
			},
			want: []string{"terraform", expectedWorkdirName("dev", "vpc")},
		},
		{
			name:            "nil componentConfig falls back to component",
			basePath:        "/base",
			componentType:   "helmfile",
			component:       "app",
			stack:           "prod",
			componentConfig: nil,
			want:            []string{"helmfile", expectedWorkdirName("prod", "app")},
		},
		{
			// A nested component name (containing "/", e.g. an ecs/cluster
			// directory layout) must sanitize to a single path segment, not
			// add an extra directory level. Otherwise a relative backend
			// path template like `../../../` climbs a different number of
			// real ancestors for a nested component than for a flat one at
			// the same stack, silently writing state under a different root
			// (see docs/fixes for the regression this guards).
			name:            "nested component name does not add a path segment",
			basePath:        "/base",
			componentType:   "terraform",
			component:       "ecs/cluster",
			stack:           "fixtures",
			componentConfig: map[string]any{},
			want:            []string{"terraform", expectedWorkdirName("fixtures", "ecs/cluster")},
		},
		{
			name:          "nested atmos_component instance name does not add a path segment",
			basePath:      "/base",
			componentType: "terraform",
			component:     "ecs/cluster",
			stack:         "fixtures",
			componentConfig: map[string]any{
				"atmos_component": "ecs/cluster-inherited-instance",
			},
			want: []string{"terraform", expectedWorkdirName("fixtures", "ecs/cluster-inherited-instance")},
		},
		{
			// "\" is Windows' real path separator: a component name
			// containing it must sanitize to a single segment, or
			// filepath.Join/Clean would treat it as real directory
			// segments (including ".."-traversal) on that platform. "/"
			// and "\" both sanitize to the same "-" (see
			// sanitizeComponentNameForPath), so the display prefix is
			// identical to "ecs/cluster"'s; what keeps them from
			// resolving to the same workdir is the hash suffix, computed
			// from the unsanitized name -- see
			// TestBuildPath_NoCollisionBetweenSlashAndBackslash.
			name:            "backslash-containing component name does not add a path segment",
			basePath:        "/base",
			componentType:   "terraform",
			component:       `ecs\cluster`,
			stack:           "fixtures",
			componentConfig: map[string]any{},
			want:            []string{"terraform", expectedWorkdirName("fixtures", `ecs\cluster`)},
		},
		{
			// A component name crafted to escape the workdir root via
			// backslash-".." segments must sanitize to a single, safe
			// segment rather than letting filepath.Join/Clean resolve it
			// as real parent-directory traversal.
			name:            "backslash dot-dot traversal in component name does not escape the workdir root",
			basePath:        "/base",
			componentType:   "terraform",
			component:       `..\..\evil`,
			stack:           "fixtures",
			componentConfig: map[string]any{},
			want:            []string{"terraform", expectedWorkdirName("fixtures", `..\..\evil`)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildPath(tt.basePath, tt.componentType, tt.component, tt.stack, tt.componentConfig)
			require.NoError(t, err)
			want := filepath.Join(append([]string{tt.basePath, WorkdirPath}, tt.want...)...)
			assert.Equal(t, want, got)
		})
	}
}

// TestBuildPath_NoCollisionBetweenSlashAndHyphen is a regression test for two collision-prone
// component-name display schemes: a naive "/" -> "-" substitution alone would make "app/local"
// and "app-local" both sanitize to the identical display prefix "app-local", and a "/" -> "--"
// substitution would make "app/local" and "app--local" both sanitize to "app--local". Either
// way, if the display prefix were the only thing determining the path, two entirely distinct
// components would silently share one workdir -- and its files, metadata, and Terraform state
// (Service.Provision writes directly into whatever BuildPath returns). BuildPath's hash suffix
// (see workdirPathHash), computed from the unsanitized name, keeps all three distinct even
// though their sanitized display prefixes collide.
func TestBuildPath_NoCollisionBetweenSlashAndHyphen(t *testing.T) {
	names := []string{"app/local", "app-local", "app--local"}
	paths := make(map[string]string, len(names))

	for _, name := range names {
		path, err := BuildPath("/base", "terraform", name, "dev", map[string]any{})
		require.NoError(t, err)
		paths[name] = path
	}

	for i, a := range names {
		for _, b := range names[i+1:] {
			assert.NotEqual(t, paths[a], paths[b],
				"a component named %q must not resolve to the same workdir as a component named %q", a, b)
		}
	}
}

// TestBuildPath_NoCollisionBetweenSlashAndBackslash is a regression test for the same class of
// collision as TestBuildPath_NoCollisionBetweenSlashAndHyphen, for "/" vs "\":
// sanitizeComponentNameForPath deliberately maps both to the identical "-" display character
// (there's no reason to give them distinct tokens now that the display prefix isn't relied on
// for uniqueness -- see its doc comment), so a component literally named `ecs\cluster` and one
// named "ecs/cluster" sanitize to the exact same prefix, "ecs-cluster". What still keeps them
// on distinct workdirs is workdirPathHash, computed from the unsanitized name.
func TestBuildPath_NoCollisionBetweenSlashAndBackslash(t *testing.T) {
	slashPath, err := BuildPath("/base", "terraform", "ecs/cluster", "dev", map[string]any{})
	require.NoError(t, err)

	backslashPath, err := BuildPath("/base", "terraform", `ecs\cluster`, "dev", map[string]any{})
	require.NoError(t, err)

	assert.NotEqual(t, slashPath, backslashPath,
		"a component named %q must not resolve to the same workdir as a component named %q", "ecs/cluster", `ecs\cluster`)
}

// TestBuildPath_RejectsStackTraversal verifies BuildPath rejects a stack name whose ".."
// segments (e.g. "../../../../../../evil") would otherwise let filepath.Join's implicit
// Clean() fold it into a workdir path outside the intended root. Both the stack and component
// name can originate from user-controlled YAML.
func TestBuildPath_RejectsStackTraversal(t *testing.T) {
	base := t.TempDir()

	_, err := BuildPath(base, "terraform", "vpc", "../../../../../../evil", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
}

// TestBuildPath_RejectsStackContainingDotDotSegment is a regression test for the collision
// CodeRabbit flagged: a stack name containing a ".." segment that stays within basePath after
// filepath.Join's implicit Clean() -- e.g. "team/../prod" naively joins to the same path as
// stack "prod" -- so two distinct stack configurations would silently share one workdir (and
// therefore its files, metadata, and Terraform state). BuildPath rejects a "."/".." path
// segment in stack outright (rather than sanitizing the whole value, as it does for
// workdirComponent). It does NOT reject "/" on its own -- see
// TestBuildPath_AllowsSlashInStackWithoutDotSegments -- specifically to avoid changing the
// on-disk path for a stack name that merely contains a literal "-" (e.g. "us-east-1-dev") or a
// real, non-traversal "/" nesting convention.
func TestBuildPath_RejectsStackContainingDotDotSegment(t *testing.T) {
	base := t.TempDir()

	_, err := BuildPath(base, "terraform", "vpc", "team/../prod", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
}

// TestBuildPath_RejectsStackContainingBackslashDotDotSegment mirrors
// TestBuildPath_RejectsStackContainingDotDotSegment for "\", Windows' real path separator --
// rejected unconditionally (not just on Windows) so a stack name's rejection is deterministic
// regardless of which OS Atmos runs on.
func TestBuildPath_RejectsStackContainingBackslashDotDotSegment(t *testing.T) {
	base := t.TempDir()

	_, err := BuildPath(base, "terraform", "vpc", `team\..\prod`, map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
}

// TestBuildPath_RejectsBareDotSegment verifies a stack that is exactly "." (or contains a bare
// "." segment) is rejected the same way as "..": both are Clean()-significant tokens, even
// though only ".." can climb outside the intended root.
func TestBuildPath_RejectsBareDotSegment(t *testing.T) {
	base := t.TempDir()

	_, err := BuildPath(base, "terraform", "vpc", "team/./prod", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
}

// TestBuildPath_AllowsHyphenatedStackName is a regression test guarding against reintroducing
// the blast radius fully sanitizing stack (mirroring workdirComponent's
// sanitizeComponentNameForPath) would have: since stack only has "."/".." segments rejected, not
// its whole value sanitized, a stack name containing a literal "-" -- extremely common, e.g.
// "us-east-1-dev" -- must appear verbatim in the on-disk path, not a re-encoded one that would
// orphan every existing workdir for a hyphenated stack on upgrade.
func TestBuildPath_AllowsHyphenatedStackName(t *testing.T) {
	base := t.TempDir()

	path, err := BuildPath(base, "terraform", "vpc", "us-east-1-dev", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, WorkdirPath, "terraform", expectedWorkdirName("us-east-1-dev", "vpc")), path)
}

// TestBuildPath_AllowsSlashInStackWithoutDotSegments is a regression test for a real CI
// failure this exact package caused: an earlier revision of validateStackForPath rejected any
// "/" in stack outright, breaking cmd/terraform/migrate's own test fixtures, which use a stack
// literally named "deploy/test" -- a supported nesting convention with no traversal risk at
// all, since it contains no "."/".." segment for Clean() to fold. BuildPath must keep resolving
// it to a real nested directory, exactly as it always has (stack has never been sanitized the
// way workdirComponent is).
func TestBuildPath_AllowsSlashInStackWithoutDotSegments(t *testing.T) {
	base := t.TempDir()

	path, err := BuildPath(base, "terraform", "vpc", "deploy/test", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, WorkdirPath, "terraform", expectedWorkdirName("deploy/test", "vpc")), path)
}

// TestBuildPath_RejectsLeadingSlashInStack is a regression test for a collision
// strings.FieldsFunc's empty-segment-dropping silently let through: validateStackForPath's
// earlier revision split stack with strings.FieldsFunc, which drops empty segments, so
// "/deploy/test" produced the same ["deploy", "test"] segment list "deploy/test" does. But
// filepath.Join's implicit Clean() collapses the leading "/" the same way, so stack
// "/deploy/test" and stack "deploy/test" resolved to the identical workdir path -- two distinct
// stack configurations silently sharing one workdir.
func TestBuildPath_RejectsLeadingSlashInStack(t *testing.T) {
	base := t.TempDir()

	_, err := BuildPath(base, "terraform", "vpc", "/deploy/test", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
}

// TestBuildPath_RejectsRepeatedSlashInStack mirrors TestBuildPath_RejectsLeadingSlashInStack for
// a doubled "/" in the middle of stack: "deploy//test" also silently dropped its empty segment
// under strings.FieldsFunc, and filepath.Join's implicit Clean() collapses the doubled "/" down
// to the same path stack "deploy/test" produces.
func TestBuildPath_RejectsRepeatedSlashInStack(t *testing.T) {
	base := t.TempDir()

	_, err := BuildPath(base, "terraform", "vpc", "deploy//test", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
}

// TestBuildPath_AllowsTrailingSlashInStack verifies a single trailing "/" is not rejected: unlike
// a leading or repeated "/", it does not alias its non-trailing counterpart through Clean() --
// BuildPath always appends "-<component>-<hash>" directly onto stack with no separator in
// between, so a trailing "/" becomes its own real path segment (".../test/-vpc-<hash>") distinct
// from the non-trailing form (".../test-vpc-<hash>"). There is nothing to close for it, so it is
// left alone, matching stack "/"'s general treatment as a supported nesting notation.
func TestBuildPath_AllowsTrailingSlashInStack(t *testing.T) {
	base := t.TempDir()

	path, err := BuildPath(base, "terraform", "vpc", "deploy/test/", map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(base, WorkdirPath, "terraform", expectedWorkdirName("deploy/test/", "vpc")), path)
}

// TestBuildPath_RejectsBackslashInStack verifies a bare "\" in stack (no "."/".." segment) is
// rejected, closing a Windows-specific collision: unlike "/", the one nesting notation this
// package documents and tests as supported (see TestBuildPath_AllowsSlashInStackWithoutDotSegments),
// "\" was never adopted as an equivalent second notation, because filepath.Join/Clean on
// Windows treat "\" and "/" as fully interchangeable separators, so stack `deploy\test`, left
// unrejected, would alias the very same workdir as stack "deploy/test" on that platform -- two
// distinct stack configurations silently sharing one workdir. Rejecting it unconditionally (not
// only on Windows) keeps a given stack name's validity independent of which OS Atmos runs on.
func TestBuildPath_RejectsBackslashInStack(t *testing.T) {
	base := t.TempDir()

	_, err := BuildPath(base, "terraform", "vpc", `deploy\test`, map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
}

// TestContainWithinBase_RejectsEscapingPath is a direct unit test for containWithinBase's own
// rejection branch, kept as defense-in-depth coverage now that BuildPath's stack rejection and
// workdirComponent sanitizing should already prevent any real ".." from reaching it in practice.
// Exercises the function directly with a path that lexically escapes base, bypassing BuildPath
// entirely, to confirm the guard itself still works if a future caller (or a change to either
// guard above) ever reintroduces a real separator.
func TestContainWithinBase_RejectsEscapingPath(t *testing.T) {
	base := t.TempDir()
	typeRoot := filepath.Join(base, WorkdirPath, "terraform")
	escapingPath := filepath.Join(typeRoot, "..", "..", "evil")

	_, err := containWithinBase(escapingPath, typeRoot)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
}

// TestBuildPath_AcceptsFilesystemRootBasePath is a regression test for containWithinBase's
// prior absBase+separator prefix check: when basePath resolves to a filesystem root, absBase
// already ends in the separator, so absBase+sep produced a doubled separator ("//" on Unix,
// `C:\\` on Windows) that no real path under that root ever has, rejecting every legitimate
// workdir path derived from a root basePath.
func TestBuildPath_AcceptsFilesystemRootBasePath(t *testing.T) {
	tests := []struct {
		name        string
		basePath    string
		windowsOnly bool
		unixOnly    bool
	}{
		{name: "unix filesystem root", basePath: "/", unixOnly: true},
		{name: "windows volume root", basePath: `C:\`, windowsOnly: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.windowsOnly && runtime.GOOS != "windows" {
				t.Skip("only meaningful on Windows")
			}
			if tt.unixOnly && runtime.GOOS == "windows" {
				t.Skip("only meaningful on non-Windows")
			}
			_, err := BuildPath(tt.basePath, "terraform", "vpc", "dev", map[string]any{})
			require.NoError(t, err,
				"a legitimate workdir path under a filesystem-root basePath must not be rejected as a traversal")
		})
	}
}
