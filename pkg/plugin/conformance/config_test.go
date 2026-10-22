package conformance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestLoadRejectsAmbiguousOrUnsupportedFixtures(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"unsupported version": "version: 2\ncommands: [{name: about}]\n",
		"missing version":     "commands: [{name: about}]\n",
		"unknown field":       "version: 1\ncommand: about\n",
		"multiple documents":  "version: 1\ncommands: [{name: about}]\n---\nversion: 1\n",
		"missing provider":    "version: 1\noidc: {identity: broker, session: oidc-starter/v1}\n",
		"unsupported session": "version: 1\noidc: {provider: oidc, identity: broker, session: arbitrary}\n",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			directory := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.tests.yaml"), []byte(raw), 0o600))
			_, err := load(directory)
			require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
		})
	}
}

func TestLoadSeparatesMockAndLiveFixtures(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	raw := "version: 1\ncommands:\n  - name: about\n    arguments: [--version]\n    stdout: '0.1.0'\n    exit_code: 0\n"
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.tests.yaml"), []byte(raw), 0o600))
	config, err := load(directory)
	require.NoError(t, err)
	require.Equal(t, []CommandFixture{{Name: "about", Arguments: []string{"--version"}, Stdout: "0.1.0"}}, config.Commands)
	require.Empty(t, config.Integration)
}

func TestLoadAllowsRegisteredHooksWithoutAuthoredFixtures(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.tests.yaml"), []byte("version: 1\n"), 0o600))
	config, err := load(directory)
	require.NoError(t, err)
	require.Equal(t, &Config{Version: 1}, config)
}
