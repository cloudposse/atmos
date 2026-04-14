package detect_test

import (
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/atmospro/detect"
)

func TestAtmosAuth_Detected(t *testing.T) {
	fsys := fstest.MapFS{
		"atmos.yaml": &fstest.MapFile{
			Data: []byte(`base_path: "./"
auth:
  providers:
    sso: {}
`),
		},
	}
	r, err := detect.AtmosAuth(fsys)
	require.NoError(t, err)
	assert.True(t, r.Detected)
	assert.Contains(t, r.Evidence, "atmos.yaml")
	assert.Contains(t, r.Hint, "patch file")
}

func TestAtmosAuth_NotDetected(t *testing.T) {
	fsys := fstest.MapFS{
		"atmos.yaml": &fstest.MapFile{
			Data: []byte(`base_path: "./"
components:
  terraform:
    base_path: components/terraform
`),
		},
	}
	r, err := detect.AtmosAuth(fsys)
	require.NoError(t, err)
	assert.False(t, r.Detected)
	assert.Contains(t, r.Hint, "standalone profiles")
	assert.Empty(t, r.Evidence)
}

func TestAtmosAuth_DiscoversAtmosDImports(t *testing.T) {
	fsys := fstest.MapFS{
		"atmos.yaml": &fstest.MapFile{
			Data: []byte("base_path: \"./\"\n"),
		},
		"atmos.d/auth.yaml": &fstest.MapFile{
			Data: []byte("auth:\n  providers:\n    sso: {}\n"),
		},
		"atmos.d/unrelated.yaml": &fstest.MapFile{
			Data: []byte("components: {}\n"),
		},
	}
	r, err := detect.AtmosAuth(fsys)
	require.NoError(t, err)
	assert.True(t, r.Detected)
	assert.Equal(t, []string{"atmos.d/auth.yaml"}, r.Evidence)
}

func TestAtmosAuth_DoesNotConfuseNestedKeys(t *testing.T) {
	// A "settings.auth" or "components.terraform.auth" block must not trigger
	// a positive detection. Only top-level auth: counts.
	fsys := fstest.MapFS{
		"atmos.yaml": &fstest.MapFile{
			Data: []byte(`settings:
  auth:
    strict: true
`),
		},
	}
	r, err := detect.AtmosAuth(fsys)
	require.NoError(t, err)
	assert.False(t, r.Detected, "indented auth: must not match the top-level probe")
}

func TestSpacelift_Detected(t *testing.T) {
	fsys := fstest.MapFS{
		"stacks/orgs/example/_defaults.yaml": &fstest.MapFile{
			Data: []byte(`settings:
  spacelift:
    workspace_enabled: true
`),
		},
		"stacks/unrelated.yaml": &fstest.MapFile{
			Data: []byte("components: {}\n"),
		},
	}
	r, err := detect.Spacelift(fsys, "stacks")
	require.NoError(t, err)
	assert.True(t, r.Detected)
	assert.Equal(t, []string{"stacks/orgs/example/_defaults.yaml"}, r.Evidence)
	assert.Contains(t, r.Hint, "migration")
}

func TestSpacelift_MissingStacksDir(t *testing.T) {
	fsys := fstest.MapFS{
		"atmos.yaml": &fstest.MapFile{Data: []byte("")},
	}
	r, err := detect.Spacelift(fsys, "stacks")
	require.NoError(t, err, "missing stacks dir must not be a hard error")
	assert.False(t, r.Detected)
	assert.Empty(t, r.Evidence)
}

func TestSpacelift_IgnoresDisabled(t *testing.T) {
	fsys := fstest.MapFS{
		"stacks/a.yaml": &fstest.MapFile{
			Data: []byte("settings:\n  spacelift:\n    workspace_enabled: false\n"),
		},
	}
	r, err := detect.Spacelift(fsys, "stacks")
	require.NoError(t, err)
	assert.False(t, r.Detected)
}

func TestGeodesic_DetectedViaDockerfile(t *testing.T) {
	fsys := fstest.MapFS{
		"Dockerfile": &fstest.MapFile{
			Data: []byte("FROM cloudposse/geodesic:latest\nRUN ls\n"),
		},
	}
	r, err := detect.Geodesic(fsys)
	require.NoError(t, err)
	assert.True(t, r.Detected)
	assert.Contains(t, r.Evidence, "Dockerfile")
	assert.Contains(t, r.Hint, "GITHUB_TOKEN")
}

func TestGeodesic_DetectedViaGeodesicMk(t *testing.T) {
	fsys := fstest.MapFS{
		"geodesic.mk": &fstest.MapFile{Data: []byte("# anything\n")},
	}
	r, err := detect.Geodesic(fsys)
	require.NoError(t, err)
	assert.True(t, r.Detected, "geodesic.mk presence alone is enough")
}

func TestGeodesic_NotDetected(t *testing.T) {
	fsys := fstest.MapFS{
		"Dockerfile": &fstest.MapFile{Data: []byte("FROM alpine:3\n")},
		"Makefile":   &fstest.MapFile{Data: []byte("build:\n\tgo build\n")},
	}
	r, err := detect.Geodesic(fsys)
	require.NoError(t, err)
	assert.False(t, r.Detected)
	assert.Empty(t, r.Evidence)
}

func TestAll_EmptyFS(t *testing.T) {
	_, err := detect.All(nil, "stacks")
	require.Error(t, err)
	assert.ErrorIs(t, err, detect.ErrEmptyFS)
}

func TestAll_ReturnsDeterministicOrder(t *testing.T) {
	fsys := fstest.MapFS{
		"atmos.yaml": &fstest.MapFile{Data: []byte("")},
	}
	results, err := detect.All(fsys, "stacks")
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "atmos-auth", results[0].Name)
	assert.Equal(t, "spacelift", results[1].Name)
	assert.Equal(t, "geodesic", results[2].Name)
}

func TestAtmosAuth_DiscoversGeodesicConfigPath(t *testing.T) {
	// Geodesic-hosted repos store atmos.yaml at rootfs/usr/local/etc/atmos/ and
	// reference it via ATMOS_CLI_CONFIG_PATH in workflows. The probe must detect
	// auth: declared at that location even when no atmos.yaml exists at repo root.
	fsys := fstest.MapFS{
		"rootfs/usr/local/etc/atmos/atmos.yaml": &fstest.MapFile{
			Data: []byte("auth:\n  providers:\n    sso: {}\n"),
		},
	}
	r, err := detect.AtmosAuth(fsys)
	require.NoError(t, err)
	assert.True(t, r.Detected, "Geodesic-path atmos.yaml must be discovered")
	assert.Equal(t, []string{"rootfs/usr/local/etc/atmos/atmos.yaml"}, r.Evidence)
}

func TestLocateAtmosYAML_Geodesic(t *testing.T) {
	fsys := fstest.MapFS{
		"rootfs/usr/local/etc/atmos/atmos.yaml": &fstest.MapFile{Data: []byte("")},
	}
	path, err := detect.LocateAtmosYAML(fsys)
	require.NoError(t, err)
	assert.Equal(t, "rootfs/usr/local/etc/atmos/atmos.yaml", path)
}

func TestLocateAtmosYAML_PrefersGeodesicPathOverRoot(t *testing.T) {
	// When both exist, the Geodesic path wins because workflows read from there.
	fsys := fstest.MapFS{
		"atmos.yaml":                            &fstest.MapFile{Data: []byte("")},
		"rootfs/usr/local/etc/atmos/atmos.yaml": &fstest.MapFile{Data: []byte("")},
	}
	path, err := detect.LocateAtmosYAML(fsys)
	require.NoError(t, err)
	assert.Equal(t, "rootfs/usr/local/etc/atmos/atmos.yaml", path,
		"Geodesic path must win because workflows set ATMOS_CLI_CONFIG_PATH to it")
}

func TestLocateAtmosYAML_FallsBackToRoot(t *testing.T) {
	fsys := fstest.MapFS{
		"atmos.yaml": &fstest.MapFile{Data: []byte("")},
	}
	path, err := detect.LocateAtmosYAML(fsys)
	require.NoError(t, err)
	assert.Equal(t, "atmos.yaml", path)
}

func TestLocateAtmosYAML_EmptyWhenAbsent(t *testing.T) {
	fsys := fstest.MapFS{
		"Dockerfile": &fstest.MapFile{Data: []byte("")},
	}
	path, err := detect.LocateAtmosYAML(fsys)
	require.NoError(t, err)
	assert.Empty(t, path)
}

func TestLocateAtmosYAML_NilFS(t *testing.T) {
	_, err := detect.LocateAtmosYAML(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, detect.ErrEmptyFS)
}

func TestAll_ClassifiesRealisticRepo(t *testing.T) {
	// A representative "mid-journey" repo: Atmos Auth configured, Spacelift
	// still enabled in one stack, no Geodesic. The skill must detect all three
	// correctly to drive the right variants.
	fsys := fstest.MapFS{
		"atmos.yaml": &fstest.MapFile{
			Data: []byte("auth:\n  providers:\n    sso: {}\n"),
		},
		"stacks/orgs/acme/_defaults.yaml": &fstest.MapFile{
			Data: []byte("settings:\n  spacelift:\n    workspace_enabled: true\n"),
		},
	}
	results, err := detect.All(fsys, "stacks")
	require.NoError(t, err)

	byName := map[string]detect.Result{}
	for _, r := range results {
		byName[r.Name] = r
	}

	assert.True(t, byName["atmos-auth"].Detected)
	assert.True(t, byName["spacelift"].Detected)
	assert.False(t, byName["geodesic"].Detected)
}
