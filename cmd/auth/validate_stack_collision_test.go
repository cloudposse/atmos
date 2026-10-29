package auth

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuthValidatePhysicalFilenamePreservesLogicalMatches(t *testing.T) {
	for _, naming := range []string{"manifest", "pattern", "template", "literal manifest", "literal template", "literal pattern", "dynamic manifest", "native manifest", "native template", "native pattern", "native conditional template"} {
		t.Run(naming, func(t *testing.T) {
			root := setupScopedAuthFixture(t)
			configPath := filepath.Join(root, "atmos.yaml")
			config, err := os.ReadFile(configPath)
			require.NoError(t, err)
			text := strings.ReplaceAll(string(config), "included_paths: ['deploy/**/*']", "included_paths: ['dev', 'other']")
			if naming == "native conditional template" {
				text = strings.ReplaceAll(text, "name_pattern: '{stage}'", "name_template: '{{ if eq .vars.stage `dev` }}dev{{ else }}other{{ end }}'")
			}
			if naming == "template" || naming == "literal template" || naming == "native template" {
				text = strings.ReplaceAll(text, "name_pattern: '{stage}'", "name_template: '{{ .vars.stage }}'")
			}
			require.NoError(t, os.WriteFile(configPath, []byte(text), 0o600))
			local, err := os.ReadFile(filepath.Join(root, "stacks", "deploy", "dev.yaml"))
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "dev.yaml"), local, 0o600))
			other := "vars: {stage: dev}\ncomponents:\n  helmfile:\n    bad: {vars: {}}\n"
			if naming == "manifest" {
				other = "name: dev\n" + strings.ReplaceAll(other, "stage: dev", "stage: other")
			}
			switch naming {
			case "literal manifest":
				other = "name: !literal dev\n" + strings.ReplaceAll(other, "stage: dev", "stage: other")
			case "literal template", "literal pattern":
				other = strings.ReplaceAll(other, "stage: dev", "stage: !literal dev")
			case "native template", "native pattern", "native conditional template":
				other = strings.ReplaceAll(other, "stage: dev", "stage: !env ATMOS_BOUNDARY_NAME")
			case "native manifest":
				other = "name: !env ATMOS_BOUNDARY_NAME\n" + strings.ReplaceAll(other, "stage: dev", "stage: other")
			case "dynamic manifest":
				other = "name: '{{ print \"dev\" }}'\n" + strings.ReplaceAll(other, "stage: dev", "stage: other")
			}
			require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "other.yaml"), []byte(other), 0o600))
			cmd := authValidateCmd
			resetAuthCmdFlags(t, cmd)
			cmd.SetContext(context.Background())
			require.NoError(t, cmd.ParseFlags([]string{"--stack", "dev"}))
			err = executeAuthValidateCommand(cmd, nil)
			if naming == "dynamic manifest" || strings.HasPrefix(naming, "native ") {
				require.ErrorContains(t, err, "cannot determine stack name")
			} else {
				require.ErrorContains(t, err, `component "bad" (helmfile)`)
			}
		})
	}
}
