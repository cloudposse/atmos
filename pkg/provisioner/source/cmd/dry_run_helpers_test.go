package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/provisioner/source"
	"github.com/cloudposse/atmos/pkg/schema"
)

// dryRunEnv wires the package-level seams of the source commands to an in-memory stack and a
// temporary components directory, and records every side effect the commands could perform.
type dryRunEnv struct {
	root      string
	targetDir string
	// provisions counts source.Provision invocations, authentications counts auth manager creation.
	provisions      int
	authentications int
}

// dryRunScenario describes the component under test.
type dryRunScenario struct {
	// section is the described component configuration; nil describes a nonexistent component.
	section map[string]any
	// stackComponents is the `components.aws/cloudformation` map returned for the stack.
	stackComponents map[string]any
	// provisioned writes the provenance marker into the target directory.
	provisioned bool
	// existing creates the target directory.
	existing bool
}

// newDryRunEnv installs the seams for one scenario. The target directory is
// components/cloudformation/vpc inside a fresh temporary base path.
func newDryRunEnv(t *testing.T, sc dryRunScenario) *dryRunEnv {
	t.Helper()
	viper.Reset()
	viper.Set("interactive", false)
	t.Cleanup(viper.Reset)

	env := &dryRunEnv{root: t.TempDir()}
	env.targetDir = filepath.Join(env.root, "components", "cloudformation", "vpc")
	if sc.existing {
		require.NoError(t, os.MkdirAll(env.targetDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(env.targetDir, "template.yaml"), []byte("Resources: {}\n"), 0o600))
		if sc.provisioned {
			require.NoError(t, source.WriteProvenance(env.targetDir, &source.Provenance{Component: "vpc", Source: "https://example.invalid/x"}))
		}
	}

	config := schema.AtmosConfiguration{BasePath: env.root}
	config.Components.CloudFormation.BasePath = "components/cloudformation"

	oldInit, oldDescribe, oldMerge, oldCreate, oldProvision := initCliConfigFunc, describeComponentFunc, mergeAuthFunc, createAuthFunc, provisionSourceFunc
	t.Cleanup(func() {
		initCliConfigFunc, describeComponentFunc, mergeAuthFunc, createAuthFunc, provisionSourceFunc = oldInit, oldDescribe, oldMerge, oldCreate, oldProvision
	})
	initCliConfigFunc = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) { return config, nil }
	describeComponentFunc = func(string, string) (map[string]any, error) {
		if sc.section == nil {
			return nil, os.ErrNotExist
		}
		return sc.section, nil
	}
	mergeAuthFunc = func(*schema.AuthConfig, map[string]any, *schema.AtmosConfiguration, string) (*schema.AuthConfig, error) {
		return &schema.AuthConfig{}, nil
	}
	createAuthFunc = func(string, *schema.AuthConfig, string) (auth.AuthManager, error) {
		env.authentications++
		return nil, nil
	}
	provisionSourceFunc = func(context.Context, *source.ProvisionParams) error {
		env.provisions++
		return nil
	}
	stubDescribeStacks(t, sc.stackComponents)
	return env
}

// vpcSection returns a described component section for a sourced `vpc` instance.
func vpcSection(extra map[string]any) map[string]any {
	section := map[string]any{
		"source":          map[string]any{"uri": "https://example.invalid/component.zip"},
		"atmos_component": "vpc",
		"atmos_stack":     "dev",
	}
	for key, value := range extra {
		section[key] = value
	}
	return section
}
