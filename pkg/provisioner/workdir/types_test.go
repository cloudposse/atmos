package workdir

import (
	"path/filepath"
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
			want:            []string{"terraform", "dev-vpc"},
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
			want: []string{"terraform", "dev-vpc-hinherited-hinstance"},
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
			want: []string{"terraform", "dev-vpc"},
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
			want: []string{"terraform", "dev-vpc"},
		},
		{
			name:            "nil componentConfig falls back to component",
			basePath:        "/base",
			componentType:   "helmfile",
			component:       "app",
			stack:           "prod",
			componentConfig: nil,
			want:            []string{"helmfile", "prod-app"},
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
			want:            []string{"terraform", "fixtures-ecs-scluster"},
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
			want: []string{"terraform", "fixtures-ecs-scluster-hinherited-hinstance"},
		},
		{
			// "\" is Windows' real path separator: a component name
			// containing it must sanitize identically to "/", or
			// filepath.Join/Clean would treat it as real directory
			// segments (including ".."-traversal) on that platform.
			name:            "backslash-containing component name does not add a path segment",
			basePath:        "/base",
			componentType:   "terraform",
			component:       `ecs\cluster`,
			stack:           "fixtures",
			componentConfig: map[string]any{},
			want:            []string{"terraform", "fixtures-ecs-scluster"},
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
			want:            []string{"terraform", "fixtures-..-s..-sevil"},
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
// component-name encodings: a naive "/" -> "-" substitution would make "app/local" and
// "app-local" both resolve to "dev-app-local", and a "/" -> "--" substitution (escaping the
// separator but not the literal hyphen) would still make "app/local" and "app--local" both
// resolve to "dev-app--local". Either way, two entirely distinct components would silently
// share one workdir -- and its files, metadata, and Terraform state (Service.Provision
// writes directly into whatever BuildPath returns). The injective encoding
// escapeComponentNameForPath uses instead (escaping the literal hyphen before the separator)
// keeps all three distinct.
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

// TestBuildPath_RejectsStackTraversal verifies BuildPath rejects a stack name containing
// ".." segments instead of silently resolving a workdir path outside basePath. Both the
// stack and component name can originate from user-controlled YAML.
func TestBuildPath_RejectsStackTraversal(t *testing.T) {
	base := t.TempDir()

	_, err := BuildPath(base, "terraform", "vpc", "../../../../../../evil", map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPathTraversal)
}
