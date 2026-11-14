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
	tfconfig "github.com/hashicorp/terraform-config-inspect/tfconfig"
	assert "github.com/stretchr/testify/assert"
	require "github.com/stretchr/testify/require"
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
