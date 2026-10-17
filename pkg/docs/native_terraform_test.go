package docs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tfdocsFormat "github.com/terraform-docs/terraform-docs/format"
	tfdocsPrint "github.com/terraform-docs/terraform-docs/print"
	tfdocsTf "github.com/terraform-docs/terraform-docs/terraform"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/datasource"
)

// nativeDocsFixture locates the shared Terraform module and recovered README templates.
func nativeDocsFixture(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("../../tests/fixtures/scenarios/docs-generate/native-terraform")
	require.NoError(t, err)
	return dir
}

// readDocsTestFile reads a fixture or generated document, failing the test on I/O errors.
func readDocsTestFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(content)
}

// Render through the production pipeline, then compare both a reviewed golden
// and the independently rendered, unmodified template recovered from PR #1074.
func TestGenerateDocument_NativeTerraformOriginalDesign(t *testing.T) {
	dir := nativeDocsFixture(t)
	var config struct {
		Docs schema.Docs `yaml:"docs"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(readDocsTestFile(t, filepath.Join(dir, "atmos.yaml"))), &config))
	docs := config.Docs.Generate["readme"]
	require.True(t, docs.Terraform.Enabled)
	docs.Input = []any{filepath.Join(dir, "README.yaml")}
	docs.Template = filepath.Join(dir, "README.md.gomplate")
	docs.Output = filepath.Join(t.TempDir(), "README.md")
	require.NoError(t, Generate(&schema.AtmosConfiguration{}, dir, &docs))
	actual := readDocsTestFile(t, docs.Output)
	// Source positions contain the module's absolute path; make it portable.
	normalizePaths := func(s string) string {
		return strings.ReplaceAll(strings.ReplaceAll(s, dir, "."), `.\`, "./")
	}
	assert.Equal(t, readDocsTestFile(t, filepath.Join(dir, "expected.md")), normalizePaths(trimDocsTrailingSpace(actual)))

	var original struct {
		Content string `yaml:"content"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(readDocsTestFile(t, filepath.Join(dir, "original.terraform-docs.yaml"))), &original))
	settings := tfdocsPrint.DefaultConfig()
	settings.ModuleRoot = dir
	settings.Sort.By = "required"
	settings.Settings.LockFile = false
	module, err := tfdocsTf.LoadWithOptions(settings)
	require.NoError(t, err)
	formatter := tfdocsFormat.NewMarkdownDocument(settings)
	require.NoError(t, formatter.Generate(module))
	reference, err := formatter.Render(original.Content)
	require.NoError(t, err)
	_, body, ok := strings.Cut(actual, "## Version Requirements")
	require.True(t, ok)
	assert.Equal(t, nonblankDocsLines(normalizePaths(reference)),
		nonblankDocsLines(normalizePaths("## Version Requirements"+body)))
}

// trimDocsTrailingSpace removes incidental trailing whitespace from generated Markdown.
func trimDocsTrailingSpace(s string) string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
}

// Ignore blank lines and surrounding indentation only, preserving the actual
// content of every line (including links, HTML, values, and section order).
func nonblankDocsLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func TestGenerateDocument_NativeTerraformMethods(t *testing.T) {
	dir := nativeDocsFixture(t)
	tmpl := `{{ $data := ds "config" }}{{ $data.name }}|{{ $data.terraform }}
{{ with terraform }}
{{ range .RequiredInputs }}required={{ .Name }}:{{ .HasDefault }}:{{ .GetValue }};{{ end }}
{{ range .OptionalInputs }}optional={{ .Name }}:{{ .HasDefault }}:{{ .GetValue }};{{ end }}
{{ range .Inputs }}type={{ .Name }}:{{ .Type.Raw }};description={{ .Description.Raw }};position={{ filepath.Base .Position.Filename }}:{{ .Position.Line }};{{ end }}
{{ range .Resources }}resource={{ .Spec }}:{{ .GetMode }}:{{ .URL }};position={{ filepath.Base .Position.Filename }}:{{ .Position.Line }};{{ end }}
providers={{ len .Providers }};outputs={{ len .Outputs }};modules={{ len .ModuleCalls }};requirements={{ len .Requirements }}
{{ end }}
legacy={{ $data.terraform_docs }}`
	docs := nativeDocsTestConfig(t, tmpl)
	docs.Input = []any{map[string]any{"name": "metadata", "terraform": "user-owned key"}}
	// No section display flags: the native object still contains every section.
	require.NoError(t, Generate(&schema.AtmosConfiguration{}, dir, &docs))
	actual := readDocsTestFile(t, docs.Output)
	for _, expected := range []string{
		"metadata|user-owned key", "required=bucket_name:false:;", "required=namespace:false:;",
		"optional=optional_note:true:null;", "optional=legacy_mode:true:false;", "optional=replicas:true:2;",
		"type=configuration:object({", "description=Name of the assets bucket.\nMust be unique across AWS.",
		"position=main.tf:", "position=variables.tf:1;", "position=context.tf:1;",
		"resource=aws_s3_bucket.assets:resource:https://registry.terraform.io/providers/hashicorp/aws/",
		"resource=aws_caller_identity.current:data source:", "providers=1;outputs=2;modules=3;requirements=2",
		"## Requirements", "## Providers", "## Inputs", "## Outputs",
	} {
		assert.Contains(t, actual, expected)
	}
}

// nativeDocsTestConfig creates an isolated template and output with Terraform enabled.
func nativeDocsTestConfig(t *testing.T, tmpl string) schema.DocsGenerate {
	t.Helper()
	dir := t.TempDir()
	templatePath := filepath.Join(dir, "README.md.gomplate")
	require.NoError(t, os.WriteFile(templatePath, []byte(tmpl), 0o600))
	return schema.DocsGenerate{
		Template: templatePath,
		Output:   filepath.Join(dir, "README.md"),
		Terraform: schema.TerraformDocsReadmeSettings{
			Enabled: true,
			SortBy:  "name",
		},
	}
}

func TestGenerateDocument_NativeTerraformUnavailable(t *testing.T) {
	for _, mode := range []string{"disabled", "missing source", "empty module"} {
		t.Run(mode, func(t *testing.T) {
			docs := nativeDocsTestConfig(t, `{{ with terraform }}inputs={{ len .Inputs }}{{ else }}unavailable{{ end }}`)
			want := "unavailable"
			switch mode {
			case "disabled":
				docs.Terraform.Enabled = false
			case "missing source":
				docs.Terraform.Source = "nonexistent"
			case "empty module":
				want = "inputs=0"
			}
			require.NoError(t, Generate(&schema.AtmosConfiguration{}, t.TempDir(), &docs))
			assert.Equal(t, want, readDocsTestFile(t, docs.Output))
		})
	}
}

func TestGenerateDocument_NativeTerraformErrors(t *testing.T) {
	for _, mode := range []string{"invalid terraform", "invalid template", "invalid method"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			docs := nativeDocsTestConfig(t, `{{ (terraform).DoesNotExist }}`)
			wantErr := errUtils.ErrRenderTemplate
			switch mode {
			case "invalid terraform":
				require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`variable "broken" {`), 0o600))
				wantErr = errUtils.ErrGenerateTerraformDocs
			case "invalid template":
				require.NoError(t, os.WriteFile(docs.Template, []byte(`{{ with terraform }}`), 0o600))
			}
			err := Generate(&schema.AtmosConfiguration{}, dir, &docs)
			require.ErrorIs(t, err, wantErr)
			_, err = os.Stat(docs.Output)
			assert.True(t, os.IsNotExist(err), "render failure must not write a README")
		})
	}
}

func TestGenerateDocument_NativeTerraformRenderIsolation(t *testing.T) {
	renderer := defaultTemplateRenderer{}
	for _, name := range []string{"first", "second"} {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`variable "`+name+`" { type = string }`), 0o600))
		docs := nativeDocsTestConfig(t, `{{ range (terraform).Inputs }}{{ .Name }}{{ end }}`)
		require.NoError(t, generateDocument(&schema.AtmosConfiguration{}, dir, &docs, renderer))
		assert.Equal(t, name, readDocsTestFile(t, docs.Output))
	}
	// A subsequent render without Terraform must not inherit the previous module.
	docs := nativeDocsTestConfig(t, `{{ if terraform }}leaked{{ else }}unavailable{{ end }}`)
	docs.Terraform.Enabled = false
	require.NoError(t, generateDocument(&schema.AtmosConfiguration{}, t.TempDir(), &docs, renderer))
	assert.Equal(t, "unavailable", readDocsTestFile(t, docs.Output))
	// The helper is not registered globally for unrelated template consumers.
	_, err := datasource.Render(nil, "unrelated", `{{ terraform }}`, map[string]any{}, nil)
	require.ErrorContains(t, err, `function "terraform" not defined`)
}
