package terraform

import (
	bytes "bytes"
	context "context"
	fmt "fmt"
	os "os"
	filepath "path/filepath"
	strings "strings"
	testing "testing"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	degradation "github.com/cloudposse/atmos/pkg/degradation"
	iolib "github.com/cloudposse/atmos/pkg/io"
	process "github.com/cloudposse/atmos/pkg/process"
	provisioner "github.com/cloudposse/atmos/pkg/provisioner"
	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	schema "github.com/cloudposse/atmos/pkg/schema"
	store "github.com/cloudposse/atmos/pkg/store"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
	hcl "github.com/hashicorp/hcl/v2"
	hclsyntax "github.com/hashicorp/hcl/v2/hclsyntax"
	tfconfig "github.com/hashicorp/terraform-config-inspect/tfconfig"
	assert "github.com/stretchr/testify/assert"
	require "github.com/stretchr/testify/require"
	cty "github.com/zclconf/go-cty/cty"
	gomock "go.uber.org/mock/gomock"
)

func TestEnsureTerraformComponentExists_ExistingComponent(t *testing.T) {
	// Create a temporary directory structure.
	tempDir := t.TempDir()
	componentPath := filepath.Join(tempDir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentPath, 0o755))

	// Create a minimal main.tf to make it a valid component.
	mainTF := filepath.Join(componentPath, "main.tf")
	require.NoError(t, os.WriteFile(mainTF, []byte("# vpc component\n"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg:      "vpc",
		FinalComponent:        "vpc",
		ComponentFolderPrefix: "",
		ComponentSection:      map[string]any{},
	}

	err := EnsureTerraformComponentExists(atmosConfig, info)
	assert.NoError(t, err, "existing component should not return error")
}

func TestEnsureTerraformComponentExists_MissingComponentNoSource(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg:      "nonexistent",
		FinalComponent:        "nonexistent",
		ComponentFolderPrefix: "",
		ComponentSection:      map[string]any{},
	}

	err := EnsureTerraformComponentExists(atmosConfig, info)
	assert.Error(t, err, "missing component without source should return error")
	assert.Contains(t, err.Error(), "nonexistent")
}

func TestEnsureTerraformComponentExists_WorkdirPathSet(t *testing.T) {
	tempDir := t.TempDir()

	// Create workdir path.
	workdirPath := filepath.Join(tempDir, "workdir", "vpc")
	require.NoError(t, os.MkdirAll(workdirPath, 0o755))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg:      "vpc",
		FinalComponent:        "vpc",
		ComponentFolderPrefix: "",
		ComponentSection: map[string]any{
			provWorkdir.WorkdirPathKey: workdirPath,
		},
	}

	// Even though the original component path doesn't exist, the workdir path is set.
	err := EnsureTerraformComponentExists(atmosConfig, info)
	assert.NoError(t, err, "component with workdir path set should pass")
}

func TestEnsureTerraformComponentExists_WithFolderPrefix(t *testing.T) {
	tempDir := t.TempDir()
	componentPath := filepath.Join(tempDir, "components", "terraform", "myprefix", "vpc")
	require.NoError(t, os.MkdirAll(componentPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentPath, "main.tf"), []byte("# vpc\n"), 0o644))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg:      "vpc",
		FinalComponent:        "vpc",
		ComponentFolderPrefix: "myprefix",
		ComponentSection:      map[string]any{},
	}

	err := EnsureTerraformComponentExists(atmosConfig, info)
	assert.NoError(t, err, "component with folder prefix should be found")
}

func TestEnsureTerraformComponentExists_ReturnsErrorWithBasePath(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg:      "missing-comp",
		FinalComponent:        "missing-comp",
		ComponentFolderPrefix: "",
		ComponentSection:      map[string]any{},
	}

	err := EnsureTerraformComponentExists(atmosConfig, info)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "missing-comp")
	assert.Contains(t, err.Error(), filepath.Join("components", "terraform"))
}

func TestExecuteTerraformGenerateVarfileCmd_Deprecated(t *testing.T) {
	err := ExecuteTerraformGenerateVarfileCmd(nil, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDeprecatedCmdNotCallable)
}

func TestEnsureTerraformComponentExists_JITProvisionFails(t *testing.T) {
	tempDir := t.TempDir()

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	// Component doesn't exist and has a source with an unreachable URI — JIT provision will fail.
	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg:      "jit-fail",
		FinalComponent:        "jit-fail",
		ComponentFolderPrefix: "",
		ComponentSection: map[string]any{
			"source": map[string]any{
				"uri": "file:///nonexistent/path/to/source",
			},
		},
	}

	err := EnsureTerraformComponentExists(atmosConfig, info)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidTerraformComponent)
	assert.Contains(t, err.Error(), "auto-provision")
}

func TestEnsureTerraformComponentExists_PostJITComponentAppears(t *testing.T) {
	tempDir := t.TempDir()

	// Do NOT create the component directory initially.
	componentPath := filepath.Join(tempDir, "components", "terraform", "lazy-vpc")

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: filepath.Join("components", "terraform"),
			},
		},
	}

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg:      "lazy-vpc",
		FinalComponent:        "lazy-vpc",
		ComponentFolderPrefix: "",
		ComponentSection:      map[string]any{},
	}

	// First call: component doesn't exist, no source, no workdir → error.
	err := EnsureTerraformComponentExists(atmosConfig, info)
	assert.Error(t, err, "should fail when component doesn't exist")

	// Now create the directory to simulate JIT provisioning having put it there.
	require.NoError(t, os.MkdirAll(componentPath, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(componentPath, "main.tf"), []byte("# lazy-vpc\n"), 0o644))

	// Second call: component now exists at the standard path.
	err = EnsureTerraformComponentExists(atmosConfig, info)
	assert.NoError(t, err, "should pass when component exists")
}

func TestVarfileVarsToWrite(t *testing.T) {
	const secret = "genvarfile-secret-aa11bb22"
	iolib.RegisterSecret(secret)

	newInfo := func() *schema.ConfigAndStacksInfo {
		return &schema.ConfigAndStacksInfo{
			ComponentVarsSection: map[string]any{
				"db_password": secret,
				"region":      "us-east-1-genvarfile",
			},
		}
	}

	t.Run("default omits secrets", func(t *testing.T) {
		info := newInfo()
		got := VarfileVarsToWrite(info, false, "test.tfvars.json")
		_, hasSecret := got["db_password"]
		assert.False(t, hasSecret, "secret var must be omitted from the varfile by default")
		assert.Equal(t, "us-east-1-genvarfile", got["region"], "non-secret var must remain")
	})

	t.Run("with-secrets includes secrets", func(t *testing.T) {
		info := newInfo()
		got := VarfileVarsToWrite(info, true, "test.tfvars.json")
		assert.Equal(t, secret, got["db_password"], "secret var must be written with --with-secrets")
		assert.Equal(t, "us-east-1-genvarfile", got["region"])
	})
}

func TestTerraformSecretVars_NoSecrets(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		ComponentVarsSection: map[string]any{"region": "us-west-2-nosecret-xyz", "count": 1},
	}
	ComputeTerraformSecretVarKeys(info)
	assert.Nil(t, info.TerraformSecretVarKeys, "no secret keys expected when no secrets present")

	env, err := SecretVarEnv(info)
	require.NoError(t, err)
	assert.Empty(t, env, "no TF_VAR_ entries expected when no secrets present")
}

func TestRejectComputedTerraformVars(t *testing.T) {
	assert.NoError(t, RejectComputedTerraformVars(map[string]any{"region": "us-east-1"}))
	err := RejectComputedTerraformVars(map[string]any{"value": degradation.AtmosComputedValue{}})
	assert.ErrorIs(t, err, errUtils.ErrUnresolvedComputedTerraformVar)
	err = RejectComputedTerraformVars(map[string]any{
		"nested": []any{map[string]any{"value": degradation.AtmosComputedValue{}}},
	})
	assert.ErrorIs(t, err, errUtils.ErrUnresolvedComputedTerraformVar)
}

const nativeReferenceSecret = "synthetic-reference-secret-63f7a92"

// setupNativeSecretReference loads real stack files with a mock secret store.
// Mocking the component describer would miss the inspection-mode regression.
func setupNativeSecretReference(t *testing.T) (*schema.AtmosConfiguration, *store.MockStore) {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"atmos.yaml": `base_path: "."
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths: ["**/*"]
templates:
  settings:
    enabled: true
    evaluations: 1
settings:
  terminal:
    mask:
      enabled: true
`,
		"components/terraform/producer/main.tf": "",
		"components/terraform/consumer/main.tf": "variable \"credential\" { type = string }\n",
		"stacks/dev.yaml": `components:
  terraform:
    producer:
      secrets:
        vars:
          CREDENTIAL:
            store: test-secrets
            required: true
      vars:
        credential: !secret CREDENTIAL
      env:
        AWS_SECRET_ACCESS_KEY: !secret CREDENTIAL
      backend_type: s3
      backend:
        s3:
          bucket: test-state
          region: us-east-1
          access_key: !secret CREDENTIAL
          secret_key: !secret CREDENTIAL
      remote_state_backend_type: static
      remote_state_backend:
        static:
          credential: !secret CREDENTIAL
    consumer:
      vars:
        credential: !terraform.output producer credential
`,
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	}
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", dir)
	t.Setenv("ATMOS_BASE_PATH", dir)
	t.Setenv("ATMOS_MASK", "true")
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	require.NoError(t, err)
	mockStore := store.NewMockStore(gomock.NewController(t))
	atmosConfig.StoresConfig = store.StoresConfig{"test-secrets": {Type: "test", Secret: true}}
	atmosConfig.Stores = store.StoreRegistry{"test-secrets": mockStore}
	iolib.Reset()
	require.NoError(t, iolib.Initialize())
	iolib.ApplyMaskingConfig(&iolib.Config{AtmosConfig: atmosConfig})
	reset := func() {
		ResetStateCache()
		tfoutput.ResetOutputsCache()
	}
	reset()
	t.Cleanup(func() { reset(); iolib.Reset() })
	return &atmosConfig, mockStore
}

// runSecretPlanForTest is a controlled Terraform subprocess: it rejects masked
// inputs and echoes the received synthetic secret to exercise terminal redaction.
func runSecretPlanForTest() int {
	if len(os.Args) < 2 || os.Args[1] != "plan" {
		fmt.Fprintln(os.Stderr, "expected a plan invocation")
		return 1
	}
	if os.Getenv("TF_VAR_credential") != nativeReferenceSecret {
		fmt.Fprintln(os.Stderr, "plan received an incorrect credential")
		return 1
	}
	foundVarfile := false
	for i, arg := range os.Args[1:] {
		if arg != "-var-file" || i+2 >= len(os.Args) {
			continue
		}
		content, err := os.ReadFile(os.Args[i+2])
		if err != nil || strings.Contains(string(content), nativeReferenceSecret) || strings.Contains(string(content), "credential") {
			fmt.Fprintln(os.Stderr, "secret must be excluded from the generated varfile")
			return 1
		}
		foundVarfile = true
	}
	if !foundVarfile {
		return 1
	}
	fmt.Fprintln(os.Stdout, "plan credential:", os.Getenv("TF_VAR_credential"))
	fmt.Fprintln(os.Stderr, "diagnostic credential:", os.Getenv("TF_VAR_credential"))
	return 0
}

// TestSecretReferenceReachesTerraformPlan checks subprocess inputs and masked output together.
func TestSecretReferenceReachesTerraformPlan(t *testing.T) {
	config, mockStore := setupNativeSecretReference(t)
	mockStore.EXPECT().Get("dev", "producer", "CREDENTIAL").Return(nativeReferenceSecret, nil).MinTimes(1)
	exe, err := os.Executable()
	require.NoError(t, err)
	// Load the consumer through the same execution stack processor used by plan.
	info, err := testExecutor(t).services.ProcessStacks(config, schema.ConfigAndStacksInfo{
		ComponentFromArg: "consumer", ComponentType: "terraform", Stack: "dev", SubCommand: "plan",
	}, true, true, true, nil, nil)
	require.NoError(t, err)
	info.Command = exe
	info.SkipInit = true
	info.ComponentBackendType = "http" // Skip workspace selection; only plan is under test.
	info.ComponentEnvSection["_ATMOS_TEST_SECRET_PLAN"] = "1"
	execCtx, err := testExecutor(t).prepareComponentExecution(context.Background(), provisioner.OutputWriters{}, config, &info, false)
	require.NoError(t, err)
	t.Cleanup(func() {
		if info.RCCleanup != nil {
			assert.NoError(t, info.RCCleanup())
		}
	})
	var stdout, stderr bytes.Buffer
	err = testExecutor(t).executeCommandPipeline(
		config, &info, execCtx,
		WithProcessStreams(process.Streams{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}),
	)
	require.NoError(t, err, "controlled Terraform stderr: %s", stderr.String())
	assert.Contains(t, stdout.String(), "plan credential: "+iolib.MaskReplacement)
	assert.Contains(t, stderr.String(), "diagnostic credential: "+iolib.MaskReplacement)
	assert.NotContains(t, stdout.String()+stderr.String(), nativeReferenceSecret)
}

// writeSensitiveModule writes a root module declaring one sensitive and one plain input.
func writeSensitiveModule(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	module := "variable \"db_password\" {\n  sensitive = true\n}\n\nvariable \"region\" {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "variables.tf"), []byte(module), 0o600))
}

func TestRefreshTerraformModuleInfoLoadsProvisionedModule(t *testing.T) {
	provisioned := filepath.Join(t.TempDir(), "workdir")
	writeSensitiveModule(t, provisioned)
	// Stack processing inspected the not-yet-provisioned source directory and found nothing.
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{
		"component_info": map[string]any{
			cfg.ComponentPathSectionName: filepath.Join(t.TempDir(), "missing"),
			"terraform_config":           nil,
		},
	}}
	require.Empty(t, terraformSensitiveVarKeys(info))

	refreshTerraformModuleInfo(info, provisioned)

	assert.Equal(t, map[string]bool{"db_password": true}, terraformSensitiveVarKeys(info))
}

func TestRefreshTerraformModuleInfoKeepsInspectedModule(t *testing.T) {
	componentPath := t.TempDir()
	writeSensitiveModule(t, componentPath)
	inspected := &tfconfig.Module{Variables: map[string]*tfconfig.Variable{}}
	componentInfo := map[string]any{cfg.ComponentPathSectionName: componentPath, "terraform_config": inspected}
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"component_info": componentInfo}}

	refreshTerraformModuleInfo(info, componentPath)

	assert.Same(t, inspected, componentInfo["terraform_config"], "a module inspected at the same path is not reloaded")
}

func TestRefreshTerraformModuleInfoKeepsValueWhenUninspectable(t *testing.T) {
	componentInfo := map[string]any{cfg.ComponentPathSectionName: "elsewhere", "terraform_config": nil}
	info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{"component_info": componentInfo}}

	refreshTerraformModuleInfo(info, filepath.Join(t.TempDir(), "absent"))

	assert.Nil(t, componentInfo["terraform_config"])
}

func TestTerraformTestVarsKeepSensitiveRootVariablesOutOfVarfile(t *testing.T) {
	componentPath := t.TempDir()
	writeSensitiveModule(t, componentPath)
	module, diags := tfconfig.LoadModule(componentPath)
	require.False(t, diags.HasErrors())
	info := &schema.ConfigAndStacksInfo{
		SubCommand:           "test",
		ComponentVarsSection: map[string]any{"db_password": "component-value", "region": "us-east-1"},
		ComponentSection: map[string]any{
			"component_info": map[string]any{cfg.ComponentPathSectionName: componentPath, "terraform_config": module},
			cfg.TestSectionName: map[string]any{
				cfg.VarsSectionName: map[string]any{"db_password": "unregistered-test-value", "region": "us-west-2"},
			},
		},
	}
	require.True(t, iolib.MaskingEnabled(), "masking must be enabled for declaration-aware partitioning")

	assert.Equal(t, map[string]any{"region": "us-west-2"}, diskSafeTerraformTestVars(info))
	// The colliding component value is dropped too, so the test value's TF_VAR_ entry wins.
	assert.NotContains(t, diskSafeVars(info), "db_password")
	env, err := terraformTestSecretVarEnv(info)
	require.NoError(t, err)
	assert.Equal(t, []string{"TF_VAR_db_password=unregistered-test-value"}, env)
}

func TestWriteComponentVarfileOmitsDeclaredSensitiveVariables(t *testing.T) {
	componentPath := t.TempDir()
	writeSensitiveModule(t, componentPath)
	require.True(t, iolib.MaskingEnabled(), "masking must be enabled for declaration-aware partitioning")

	vars := map[string]any{"db_password": "unregistered-literal-value", "region": "us-east-1"}
	// Batch generation renders components from the raw stacks map, so the section carries no
	// component_info and the module must be inspected from the component path.
	newRun := func(dir string) *generateRun {
		return &generateRun{
			atmosConfig:  &schema.AtmosConfiguration{},
			fileTemplate: filepath.Join(dir, "out.tfvars.json"),
			format:       varfileFormatJSON,
		}
	}
	ref := componentRef{terraformComponentPath: componentPath}

	t.Run("masking enabled drops declared-sensitive keys", func(t *testing.T) {
		dir := t.TempDir()
		info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}}

		require.NoError(t, writeComponentVarfile(newRun(dir), info, vars, ref))

		written, err := os.ReadFile(filepath.Join(dir, "out.tfvars.json"))
		require.NoError(t, err)
		assert.NotContains(t, string(written), "unregistered-literal-value")
		assert.Contains(t, string(written), "us-east-1")
	})

	t.Run("module inspected during stack processing is reused", func(t *testing.T) {
		dir := t.TempDir()
		module, diags := tfconfig.LoadModule(componentPath)
		require.False(t, diags.HasErrors())
		info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{
			"component_info": map[string]any{"terraform_config": module},
		}}

		require.NoError(t, writeComponentVarfile(newRun(dir), info, vars, componentRef{terraformComponentPath: filepath.Join(dir, "missing")}))

		written, err := os.ReadFile(filepath.Join(dir, "out.tfvars.json"))
		require.NoError(t, err)
		assert.NotContains(t, string(written), "unregistered-literal-value")
	})

	t.Run("component without a module keeps every variable", func(t *testing.T) {
		dir := t.TempDir()
		info := &schema.ConfigAndStacksInfo{ComponentSection: map[string]any{}}

		require.NoError(t, writeComponentVarfile(newRun(dir), info, vars, componentRef{terraformComponentPath: filepath.Join(dir, "missing")}))

		written, err := os.ReadFile(filepath.Join(dir, "out.tfvars.json"))
		require.NoError(t, err)
		assert.Contains(t, string(written), "unregistered-literal-value")
	})
}

func TestWriteTerraformTestVars_RejectsComputedValues(t *testing.T) {
	for _, test := range []struct {
		name   string
		vars   map[string]any
		dryRun bool
		reject bool
	}{
		{name: "computed placeholder", vars: map[string]any{"fixture": degradation.AtmosComputedValue{}}, reject: true},
		{name: "computed placeholder nested", vars: map[string]any{"fixture": []any{map[string]any{"id": degradation.AtmosComputedValue{}}}}, reject: true},
		{name: "computed placeholder in a dry run", vars: map[string]any{"fixture": degradation.AtmosComputedValue{}}, dryRun: true, reject: true},
		{name: "resolved values", vars: map[string]any{"fixture": "vpc-123"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			atmosConfig := schema.AtmosConfiguration{BasePath: t.TempDir()}
			info := schema.ConfigAndStacksInfo{
				SubCommand:    "test",
				ContextPrefix: "ctx",
				Component:     "mycomp",
				DryRun:        test.dryRun,
				ComponentSection: map[string]any{
					cfg.TestSectionName: map[string]any{cfg.VarsSectionName: test.vars},
				},
			}
			varfile := ConstructTerraformComponentTestVarfilePath(&atmosConfig, &info)
			require.NoError(t, os.MkdirAll(filepath.Dir(varfile), 0o755))

			err := writeTerraformTestVars(&atmosConfig, &info)

			if test.reject {
				require.ErrorIs(t, err, errUtils.ErrUnresolvedComputedTerraformVar)
				assert.NoFileExists(t, varfile, "a rejected computed value is never written")
				return
			}
			require.NoError(t, err)
			assert.FileExists(t, varfile)
		})
	}
}

func TestSecretVarEnv_QuotesStringsForVariablesTerraformParsesAsHCL(t *testing.T) {
	secret := "declared-type-secret-${value}-5b3e"
	iolib.RegisterSecret(secret)
	module := &tfconfig.Module{Variables: map[string]*tfconfig.Variable{
		"any_token":    {Name: "any_token", Type: "any"},
		"string_token": {Name: "string_token", Type: "string"},
	}}
	info := &schema.ConfigAndStacksInfo{
		ComponentVarsSection: map[string]any{"any_token": secret, "string_token": secret, "untyped_token": secret},
		ComponentSection: map[string]any{
			"component_info": map[string]any{"terraform_config": module},
		},
	}
	ComputeTerraformSecretVarKeys(info)

	env, err := SecretVarEnv(info)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{
		`TF_VAR_any_token="declared-type-secret-$${value}-5b3e"`,
		"TF_VAR_string_token=" + secret,
		"TF_VAR_untyped_token=" + secret,
	}, env)
}

func TestSecretEnvForDeclaredTypesEncodesStringsForTheDeclaredType(t *testing.T) {
	module := &tfconfig.Module{Variables: map[string]*tfconfig.Variable{}}
	for name, typeText := range map[string]string{
		"typed_string": "string", "typed_number": "number", "typed_any": "any", "typed_list": "list(string)",
		"typed_object": "object({ token = string })", "spaced_any": "  any ",
	} {
		module.Variables[name] = &tfconfig.Variable{Name: name, Type: typeText}
	}
	section := map[string]any{"component_info": map[string]any{"terraform_config": module}}

	for _, test := range []struct {
		name  string
		value any
		want  string
	}{
		{name: "typed_string", value: "plain", want: "plain"},
		{name: "typed_number", value: "42", want: "42"},
		{name: "undeclared", value: "plain", want: "plain"},
		{name: "typed_any", value: "plain", want: `"plain"`},
		{name: "spaced_any", value: "plain", want: `"plain"`},
		{name: "typed_list", value: "plain", want: `"plain"`},
		{name: "typed_object", value: "plain", want: `"plain"`},
		{name: "typed_any", value: `say "hi"\n`, want: `"say \"hi\"\\n"`},
		{name: "typed_any", value: "token-${secret}", want: `"token-$${secret}"`},
		{name: "typed_any", value: "token-%{if x}", want: `"token-%%{if x}"`},
		{name: "typed_any", value: "line1\nline2", want: `"line1\nline2"`},
		{name: "typed_object", value: map[string]any{"token": "s"}, want: `{"token":"s"}`},
		{name: "typed_list", value: []any{"a", "b"}, want: `["a","b"]`},
		{name: "typed_number", value: 7, want: "7"},
		{name: "undeclared", value: true, want: "true"},
	} {
		t.Run(test.name+"/"+test.want, func(t *testing.T) {
			env, err := secretEnvForDeclaredTypes(section, map[string]any{test.name: test.value})

			require.NoError(t, err)
			require.Equal(t, []string{"TF_VAR_" + test.name + "=" + test.want}, env)
		})
	}
}

func TestSecretEnvForDeclaredTypesWithoutAModuleKeepsStringsVerbatim(t *testing.T) {
	env, err := secretEnvForDeclaredTypes(nil, map[string]any{"a": "plain", "b": `has "quotes"`})

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"TF_VAR_a=plain", `TF_VAR_b=has "quotes"`}, env)
}

// TestSecretEnvForDeclaredTypesCollectionsRoundTripThroughHCL parses the environment value the way Terraform does
// and verifies that interpolation-like text in nested strings and object keys is unchanged.
func TestSecretEnvForDeclaredTypesCollectionsRoundTripThroughHCL(t *testing.T) {
	module := &tfconfig.Module{Variables: map[string]*tfconfig.Variable{
		"typed_object": {Name: "typed_object", Type: "map(any)"},
		"typed_list":   {Name: "typed_list", Type: "list(any)"},
		"typed_any":    {Name: "typed_any", Type: "any"},
		"typed_typed":  {Name: "typed_typed", Type: "map(list(string))"},
	}}
	section := map[string]any{"component_info": map[string]any{"terraform_config": module}}

	for _, test := range []struct {
		name  string
		value any
		want  any
	}{
		{name: "typed_object", value: map[string]any{"k": "a${b}", "c%{if x}": "%{if y}z%{endif}"}, want: map[string]any{"k": "a${b}", "c%{if x}": "%{if y}z%{endif}"}},
		{name: "typed_list", value: []any{"${first}", "plain", "%{for x in y}"}, want: []any{"${first}", "plain", "%{for x in y}"}},
		{name: "typed_any", value: map[string]any{"outer": map[string]any{"${key}": []any{"x${y}", "$${already}"}}}, want: map[string]any{"outer": map[string]any{"${key}": []any{"x${y}", "$${already}"}}}},
		{name: "typed_typed", value: map[string][]string{"%{k}": {"${v}"}}, want: map[string]any{"%{k}": []any{"${v}"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			env, err := secretEnvForDeclaredTypes(section, map[string]any{test.name: test.value})
			require.NoError(t, err)
			require.Len(t, env, 1)

			text := strings.TrimPrefix(env[0], "TF_VAR_"+test.name+"=")
			expr, diags := hclsyntax.ParseExpression([]byte(text), "env", hcl.InitialPos)
			require.False(t, diags.HasErrors(), diags.Error())
			got, diags := expr.Value(nil)
			require.False(t, diags.HasErrors(), diags.Error())

			// Render the evaluated value back to generic Go values for comparison.
			assert.Equal(t, test.want, ctyToGeneric(t, got))
		})
	}
}

// evalHCLEnvValue parses a TF_VAR_ environment entry the way Terraform does for an HCL-typed variable.
func evalHCLEnvValue(t *testing.T, entry, name string) cty.Value {
	t.Helper()
	text := strings.TrimPrefix(entry, "TF_VAR_"+name+"=")
	expr, diags := hclsyntax.ParseExpression([]byte(text), "env", hcl.InitialPos)
	require.False(t, diags.HasErrors(), diags.Error())
	got, diags := expr.Value(nil)
	require.False(t, diags.HasErrors(), diags.Error())
	return got
}

// TestSecretEnvForDeclaredTypesUsesOnlyHCLEscapes verifies that characters JSON escapes in ways HCL string
// literals do not accept (backspace and form feed) survive the round trip, for scalars, collection values, and
// object keys, and that primitive-typed and untyped variables stay verbatim.
func TestSecretEnvForDeclaredTypesUsesOnlyHCLEscapes(t *testing.T) {
	module := &tfconfig.Module{Variables: map[string]*tfconfig.Variable{
		"typed_any":    {Name: "typed_any", Type: "any"},
		"typed_map":    {Name: "typed_map", Type: "map(string)"},
		"typed_list":   {Name: "typed_list", Type: "list(string)"},
		"typed_string": {Name: "typed_string", Type: "string"},
	}}
	section := map[string]any{"component_info": map[string]any{"terraform_config": module}}
	const special = "a\bb\fc\x00d\x1fe\x7ff\ng\rh\ti\"j\\k${l}%{m}"

	t.Run("scalar", func(t *testing.T) {
		env, err := secretEnvForDeclaredTypes(section, map[string]any{"typed_any": special})
		require.NoError(t, err)
		require.Len(t, env, 1)
		assert.Equal(t, special, evalHCLEnvValue(t, env[0], "typed_any").AsString())
	})

	t.Run("collections with keys", func(t *testing.T) {
		env, err := secretEnvForDeclaredTypes(section, map[string]any{
			"typed_map":  map[string]string{"k\b\f": "v\f\b", special: special},
			"typed_list": []string{"\b", "\f", special},
		})
		require.NoError(t, err)
		require.Len(t, env, 2)
		for _, entry := range env {
			name := "typed_map"
			if strings.HasPrefix(entry, "TF_VAR_typed_list=") {
				name = "typed_list"
			}
			got := ctyToGeneric(t, evalHCLEnvValue(t, entry, name))
			if name == "typed_map" {
				assert.Equal(t, map[string]any{"k\b\f": "v\f\b", special: special}, got)
			} else {
				assert.Equal(t, []any{"\b", "\f", special}, got)
			}
		}
	})

	t.Run("primitive typed and untyped stay verbatim", func(t *testing.T) {
		env, err := secretEnvForDeclaredTypes(section, map[string]any{"typed_string": "a\bb\f", "untyped": "a\bb\f"})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"TF_VAR_typed_string=a\bb\f", "TF_VAR_untyped=a\bb\f"}, env)
	})
}

// ctyToGeneric converts an evaluated HCL value into maps, slices, and strings.
func ctyToGeneric(t *testing.T, value cty.Value) any {
	t.Helper()
	switch {
	case value.Type() == cty.String:
		return value.AsString()
	case value.Type().IsObjectType() || value.Type().IsMapType():
		out := map[string]any{}
		for key, child := range value.AsValueMap() {
			out[key] = ctyToGeneric(t, child)
		}
		return out
	case value.Type().IsTupleType() || value.Type().IsListType():
		out := []any{}
		for _, child := range value.AsValueSlice() {
			out = append(out, ctyToGeneric(t, child))
		}
		return out
	default:
		t.Fatalf("unsupported value type %s", value.Type().FriendlyName())
		return nil
	}
}
