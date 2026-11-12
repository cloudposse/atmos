package exec

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type provenanceImportRuntime struct {
	*MockComponentConfigurationRuntime
}

func (r *provenanceImportRuntime) ImportComponents(_ context.Context, uri string) (map[string]any, bool, error) {
	return map[string]any{"app": map[string]any{"settings": map[string]any{"configuration": map[string]any{"source": uri}}}}, true, nil
}

func TestConfigurationImportProvenance(t *testing.T) {
	for _, tc := range []struct {
		name, first, second string
		conflict            bool
		source              string
	}{
		{name: "ordinary YAML source settings", first: "", second: "", source: ""},
		{name: "same unit with different YAML overrides", first: "terragrunt://same", second: "terragrunt://same", source: "terragrunt://same"},
		{name: "distinct units hidden by YAML overrides", first: "terragrunt://first", second: "terragrunt://second", conflict: true},
		{name: "unit and YAML source setting", first: "terragrunt://first", second: "", source: "terragrunt://first"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			write := func(name, content string) {
				require.NoError(t, os.WriteFile(filepath.Join(root, name+".yaml"), []byte(content), 0o600))
			}
			for i, unit := range []string{tc.first, tc.second} {
				name := []string{"first", "second"}[i]
				content := ""
				if unit != "" {
					content = "import:\n  - " + unit + "\n"
				}
				// Distinct units get the same YAML source to ensure merging cannot hide the conflict.
				yamlSource := name
				if tc.conflict {
					yamlSource = "same-override"
				}
				write(name, content+"components:\n  terraform:\n    app:\n      settings:\n        configuration:\n          source: "+yamlSource+"\n")
				write(name+"-nested", "import:\n  - "+name+"\n")
			}
			write("stack", "import:\n  - first-nested\n  - second-nested\n")
			runtime := &provenanceImportRuntime{NewMockComponentConfigurationRuntime(gomock.NewController(t))}
			config := &schema.AtmosConfiguration{BasePathAbsolute: root, ComponentConfigurations: runtime}
			config.Components.Terraform.ConfigurationProviders = map[string]schema.ComponentConfiguration{"terragrunt": {Enabled: true}}
			result, err := ProcessYAMLConfigFile(config, root, filepath.Join(root, "stack.yaml"), map[string]map[string]any{}, nil, false, true, false, false, nil, nil, nil, nil, "")
			if tc.conflict {
				require.ErrorIs(t, err, errUtils.ErrConfigurationImportConflict)
				return
			}
			require.NoError(t, err)
			if tc.source == "" {
				require.Empty(t, result.ImportedUnitSources)
			} else {
				require.Equal(t, map[string]string{"app": tc.source}, result.ImportedUnitSources)
			}
			app := result.DeepMergedConfig["components"].(map[string]any)["terraform"].(map[string]any)["app"].(map[string]any)
			require.Equal(t, "second", app["settings"].(map[string]any)["configuration"].(map[string]any)["source"])
		})
	}
}
