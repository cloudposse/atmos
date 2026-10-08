package exec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Guard the stack naming configuration exercised by these regressions.
var _ = schema.Stacks{NamePattern: "{stage}", NameTemplate: "{{ .vars.stage }}"}

// TestTerraformGenerators_ResolvedContext exercises the same stack filters and
// output templates exposed by the batch generate varfiles/backends commands.
func TestTerraformGenerators_ResolvedContext(t *testing.T) {
	for _, generator := range []struct {
		name string
		run  func(*schema.AtmosConfiguration, string, string, []string, []string) error
	}{
		{"varfiles", ExecuteTerraformGenerateVarfiles},
		{"backends", ExecuteTerraformGenerateBackends},
	} {
		t.Run(generator.name, func(t *testing.T) {
			for _, naming := range []struct {
				name, template, logicalStack string
			}{
				{"pattern", "", "dev"},
				{"template", "{{ .vars.stage }}-stack", "dev-stack"},
				{"transformed template", "{{ upper .vars.stage }}-stack", "DEV-stack"},
			} {
				t.Run(naming.name, func(t *testing.T) {
					for _, filter := range []struct {
						name   string
						stacks []string
						write  bool
					}{
						{"all", nil, true},
						{"logical", []string{naming.logicalStack}, true},
						{"physical", []string{"stack"}, true},
						{"unmatched", []string{"prod"}, false},
					} {
						t.Run(filter.name, func(t *testing.T) {
							config, componentDir := terraformGeneratorContextFixture(t)
							config.Stacks.NameTemplate = naming.template
							fileTemplate := filepath.Join("{component-path}", "{stage}-{component}.json")
							err := generator.run(config, fileTemplate, "json", filter.stacks, []string{"network/vpc"})
							require.NoError(t, err)

							outputPath := filepath.Join(componentDir, "dev-network-vpc.json")
							if !filter.write {
								assert.NoFileExists(t, outputPath)
								return
							}
							assertTerraformGeneratorContextOutput(t, generator.name, outputPath)
						})
					}
				})
			}
		})
	}
}

// Invalid computed naming inputs must fail before generating output.
func TestTerraformGenerators_InvalidResolvedContext(t *testing.T) {
	for _, generator := range []struct {
		name string
		run  func(*schema.AtmosConfiguration, string, string, []string, []string) error
	}{
		{"varfiles", ExecuteTerraformGenerateVarfiles},
		{"backends", ExecuteTerraformGenerateBackends},
	} {
		t.Run(generator.name, func(t *testing.T) {
			for _, nameTemplate := range []string{"", "{{ index .vars.stage 0 }}"} {
				t.Run(nameTemplate, func(t *testing.T) {
					config, componentDir := terraformGeneratorContextFixture(t)
					config.Stacks.NameTemplate = nameTemplate
					stackFile := config.StackConfigFilesAbsolutePaths[0]
					content, err := os.ReadFile(stackFile)
					require.NoError(t, err)
					content = []byte(strings.ReplaceAll(string(content), `return "dev"`, `return ""`))
					require.NoError(t, os.WriteFile(stackFile, content, 0o644))
					outputPath := filepath.Join(componentDir, "output.json")
					err = generator.run(config, outputPath, "json", nil, []string{"network/vpc"})
					if nameTemplate == "" {
						require.ErrorIs(t, err, errUtils.ErrStackNamePatternPartMissing)
					} else {
						var templateError template.ExecError
						require.ErrorAs(t, err, &templateError)
					}
					assert.NoFileExists(t, outputPath)
				})
			}
		})
	}
}

func assertTerraformGeneratorContextOutput(t *testing.T, generatorName, outputPath string) {
	t.Helper()
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err, "computed context must select the stack and expand output tokens")
	var written map[string]any
	require.NoError(t, json.Unmarshal(content, &written))
	if generatorName == "varfiles" {
		assert.Equal(t, "dev", written["stage"])
		return
	}
	assert.Equal(t, map[string]any{
		"terraform": map[string]any{"backend": map[string]any{
			"s3": map[string]any{
				"bucket":               "dev-bucket",
				"key":                  "terraform.tfstate",
				"workspace_key_prefix": "network-vpc",
			},
		}},
	}, written)
}

func terraformGeneratorContextFixture(t *testing.T) (*schema.AtmosConfiguration, string) {
	t.Helper()
	tempDir := t.TempDir()
	stacksDir := filepath.Join(tempDir, "stacks")
	componentDir := filepath.Join(tempDir, "components", "terraform", "network", "vpc")
	require.NoError(t, os.MkdirAll(stacksDir, 0o755))
	require.NoError(t, os.MkdirAll(componentDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentDir, "main.tf"), []byte("# vpc component\n"), 0o644))
	stackFile := filepath.Join(stacksDir, "stack.yaml")
	require.NoError(t, os.WriteFile(stackFile, []byte(`
vars:
  stage: !starlark |
    return "dev"
components:
  terraform:
    network/vpc:
      backend_type: s3
      backend:
        s3:
          bucket: !starlark |
            return ctx.vars["stage"] + "-bucket"
          key: terraform.tfstate
`), 0o644))
	return &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{Terraform: schema.Terraform{
			BasePath: filepath.Join("components", "terraform"),
		}},
		Stacks: schema.Stacks{BasePath: "stacks", NamePattern: "{stage}"},
		Templates: schema.Templates{Settings: schema.TemplatesSettings{
			Enabled: true,
		}},
		StacksBaseAbsolutePath:        stacksDir,
		TerraformDirAbsolutePath:      filepath.Join(tempDir, "components", "terraform"),
		IncludeStackAbsolutePaths:     []string{stacksDir},
		StackConfigFilesAbsolutePaths: []string{stackFile},
	}, componentDir
}
