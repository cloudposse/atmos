package output

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestExecutor_Regression_Issue2356_BackendFileUnchangedInSkipInitPath is a
// regression test for https://github.com/cloudposse/atmos/issues/2356.
//
// The bug: when an after-apply hook called GetOutputWithOptions(SkipInit=true),
// execute() regenerated backend.tf.json from sections that still contained
// literal "!terraform.state ..." strings (because ProcessYamlFunctions was
// disabled upstream to avoid needing credentials). This overwrote the
// correctly-rendered backend file produced by the preceding apply phase.
//
// This test writes a rendered backend.tf.json to a component dir, then drives
// the SkipInit+no-auth path, and asserts the file bytes are identical afterward.
// Without the fix (the processYamlFunctions guard in execute()), this test
// fails because the backend file is rewritten with literal YAML-function strings.
func TestExecutor_Regression_Issue2356_BackendFileUnchangedInSkipInitPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Set up a component working directory with a pre-existing, correctly-rendered
	// backend.tf.json (as if a prior `atmos terraform apply` had written it).
	// Layout: <tempDir>/terraform/my-app/backend.tf.json
	// This matches what utils.GetComponentPath produces when
	// TerraformDirAbsolutePath is set to <tempDir>/terraform and the base
	// component name is "my-app".
	tempDir := t.TempDir()
	terraformBase := filepath.Join(tempDir, "terraform")
	componentDir := filepath.Join(terraformBase, "my-app")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	backendPath := filepath.Join(componentDir, "backend.tf.json")
	renderedBackend := []byte(`{
  "terraform": {
    "backend": {
      "s3": {
        "bucket": "atmos-tfstate-dev",
        "dynamodb_table": "atmos-tfstate-lock-dev",
        "key": "terraform.tfstate",
        "region": "us-east-1"
      }
    }
  }
}`)
	require.NoError(t, os.WriteFile(backendPath, renderedBackend, 0o644))

	// Capture the bytes before the SkipInit path runs.
	before, err := os.ReadFile(backendPath)
	require.NoError(t, err)

	// Build a sections map whose backend config contains LITERAL
	// "!terraform.state ..." strings — this simulates what DescribeComponent
	// returns when ProcessYamlFunctions=false (the path driven by SkipInit=true
	// + authManager=nil). If the guard in execute() is absent, these literal
	// strings would be serialized into backend.tf.json and overwrite the
	// correctly-rendered file above.
	sections := regressionSectionsWithLiteralYamlFunctions()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockDescriber.EXPECT().
		DescribeComponent(gomock.Any()).
		Return(sections, nil).
		AnyTimes()

	mockRunner := NewMockTerraformRunner(ctrl)
	mockRunner.EXPECT().SetStdout(gomock.Any()).AnyTimes()
	mockRunner.EXPECT().SetStderr(gomock.Any()).AnyTimes()
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	mockRunner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{
		"rds_properties": {Value: []byte(`{"endpoint":"db.example.com","port":5432}`)},
	}, nil)

	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return mockRunner, nil
	}

	// Use the real defaultBackendGenerator — do NOT inject a mock. This test
	// must exercise the on-disk write path. If the guard ever regresses, the
	// real generator will clobber backend.tf.json and the byte-compare will fail.
	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))

	// Construct an atmos config that points at our temp terraform base dir and
	// enables backend auto-generation. AutoGenerateBackendFile=true is the
	// condition that — absent the guard — would cause GenerateBackendIfNeeded
	// to rewrite backend.tf.json from the un-rendered sections.
	atmosConfig := &schema.AtmosConfiguration{
		BasePath:                 tempDir,
		TerraformDirAbsolutePath: terraformBase,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath:                "terraform",
				AutoGenerateBackendFile: true,
				InitRunReconfigure:      false,
			},
		},
		Logs: schema.Logs{
			Level: "info",
		},
	}

	// Clear any prior cache entry so we hit the describe+execute path.
	stackSlug := stackComponentKey("dev", "my-app")
	terraformOutputsCache.Delete(stackSlug)
	defer terraformOutputsCache.Delete(stackSlug)

	// Drive the same path that the after-apply store hook uses:
	// SkipInit=true + authManager=nil => ProcessYamlFunctions=false internally.
	_, _, err = exec.GetOutputWithOptions(
		atmosConfig,
		"dev",    // stack
		"my-app", // component
		"rds_properties",
		true, // skipCache
		nil,  // authContext
		nil,  // authManager
		&OutputOptions{SkipInit: true},
	)
	require.NoError(t, err)

	// Assert the file on disk is byte-identical.
	after, err := os.ReadFile(backendPath)
	require.NoError(t, err)
	assert.Equal(t, string(before), string(after),
		"backend.tf.json was rewritten during SkipInit path — this is the issue #2356 regression")
	// Also assert raw bytes match (defense in depth against any encoding drift).
	assert.Equal(t, before, after, "backend.tf.json bytes changed during SkipInit path")
}

// regressionSectionsWithLiteralYamlFunctions returns a sections map whose
// backend config contains literal "!terraform.state ..." strings. This
// simulates DescribeComponent output when ProcessYamlFunctions=false. Keys
// match what ExtractComponentConfig reads (see pkg/terraform/output/config.go):
//   - cfg.CommandSectionName, cfg.WorkspaceSectionName, cfg.ComponentSectionName
//     — required fields.
//   - "component_info" — required, component_type drives path resolution.
//   - cfg.BackendTypeSectionName, cfg.BackendSectionName — optional but
//     required for backend auto-generation.
func regressionSectionsWithLiteralYamlFunctions() map[string]any {
	return map[string]any{
		cfg.CommandSectionName:   "/usr/local/bin/terraform",
		cfg.WorkspaceSectionName: "dev",
		cfg.ComponentSectionName: "my-app",
		"component_info": map[string]any{
			"component_type": "terraform",
		},
		cfg.BackendTypeSectionName: "s3",
		// The literal "!terraform.state ..." strings below are the smoking gun:
		// when ProcessYamlFunctions=false upstream, DescribeComponent returns
		// them verbatim. Without the guard in execute(), these strings flow
		// into GenerateBackendIfNeeded and get serialized into backend.tf.json,
		// corrupting the already-correct file on disk.
		cfg.BackendSectionName: map[string]any{
			"bucket":         "!terraform.state tfstate-backend dev s3_bucket_id",
			"dynamodb_table": "!terraform.state tfstate-backend dev dynamodb_table_name",
			"key":            "terraform.tfstate",
			"region":         "us-east-1",
		},
	}
}
