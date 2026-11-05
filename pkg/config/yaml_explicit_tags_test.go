package config

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExplicitTagsConfigImportConditions(t *testing.T) {
	setupTestAdapters()
	t.Cleanup(ResetImportAdapterRegistry)
	for _, tc := range []struct {
		name, pin, env, expression string
		wantError                  bool
	}{
		{"explicit latest", "", "", `!cel 'env.ATMOS_IMPORT_TEST == "yes"'`, false},
		{"literal latest", "", "", `'!cel env.ATMOS_IMPORT_TEST == "yes"'`, true},
		{"legacy", "2026-10-19", "", `'!cel env.ATMOS_IMPORT_TEST == "yes"'`, false},
		{"boundary", "2026-10-20", "", `'!cel env.ATMOS_IMPORT_TEST == "yes"'`, true},
		{"env overrides old", "2026-10-19", "2026-10-20", `'!cel env.ATMOS_IMPORT_TEST == "yes"'`, true},
		{"env overrides new", "2026-10-20", "2026-10-19", `'!cel env.ATMOS_IMPORT_TEST == "yes"'`, false},
		{"shorthand", "", "", `'env.ATMOS_IMPORT_TEST == "yes"'`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			writeEditionTestConfig(t, fmt.Sprintf("base_path: .\nedition: %q\nimport:\n  - path: imported.yaml\n    when: %s\n", tc.pin, tc.expression))
			t.Setenv("ATMOS_IMPORT_TEST", "yes")
			if tc.env != "" {
				t.Setenv("ATMOS_EDITION", tc.env)
			}
			require.NoError(t, os.WriteFile("imported.yaml", []byte("logs:\n  level: Debug\n"), 0o600))
			config, err := LoadConfig(&schema.ConfigAndStacksInfo{})
			if tc.wantError {
				require.Error(t, err)
				require.Contains(t, err.Error(), "quoted !cel")
				return
			}
			require.NoError(t, err)
			require.Equal(t, "Debug", config.Logs.Level)
		})
	}
}

func TestExplicitTagsCommandConditions(t *testing.T) {
	for _, pin := range []string{"2026-10-19", "2026-10-20"} {
		t.Run(pin, func(t *testing.T) {
			writeEditionTestConfig(t, fmt.Sprintf("base_path: .\nedition: %q\ncommands:\n  - name: test\n    steps:\n      - command: echo hello\n        when: '!cel ci'\n", pin))
			config, err := LoadConfig(&schema.ConfigAndStacksInfo{})
			if pin == "2026-10-20" {
				// Invalid tasks may be retained as a command-local deferred load error.
				if err == nil {
					require.NotEmpty(t, config.Commands)
					err = config.Commands[0].Steps.LoadError()
				}
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.NoError(t, config.Commands[0].Steps.LoadError())
		})
	}
}

func TestExplicitTagsImportFlagPrecedence(t *testing.T) {
	setupTestAdapters()
	t.Cleanup(ResetImportAdapterRegistry)
	for _, flag := range []string{"2026-10-19", "2026-10-20"} {
		t.Run(flag, func(t *testing.T) {
			writeEditionTestConfig(t, "edition: \"2026-10-20\"\nimport:\n  - path: imported.yaml\n    when: '!cel true'\n")
			t.Setenv("ATMOS_EDITION", "2026-10-20")
			previous := GlobalViper().GetString(editionKey)
			GlobalViper().Set(editionKey, flag)
			t.Cleanup(func() { GlobalViper().Set(editionKey, previous) })
			require.NoError(t, os.WriteFile("imported.yaml", []byte("logs:\n  level: Debug\n"), 0o600))
			config, err := LoadConfig(&schema.ConfigAndStacksInfo{})
			if flag == "2026-10-20" {
				require.ErrorContains(t, err, "quoted !cel")
			} else {
				require.NoError(t, err)
				require.Equal(t, flag, config.Edition)
				require.Equal(t, "Debug", config.Logs.Level)
			}
		})
	}
}
