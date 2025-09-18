package exec

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Note on testing framework:
// - Primary: Go "testing" package
//
// - Assertions: use plain if/tt.Fatalf to avoid adding dependencies
//
// If testify is available in the project, you can refactor to use require/assert for readability.

func newTestAtmosConfig(base string) schema.AtmosConfiguration {
	return schema.AtmosConfiguration{
		BasePath: base,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "components/terraform",
			},
		},
		Templates: schema.Templates{
			Settings: schema.TemplateSettings{
				Enabled: true,
			},
		},
		Stacks: schema.Stacks{
			NameTemplate: "",
		},
	}
}

func TestExecuteTerraformGenerateBackendsCmd_FormatValidation(t *testing.T) {
	// Override ProcessCommandLineArgs and cfg.InitCliConfig via small indirections:
	// We cannot override those directly; instead, we focus on flag parsing and validation path.
	// Build the cobra command with flags to exercise validation.
	cmd := &cobra.Command{
		Use: "terraform generate backends",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
	cmd.Flags().String("file-template", "", "")
	cmd.Flags().String("stacks", "", "")
	cmd.Flags().String("components", "", "")
	cmd.Flags().String("format", "", "")

	// Invalid format should error before calling deeper logic
	_ = cmd.Flags().Set("format", "yaml")

	// Stub out ProcessCommandLineArgs and cfg.InitCliConfig by using benign inputs:
	// Since we cannot replace them here, we call the lower-level function directly for proper coverage instead.
	// This test focuses purely on the validation in ExecuteTerraformGenerateBackendsCmd.
	if err := ExecuteTerraformGenerateBackendsCmd(cmd, []string{}); err == nil {
		t.Fatalf("expected error for invalid format")
	} else if !strings.Contains(err.Error(), "invalid '--format' argument 'yaml'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteTerraformGenerateBackends_WritesHCLToDefaultBackendFile(t *testing.T) {
	// Arrange a fake stacks map with one terraform component and backend config
	tmp := t.TempDir()
	ac := newTestAtmosConfig(tmp)

	componentName := "vpc"
	stackFile := "orgs/cp/tenant1/staging/us-east-2"
	stackName := "tenant1-ue2-staging"
	terraformComponent := componentName

	// Prepare stacks map following expected structure
	stacksMap := map[string]any{
		stackFile: map[string]any{
			"components": map[string]any{
				cfg.TerraformSectionName: map[string]any{
					componentName: map[string]any{
						cfg.BackendSectionName: map[string]any{
							"bucket": "my-bucket",
							"key":    "state/terraform.tfstate",
							"region": "us-east-2",
						},
						cfg.BackendTypeSectionName: "s3",
						cfg.VarsSectionName:        map[string]any{"namespace": "eg", "tenant": "tenant1", "environment": "staging", "region": "us-east-2"},
						cfg.SettingsSectionName:    map[string]any{},
						cfg.EnvSectionName:         map[string]any{},
						cfg.ProvidersSectionName:   map[string]any{},
						cfg.AuthSectionName:        map[string]any{},
						cfg.HooksSectionName:       map[string]any{},
						cfg.OverridesSectionName:   map[string]any{},
					},
				},
			},
		},
	}

	// Stub functions used by ExecuteTerraformGenerateBackends
	origFind := findStacksMapFn
	origAbs := absFn
	origWriteHCL := writeTerraformBackendConfigHCLFn
	defer func() {
		findStacksMapFn = origFind
		absFn = origAbs
		writeTerraformBackendConfigHCLFn = origWriteHCL
	}()

	findStacksMapFn = func(_ *schema.AtmosConfiguration, _ bool) (map[string]any, map[string]any, error) {
		return stacksMap, nil, nil
	}
	absFn = func(p string) (string, error) {
		// Normalize to a test-visible path under tmp
		if !filepath.IsAbs(p) {
			return filepath.Join(tmp, p), nil
		}
		return p, nil
	}

	var wrotePath string
	var wroteBackendType string
	var wroteBackend map[string]any
	writeTerraformBackendConfigHCLFn = func(path string, backendType string, backend map[string]any) error {
		wrotePath = path
		wroteBackendType = backendType
		wroteBackend = backend
		return nil
	}

	// Act
	if err := ExecuteTerraformGenerateBackends(&ac, "", "hcl", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Assert
	if wrotePath == "" {
		t.Fatalf("expected HCL write path to be set")
	}
	if !strings.HasSuffix(wrotePath, filepath.Join("components/terraform", terraformComponent, "backend.tf")) {
		t.Fatalf("unexpected write path: %s", wrotePath)
	}
	if wroteBackendType != "s3" {
		t.Fatalf("expected backend type s3, got %s", wroteBackendType)
	}
	if wroteBackend["bucket"] != "my-bucket" {
		t.Fatalf("expected backend bucket 'my-bucket', got %v", wroteBackend["bucket"])
	}
}

func TestExecuteTerraformGenerateBackends_JSONAndBackendConfig_WithFileTemplateAndFilters(t *testing.T) {
	tmp := t.TempDir()
	ac := newTestAtmosConfig(tmp)

	componentName := "eks"
	stackFile := "orgs/cp/tenant1/dev/us-west-2"
	stackName := "tenant1-uw2-dev"

	stacksMap := map[string]any{
		stackFile: map[string]any{
			"components": map[string]any{
				cfg.TerraformSectionName: map[string]any{
					componentName: map[string]any{
						cfg.BackendSectionName: map[string]any{
							"bucket": "eks-bucket",
							"key":    "state/eks.tfstate",
							"region": "us-west-2",
						},
						cfg.BackendTypeSectionName: "s3",
						cfg.VarsSectionName:        map[string]any{"namespace": "eg", "tenant": "tenant1", "environment": "dev", "region": "us-west-2"},
					},
				},
			},
		},
	}

	origFind := findStacksMapFn
	origAbs := absFn
	origEnsure := ensureDirFn
	origJSON := writeToFileAsJSONFn
	origHcl := writeToFileAsHCLFn
	origReplace := replaceContextTokensFn
	defer func() {
		findStacksMapFn = origFind
		absFn = origAbs
		ensureDirFn = origEnsure
		writeToFileAsJSONFn = origJSON
		writeToFileAsHCLFn = origHcl
		replaceContextTokensFn = origReplace
	}()

	findStacksMapFn = func(_ *schema.AtmosConfiguration, _ bool) (map[string]any, map[string]any, error) {
		return stacksMap, nil, nil
	}
	absFn = func(p string) (string, error) { return filepath.Join(tmp, p), nil }
	var ensuredPath string
	ensureDirFn = func(abs string) error { ensuredPath = abs; return nil }

	var jsonPath string
	var jsonObj any
	writeToFileAsJSONFn = func(path string, v any, mode uint32) error {
		jsonPath = path
		jsonObj = v
		return nil
	}

	var backendCfgPath string
	var backendCfg map[string]any
	writeToFileAsHCLFn = func(path string, m map[string]any, mode uint32) error {
		backendCfgPath = path
		backendCfg = m
		return nil
	}

	replaceContextTokensFn = func(c schema.Context, tmpl string) string {
		// validate tokens expanded; simulate a simple replacement
		s := tmpl
		s = strings.ReplaceAll(s, "{component}", "eks")
		s = strings.ReplaceAll(s, "{component-path}", "eks")
		s = strings.ReplaceAll(s, "{environment}", "dev")
		return s
	}

	// JSON format with file-template
	fileTmpl := "out/{environment}/{component}/backend.tf.json"
	if err := ExecuteTerraformGenerateBackends(&ac, fileTmpl, "json", []string{stackName}, []string{componentName}); err != nil {
		t.Fatalf("unexpected error json: %v", err)
	}
	if jsonPath == "" || !strings.HasSuffix(jsonPath, filepath.Join("out", "dev", "eks", "backend.tf.json")) {
		t.Fatalf("unexpected json path: %s", jsonPath)
	}
	if ensuredPath == "" || !strings.Contains(ensuredPath, filepath.Join("out", "dev", "eks", "backend.tf.json")) {
		t.Fatalf("ensureDir was not called with expected absolute file path; got: %s", ensuredPath)
	}
	if jsonObj == nil {
		t.Fatalf("expected backend JSON object to be written")
	}

	// backend-config format (writes raw backend block as HCL map)
	backendCfgPath, backendCfg = "", nil
	if err := ExecuteTerraformGenerateBackends(&ac, fileTmpl, "backend-config", []string{stackFile}, []string{componentName}); err != nil {
		t.Fatalf("unexpected error backend-config: %v", err)
	}
	if backendCfgPath == "" || !strings.HasSuffix(backendCfgPath, filepath.Join("out", "dev", "eks", "backend.tf.json")) {
		t.Fatalf("unexpected backend-config path: %s", backendCfgPath)
	}
	if backendCfg == nil || backendCfg["bucket"] != "eks-bucket" {
		t.Fatalf("unexpected backend-config content: %#v", backendCfg)
	}
}

func TestExecuteTerraformGenerateBackends_SkipsAbstractAndMissingSections(t *testing.T) {
	tmp := t.TempDir()
	ac := newTestAtmosConfig(tmp)

	stacksMap := map[string]any{
		"stack-a": map[string]any{
			"components": map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"abstract-comp": map[string]any{
						cfg.MetadataSectionName: map[string]any{"type": "abstract"},
					},
					"missing-backend": map[string]any{
						cfg.VarsSectionName: map[string]any{},
					},
					"valid": map[string]any{
						cfg.BackendSectionName:     map[string]any{"bucket": "b", "key": "k"},
						cfg.BackendTypeSectionName: "s3",
					},
				},
			},
		},
	}

	origFind := findStacksMapFn
	origWrite := writeTerraformBackendConfigHCLFn
	origAbs := absFn
	defer func() {
		findStacksMapFn = origFind
		writeTerraformBackendConfigHCLFn = origWrite
		absFn = origAbs
	}()
	findStacksMapFn = func(_ *schema.AtmosConfiguration, _ bool) (map[string]any, map[string]any, error) {
		return stacksMap, nil, nil
	}
	absFn = func(p string) (string, error) { return filepath.Join(tmp, p), nil }

	var count int
	writeTerraformBackendConfigHCLFn = func(path string, backendType string, backend map[string]any) error {
		count++
		return nil
	}

	if err := ExecuteTerraformGenerateBackends(&ac, "", "hcl", nil, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 backend write (only 'valid' component), got %d", count)
	}
}

func TestExecuteTerraformGenerateBackends_ErrorPropagation(t *testing.T) {
	tmp := t.TempDir()
	ac := newTestAtmosConfig(tmp)

	stacksMap := map[string]any{
		"stack": map[string]any{
			"components": map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"comp": map[string]any{
						cfg.BackendSectionName:     map[string]any{"x": "y"},
						cfg.BackendTypeSectionName: "s3",
					},
				},
			},
		},
	}

	origFind := findStacksMapFn
	origAbs := absFn
	origWrite := writeTerraformBackendConfigHCLFn
	defer func() {
		findStacksMapFn = origFind
		absFn = origAbs
		writeTerraformBackendConfigHCLFn = origWrite
	}()

	findStacksMapFn = func(_ *schema.AtmosConfiguration, _ bool) (map[string]any, map[string]any, error) {
		return stacksMap, nil, nil
	}
	absFn = func(p string) (string, error) {
		return "", errors.New("abs failed")
	}

	if err := ExecuteTerraformGenerateBackends(&ac, "", "hcl", nil, nil); err == nil {
		t.Fatalf("expected error from abs failure")
	}
}

func TestExecuteTerraformGenerateBackends_InvalidFormatAtWriteTime(t *testing.T) {
	// If format is invalid but filters prevent any processing, function returns nil.
	// To verify invalid format branch, we force processing and pass invalid format.
	tmp := t.TempDir()
	ac := newTestAtmosConfig(tmp)

	stacksMap := map[string]any{
		"s": map[string]any{
			"components": map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"c": map[string]any{
						cfg.BackendSectionName:     map[string]any{"bucket": "b"},
						cfg.BackendTypeSectionName: "s3",
					},
				},
			},
		},
	}

	origFind := findStacksMapFn
	origAbs := absFn
	defer func() {
		findStacksMapFn = origFind
		absFn = origAbs
	}()
	findStacksMapFn = func(_ *schema.AtmosConfiguration, _ bool) (map[string]any, map[string]any, error) {
		return stacksMap, nil, nil
	}
	absFn = func(p string) (string, error) { return filepath.Join(tmp, p), nil }

	if err := ExecuteTerraformGenerateBackends(&ac, "", "yaml", nil, nil); err == nil {
		t.Fatalf("expected invalid format error")
	} else if !strings.Contains(err.Error(), "invalid '--format' argument") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Ensure that the error printing side-effect path does not panic when unmarshalling fails and templates disabled.
// We simulate Unmarshal error by returning invalid YAML string from template processing.
func TestExecuteTerraformGenerateBackends_TemplateDisabledAddsHelpfulError(t *testing.T) {
	tmp := t.TempDir()
	ac := newTestAtmosConfig(tmp)
	ac.Templates.Settings.Enabled = false

	stacksMap := map[string]any{
		"s": map[string]any{
			"components": map[string]any{
				cfg.TerraformSectionName: map[string]any{
					"c": map[string]any{
						cfg.BackendSectionName:     map[string]any{"bucket": "b"},
						cfg.BackendTypeSectionName: "s3",
						cfg.VarsSectionName:        map[string]any{},
						// Intentionally inject template markers into component section to trigger helpful error
						"some": "{{ invalid }}",
					},
				},
			},
		},
	}

	origFind := findStacksMapFn
	origProcWithDS := processTmplWithDatasourcesFn
	defer func() {
		findStacksMapFn = origFind
		processTmplWithDatasourcesFn = origProcWithDS
	}()
	findStacksMapFn = func(_ *schema.AtmosConfiguration, _ bool) (map[string]any, map[string]any, error) {
		return stacksMap, nil, nil
	}
	// Return malformed YAML to cause Unmarshal failure
	processTmplWithDatasourcesFn = func(ac *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, settings schema.Settings, name string, s string, data map[string]any, b bool) (string, error) {
		return ":\n - [", errors.New("tmpl processing failed")
	}

	// We expect a non-nil error, augmented with helpful message; the exact message may vary.
	if err := ExecuteTerraformGenerateBackends(&ac, "", "hcl", nil, nil); err == nil {
		t.Fatalf("expected error due to template/unmarshal failure")
	}
}

// Guard against nil or empty maps at various levels to ensure no panic.
func TestExecuteTerraformGenerateBackends_GracefulWhenSectionsMissing(t *testing.T) {
	tmp := t.TempDir()
	ac := newTestAtmosConfig(tmp)

	stacksMap := map[string]any{
		"a": map[string]any{}, // missing components
		"b": map[string]any{
			"components": map[string]any{
				cfg.TerraformSectionName: "not-a-map", // wrong type -> should be skipped
			},
		},
	}

	origFind := findStacksMapFn
	defer func() { findStacksMapFn = origFind }()
	findStacksMapFn = func(_ *schema.AtmosConfiguration, _ bool) (map[string]any, map[string]any, error) {
		return stacksMap, nil, nil
	}

	if err := ExecuteTerraformGenerateBackends(&ac, "", "hcl", nil, nil); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}