package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	cfg "github.com/cloudposse/atmos/pkg/config"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/provisioner"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/store"
	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
)

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
		componentFuncSyncMap.Clear()
	}
	reset()
	t.Cleanup(func() { reset(); iolib.Reset() })
	return &atmosConfig, mockStore
}

// nativeSecretReferenceLoaders exercises each production component-reference entry point.
func nativeSecretReferenceLoaders() map[string]func(*schema.AtmosConfiguration) (any, error) {
	return map[string]func(*schema.AtmosConfiguration) (any, error){
		"terraform.output": func(config *schema.AtmosConfiguration) (any, error) {
			value, _, err := GetTerraformOutput(config, "dev", "producer", "credential", false, nil, nil)
			return value, err
		},
		"terraform.state": func(config *schema.AtmosConfiguration) (any, error) {
			return GetTerraformState(config, "!terraform.state", "dev", "producer", "credential", false, nil, nil)
		},
		"atmos.Component": func(config *schema.AtmosConfiguration) (any, error) {
			sections, err := componentFunc(config, nil, "producer", "dev")
			if err != nil {
				return nil, err
			}
			return sections.(map[string]any)["outputs"].(map[string]any)["credential"], nil
		},
	}
}

// TestNativeReferencesResolveSecrets verifies real values, output masking, and cache hits.
func TestNativeReferencesResolveSecrets(t *testing.T) {
	for name, load := range nativeSecretReferenceLoaders() {
		t.Run(name, func(t *testing.T) {
			for _, mask := range []bool{true, false} {
				t.Run(map[bool]string{true: "masked", false: "unmasked"}[mask], func(t *testing.T) {
					config, mockStore := setupNativeSecretReference(t)
					iolib.GetContext().Masker().SetEnabled(mask)
					iolib.GetContext().Masker().SetReplacement("[REDACTED]")
					calls := 0
					mockStore.EXPECT().Get("dev", "producer", "CREDENTIAL").DoAndReturn(func(_, _, _ string) (any, error) {
						calls++
						return nativeReferenceSecret, nil
					}).MinTimes(1)
					value, err := load(config)
					require.NoError(t, err)
					assert.Equal(t, nativeReferenceSecret, value)
					before := calls
					cached, err := load(config)
					require.NoError(t, err)
					assert.Equal(t, value, cached)
					assert.Equal(t, before, calls, "cache hit must preserve real values without fetching again")
					if mask {
						assert.Equal(t, "[REDACTED]", iolib.MaskString(nativeReferenceSecret))
					} else {
						assert.Equal(t, nativeReferenceSecret, iolib.MaskString(nativeReferenceSecret))
					}
				})
			}
		})
	}
}

// TestNativeReferencesMissingSecretFails rejects missing credentials during execution.
func TestNativeReferencesMissingSecretFails(t *testing.T) {
	for name, load := range nativeSecretReferenceLoaders() {
		t.Run(name, func(t *testing.T) {
			config, mockStore := setupNativeSecretReference(t)
			mockStore.EXPECT().Get("dev", "producer", "CREDENTIAL").Return(nil, errors.New("missing test secret")).MinTimes(1)
			_, err := load(config)
			require.ErrorIs(t, err, secrets.ErrSecretMissing)
		})
	}
}

// TestOutputComponentDescriberPreservesSecretConfiguration checks backend and input values.
func TestOutputComponentDescriberPreservesSecretConfiguration(t *testing.T) {
	config, mockStore := setupNativeSecretReference(t)
	mockStore.EXPECT().Get("dev", "producer", "CREDENTIAL").Return(nativeReferenceSecret, nil).MinTimes(1)
	sections, err := (&componentDescriberAdapter{}).DescribeComponent(&tfoutput.DescribeComponentParams{
		AtmosConfig: config, Component: "producer", Stack: "dev", ProcessTemplates: true, ProcessYamlFunctions: true,
	})
	require.NoError(t, err)
	assert.Equal(t, nativeReferenceSecret, sections["vars"].(map[string]any)["credential"])
	assert.Equal(t, nativeReferenceSecret, sections["env"].(map[string]any)["AWS_SECRET_ACCESS_KEY"])
	backend := sections["backend"].(map[string]any)
	assert.Equal(t, nativeReferenceSecret, backend["access_key"])
	assert.Equal(t, nativeReferenceSecret, backend["secret_key"])
	var displayed bytes.Buffer
	_, err = iolib.MaskWriter(&displayed).Write([]byte(nativeReferenceSecret))
	require.NoError(t, err)
	assert.Equal(t, iolib.MaskReplacement, displayed.String())
}

// TestDescribeComponentSecretInspection checks credential-free direct inspection with provenance.
func TestDescribeComponentSecretInspection(t *testing.T) {
	for _, provenance := range []bool{false, true} {
		t.Run(map[bool]string{true: "provenance", false: "plain"}[provenance], func(t *testing.T) {
			config, mockStore := setupNativeSecretReference(t)
			mockStore.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			var rendered any
			executor := &DescribeComponentExec{
				IsTTYSupportForStdout:    func() bool { return false },
				initCliConfig:            func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) { return *config, nil },
				executeDescribeComponent: ExecuteDescribeComponent,
				printOrWriteToFile:       func(_ *schema.AtmosConfiguration, _, _ string, value any) error { rendered = value; return nil },
			}
			// Provenance rendering writes through the normal output layer rather than
			// printOrWriteToFile. A file lets us verify that actual command path too.
			output := filepath.Join(t.TempDir(), "description.yaml")
			err := executor.ExecuteDescribeComponentCmd(DescribeComponentParams{
				Component: "producer", Stack: "dev", ProcessTemplates: true, ProcessYamlFunctions: true,
				Provenance: provenance, ProvenanceExplicit: true, Format: "yaml", File: output,
			})
			require.NoError(t, err)
			if provenance {
				content, err := os.ReadFile(output)
				require.NoError(t, err)
				assert.Contains(t, string(content), iolib.MaskReplacement)
				assert.NotContains(t, string(content), nativeReferenceSecret)
			} else {
				assert.Equal(t, iolib.MaskReplacement, rendered.(map[string]any)["vars"].(map[string]any)["credential"])
			}
		})
	}
}

// TestOutputComponentDescriberSkippedSecrets preserves explicit YAML-function skips.
func TestOutputComponentDescriberSkippedSecrets(t *testing.T) {
	for _, processFunctions := range []bool{false, true} {
		t.Run(map[bool]string{true: "skip secret", false: "skip all functions"}[processFunctions], func(t *testing.T) {
			config, mockStore := setupNativeSecretReference(t)
			mockStore.EXPECT().Get(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
			sections, err := (&componentDescriberAdapter{}).DescribeComponent(&tfoutput.DescribeComponentParams{
				AtmosConfig: config, Component: "producer", Stack: "dev", ProcessTemplates: true,
				ProcessYamlFunctions: processFunctions, Skip: []string{"secret"},
			})
			require.NoError(t, err)
			assert.Equal(t, "!secret CREDENTIAL", sections["vars"].(map[string]any)["credential"])
		})
	}
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
	info, err := ProcessStacks(config, schema.ConfigAndStacksInfo{
		ComponentFromArg: "consumer", ComponentType: "terraform", Stack: "dev", SubCommand: "plan",
	}, true, true, true, nil, nil)
	require.NoError(t, err)
	info.Command = exe
	info.SkipInit = true
	info.ComponentBackendType = "http" // Skip workspace selection; only plan is under test.
	info.ComponentEnvSection["_ATMOS_TEST_SECRET_PLAN"] = "1"
	execCtx, err := prepareComponentExecution(context.Background(), provisioner.OutputWriters{}, config, &info, false)
	require.NoError(t, err)
	t.Cleanup(func() {
		if info.RCCleanup != nil {
			assert.NoError(t, info.RCCleanup())
		}
	})
	var stdout, stderr bytes.Buffer
	err = executeCommandPipeline(
		config, &info, execCtx,
		WithProcessStreams(process.Streams{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}),
	)
	require.NoError(t, err, "controlled Terraform stderr: %s", stderr.String())
	assert.Contains(t, stdout.String(), "plan credential: "+iolib.MaskReplacement)
	assert.Contains(t, stderr.String(), "diagnostic credential: "+iolib.MaskReplacement)
	assert.NotContains(t, stdout.String()+stderr.String(), nativeReferenceSecret)
}

// TestNonTerraformSecretReferences checks native references in non-Terraform execution.
func TestNonTerraformSecretReferences(t *testing.T) {
	for _, componentType := range []string{"helm", "helmfile", "kubernetes", "container"} {
		t.Run(componentType, func(t *testing.T) {
			config, mockStore := setupNativeSecretReference(t)
			// Use an additional stack file so the producer and consumer share the dev stack.
			stackFile := filepath.Join(config.BasePath, "stacks", "dev.yaml")
			content, err := os.ReadFile(stackFile)
			require.NoError(t, err)
			content = append(content, []byte(fmt.Sprintf(`
  %s:
    app:
      secrets:
        vars:
          CREDENTIAL:
            store: test-secrets
      env:
        DIRECT: !secret CREDENTIAL
        FROM_OUTPUT: !terraform.output producer credential
        FROM_STATE: !terraform.state producer credential
        FROM_COMPONENT: '{{ (atmos.Component "producer" "dev").vars.credential }}'
`, componentType))...)
			require.NoError(t, os.WriteFile(stackFile, content, 0o600))
			// Use the same base path for each built-in component type in this isolated fixture.
			config.Components.Helm.BasePath = "components/helm"
			config.Components.Helmfile.BasePath = "components/helmfile"
			config.Components.Kubernetes.BasePath = "components/kubernetes"
			config.Components.Container.BasePath = "components/container"
			require.NoError(t, os.MkdirAll(filepath.Join(config.BasePath, "components", componentType, "app"), 0o700))
			mockStore.EXPECT().Get("dev", gomock.Any(), "CREDENTIAL").Return(nativeReferenceSecret, nil).MinTimes(1)
			info, err := ProcessStacks(config, schema.ConfigAndStacksInfo{
				ComponentFromArg: "app", ComponentType: componentType, Stack: "dev",
			}, true, true, true, nil, nil)
			require.NoError(t, err)
			for _, key := range []string{"DIRECT", "FROM_OUTPUT", "FROM_STATE", "FROM_COMPONENT"} {
				assert.Equal(t, nativeReferenceSecret, info.ComponentEnvSection[key], key)
			}
			assert.Equal(t, iolib.MaskReplacement, iolib.MaskString(nativeReferenceSecret))
		})
	}
}
