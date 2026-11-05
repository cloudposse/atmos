package dependencies

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// Compile-time sentinel for the schema field these tests configure.
var _ = schema.Toolchain{Install: schema.ToolchainInstallNever}

// TestInstalledVersionRejectsUnusableInput verifies that installedVersion reports "not installed"
// instead of probing the filesystem when the identity or constraint cannot be understood, and when
// listing installed versions fails.
func TestInstalledVersionRejectsUnusableInput(t *testing.T) {
	listErr := errors.New("cannot list installs")

	tests := []struct {
		name     string
		identity string
		version  string
		list     func(owner, repo string) ([]string, error)
	}{
		{
			name:     "identity without owner and repo",
			identity: "terraform",
			version:  "1.9.0",
		},
		{
			name:     "identity with an empty repo",
			identity: "hashicorp/",
			version:  "1.9.0",
		},
		{
			name:     "unparseable constraint",
			identity: "hashicorp/terraform",
			version:  "not a version",
		},
		{
			name:     "listing installed versions fails",
			identity: "hashicorp/terraform",
			version:  "~> 1.9",
			list:     func(string, string) ([]string, error) { return nil, listErr },
		},
		{
			name:     "no installed version satisfies the constraint",
			identity: "hashicorp/terraform",
			version:  "~> 1.9",
			list:     func(string, string) ([]string, error) { return []string{"1.8.0", "2.0.0"}, nil },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			findCalls := 0
			ids := &toolIdentity{
				list: tt.list,
				find: func(string, string, string, ...string) (string, error) {
					findCalls++
					return "", nil
				},
			}

			version, ok := ids.installedVersion(tt.identity, tt.version)

			assert.False(t, ok)
			assert.Empty(t, version)
			assert.Zero(t, findCalls, "a binary lookup must not run when the version cannot be resolved")
		})
	}
}

// TestInstalledVersionResolvesConstraintAgainstInstalledVersions is the positive counterpart: a
// constraint resolves to the highest installed version that satisfies it, and that version is then
// looked up on disk.
func TestInstalledVersionResolvesConstraintAgainstInstalledVersions(t *testing.T) {
	var foundVersion string
	ids := &toolIdentity{
		list: func(string, string) ([]string, error) { return []string{"1.9.0", "1.9.4", "1.10.0"}, nil },
		find: func(_, _, version string, _ ...string) (string, error) {
			foundVersion = version
			return "bin", nil
		},
	}

	version, ok := ids.installedVersion("hashicorp/terraform", "~> 1.9.0")

	require.True(t, ok)
	assert.Equal(t, "1.9.4", version)
	assert.Equal(t, "1.9.4", foundVersion)
}

// TestInstalledVersionUsesInstallerWhenNotInjected exercises the default (non-injected) lookups
// against a real installer rooted at a temporary install path.
func TestInstalledVersionUsesInstallerWhenNotInjected(t *testing.T) {
	config, _ := defaultsFixture(t, "")
	toolchain.SetAtmosConfig(config)
	installedDefaultBinary(t, config, "hashicorp", "terraform", "1.9.0")
	installedDefaultBinary(t, config, "hashicorp", "terraform", "1.9.5")

	ids := newToolIdentity(config, &envConfig{})

	version, ok := ids.installedVersion("hashicorp/terraform", "~> 1.9.0")
	require.True(t, ok)
	assert.Equal(t, "1.9.5", version)

	version, ok = ids.installedVersion("hashicorp/terraform", "1.9.0")
	require.True(t, ok)
	assert.Equal(t, "1.9.0", version)

	_, ok = ids.installedVersion("hashicorp/terraform", "1.8.0")
	assert.False(t, ok, "a concrete version must be installed exactly")

	_, ok = ids.installedVersion("hashicorp/terraform", "~> 2.0")
	assert.False(t, ok, "a constraint with no installed match is not installed")
}

// TestRequireInstalledPropagatesIdentityResolutionError verifies that under toolchain.install=never
// an explicit dependency whose name cannot be resolved fails with the resolver's error instead of
// being reported as merely "not installed" or silently dropped.
func TestRequireInstalledPropagatesIdentityResolutionError(t *testing.T) {
	cause := errors.New("registry unavailable")
	config, _ := defaultsFixture(t, "")
	config.Toolchain.Install = schema.ToolchainInstallNever

	env, err := ForDependencies(
		config, map[string]string{"mystery-tool": "1.0.0"},
		withResolveFunc(func(string) (string, string, error) { return "", "", cause }),
		withEnsureTools(func(map[string]string) error {
			t.Fatal("never must not install anything")
			return nil
		}),
	)

	require.ErrorIs(t, err, cause)
	require.NotErrorIs(t, err, errUtils.ErrToolNotInstalled)
	assert.Contains(t, err.Error(), "mystery-tool")
	assert.Nil(t, env)
}

// TestInstalledOnlySkipsUnresolvableAndMissingTools verifies the lenient manifest filter keeps only
// installed tools and silently drops unresolvable or missing ones.
func TestInstalledOnlySkipsUnresolvableAndMissingTools(t *testing.T) {
	ids := &toolIdentity{
		resolve: func(string) (string, string, error) { return "", "", errUnknownTool },
		find: func(owner, repo, _ string, _ ...string) (string, error) {
			if owner == "jqlang" {
				return "jq", nil
			}
			return "", os.ErrNotExist
		},
	}

	got := installedOnly(map[string]string{
		"jqlang/jq":           "1.7.1",
		"hashicorp/terraform": "1.9.0",
		"unresolvable":        "1.0.0",
	}, ids)

	assert.Equal(t, map[string]string{"jqlang/jq": "1.7.1"}, got)
}
