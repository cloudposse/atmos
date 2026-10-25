package exec

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

// fixtureBootstrapHooks models a test fixture started by a before-hook. Authentication
// checks the fixture's artifact so the test exercises the actual executor ordering.
type fixtureBootstrapHooks struct {
	schema.ComponentNodeHooks
	run           func(*schema.ConfigAndStacksInfo) error
	skipBootstrap bool
}

func (h fixtureBootstrapHooks) NeedsPreAuth(_ *schema.AtmosConfiguration, _ *schema.ConfigAndStacksInfo) (bool, error) {
	return !h.skipBootstrap, nil
}

func (h fixtureBootstrapHooks) BeforePrepared(_ context.Context, _ *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, _ schema.ComponentNodeHookWriters) error {
	return h.run(info)
}

func TestExecuteTerraform_TestHooksBeforeAuth(t *testing.T) {
	for _, tc := range []struct {
		name        string
		command     string
		noHooks     bool
		noUserHooks bool
		hookFails   bool
	}{
		{name: "test", command: "test"},
		{name: "init", command: "init"},
		{name: "destroy", command: "destroy"},
		{name: "test without hooks", command: "test", noHooks: true},
		{name: "test without user hooks", command: "test", noUserHooks: true},
		{name: "test fixture failure", command: "test", hookFails: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			t.Chdir(root)
			require.NoError(t, os.MkdirAll(filepath.Join(root, "stacks"), 0o755))
			require.NoError(t, os.MkdirAll(filepath.Join(root, "components", "terraform", "module"), 0o755))
			require.NoError(t, os.WriteFile("atmos.yaml", []byte(`base_path: .
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths: [test.yaml]
  name_pattern: "{stage}"
`), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join("stacks", "test.yaml"), []byte(`vars:
  stage: test
components:
  terraform:
    app:
      metadata:
        component: module
      provision:
        workdir:
          enabled: true
`), 0o600))
			source := filepath.Join(root, "components", "terraform", "module", "main.tf")
			require.NoError(t, os.WriteFile(source, []byte("# fixture\n"), 0o600))
			called := false
			authCalled := false
			fixturePath := ""
			hooks := fixtureBootstrapHooks{skipBootstrap: tc.noUserHooks, run: func(info *schema.ConfigAndStacksInfo) error {
				called = true
				dir, err := component.ApplyWorkdirSubpathToSection(info)
				require.NoError(t, err)
				require.NotEmpty(t, dir)
				assert.Contains(t, filepath.ToSlash(dir), ".workdir/")
				assert.FileExists(t, filepath.Join(dir, "main.tf"))
				fixturePath = filepath.Join(dir, "fixture-ready")
				if tc.hookFails {
					return errUtils.ErrPerComponentHookFailed
				}
				return os.WriteFile(fixturePath, []byte("ready"), 0o600)
			}}
			original := defaultAuthManagerCreator
			t.Cleanup(func() { defaultAuthManagerCreator = original })
			defaultAuthManagerCreator = func(_ string, _ *schema.AuthConfig, _ string, _ *schema.AtmosConfiguration, _ auth.ReExecContext) (auth.AuthManager, error) {
				authCalled = true
				assert.Equal(t, tc.command == "test" && !tc.noHooks && !tc.noUserHooks, called, "test fixtures must start before authentication; other commands retain authenticated hooks")
				if called {
					assert.FileExists(t, fixturePath)
				}
				// Stop before invoking Terraform: this regression concerns ordering at auth setup.
				return nil, errUtils.ErrAuthNotConfigured
			}
			info := schema.ConfigAndStacksInfo{ComponentType: "terraform", ComponentFromArg: "app", Stack: "test", SubCommand: tc.command}
			if !tc.noHooks {
				info.NodeHooks = hooks
			}
			err := ExecuteTerraform(info)
			if tc.hookFails {
				require.ErrorIs(t, err, errUtils.ErrPerComponentHookFailed)
				assert.False(t, authCalled, "failed fixture setup must prevent authentication")
			} else {
				require.ErrorIs(t, err, errUtils.ErrAuthNotConfigured)
				assert.True(t, authCalled)
			}
			assert.NoFileExists(t, filepath.Join(filepath.Dir(source), "fixture-ready"))
		})
	}
}
