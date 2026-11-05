package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// Exercise command selection through the CLI, stack loader, and actual subprocess.
// An invalid configured executable makes ignored overrides fail decisively.
func TestComponentCommandExecutableOverride(t *testing.T) {
	for _, family := range []string{"terraform", "helmfile", "packer", "ansible"} {
		for _, spelling := range []string{"command", family + "-command"} {
			t.Run(family+"/"+spelling, func(t *testing.T) {
				_ = NewTestKit(t)
				root := componentCommandFixture(t, family)
				marker := filepath.Join(root, "arguments")
				binary, err := os.Executable()
				require.NoError(t, err)
				t.Setenv("_ATMOS_TEST_COMPONENT_ARGS", marker)
				setComponentCommandArgs(t, []string{family, "version", "--" + spelling + "=" + binary})
				require.NoError(t, Execute())
				actual, err := os.ReadFile(marker)
				require.NoError(t, err)
				require.Contains(t, string(actual), "version")
				require.NotContains(t, string(actual), "--command")
			})
		}
	}
}

func TestComponentCommandInvalidSelection(t *testing.T) {
	for _, args := range [][]string{
		{"terraform", "version", "--command="},
		{"terraform", "version", "--command=one", "--terraform-command=two"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			_ = NewTestKit(t)
			root := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte("base_path: .\n"), 0o600))
			t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
			t.Setenv("ATMOS_BASE_PATH", root)
			setComponentCommandArgs(t, args)
			err := Execute()
			require.Error(t, err)
			require.Contains(t, err.Error(), "command")
		})
	}
}

// The override must survive stack templates and component command restoration.
func TestComponentCommandStackPrecedence(t *testing.T) {
	cases := []struct {
		family string
		verb   string
		extra  []string
	}{
		{"terraform", "validate", nil},
		{"helmfile", "template", nil},
		{"packer", "inspect", []string{"--template", "app.pkr.hcl"}},
		{"ansible", "playbook", nil},
	}
	for _, tc := range cases {
		t.Run(tc.family, func(t *testing.T) {
			_ = NewTestKit(t)
			root := componentCommandFixture(t, tc.family)
			binary, err := os.Executable()
			require.NoError(t, err)
			marker := filepath.Join(root, "arguments")
			t.Setenv("_ATMOS_TEST_COMPONENT_ARGS", marker)
			args := []string{tc.family, tc.verb, "app", "-s", "dev", "--command=" + binary}
			args = append(args, tc.extra...)
			setComponentCommandArgs(t, append(args, "--", "--command=native-value"))
			require.NoError(t, Execute())
			actual, err := os.ReadFile(marker)
			require.NoError(t, err)
			require.NotEmpty(t, actual)
			require.Contains(t, string(actual), "--command=native-value")
		})
	}
}

func componentCommandFixture(t *testing.T, family string) string {
	t.Helper()
	clearIdentityEnvVars(t)
	t.Cleanup(viper.Reset)
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "components", "app"), 0o700))
	require.NoError(t, os.Mkdir(filepath.Join(root, "stacks"), 0o700))
	config := `base_path: .
stacks:
  base_path: stacks
  included_paths: [dev]
components:
  ` + family + `:
    base_path: components
    command: executable-that-must-not-run
    auto_generate_backend_file: false
    use_eks: false
    init:
      mode: never
`
	stack := `name: dev
components:
  ` + family + `:
    app:
      command: '{{ "missing-stack-command" }}'
      settings:
        ansible:
          playbook: site.yml
`
	for name, content := range map[string]string{
		"atmos.yaml":                                        config,
		filepath.Join("stacks", "dev.yaml"):                 stack,
		filepath.Join("components", "app", "main.tf"):       "terraform {}\n",
		filepath.Join("components", "app", "app.pkr.hcl"):   "packer {}\n",
		filepath.Join("components", "app", "site.yml"):      "[]\n",
		filepath.Join("components", "app", "helmfile.yaml"): "releases: []\n",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(content), 0o600))
	}
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Setenv("ATMOS_BASE_PATH", root)
	t.Setenv("ATMOS_TELEMETRY_ENABLED", "false")
	return root
}

func setComponentCommandArgs(t *testing.T, args []string) {
	t.Helper()
	original := os.Args
	t.Cleanup(func() { os.Args = original })
	os.Args = append([]string{"atmos"}, args...)
	RootCmd.SetArgs(args)
}

func TestComponentCommandFixtureIsolatesIdentity(t *testing.T) {
	_ = NewTestKit(t)
	// Other command tests can leave an explicit Viper selection behind. Real auth
	// environment variables must also stay out of executable-selection fixtures.
	viper.Set("identity", "previous-test-identity")
	t.Cleanup(viper.Reset)
	t.Setenv("ATMOS_IDENTITY", "environment-identity")
	t.Setenv("IDENTITY", "legacy-environment-identity")
	root := componentCommandFixture(t, "ansible")
	binary, err := os.Executable()
	require.NoError(t, err)
	marker := filepath.Join(root, "arguments")
	t.Setenv("_ATMOS_TEST_COMPONENT_ARGS", marker)
	setComponentCommandArgs(t, []string{"ansible", "playbook", "app", "-s", "dev", "--command=" + binary})
	require.NoError(t, Execute())
	actual, err := os.ReadFile(marker)
	require.NoError(t, err)
	require.Contains(t, string(actual), "site.yml")
}
