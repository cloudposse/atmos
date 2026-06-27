package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel: fail the build immediately if the DescribeAffected.Sections
// field is renamed or removed.
var _ = schema.DescribeAffected{Sections: nil}

// tfStacksWithComponent wraps a single terraform component section into the nested
// stacks map shape (stack -> components -> terraform -> component) that the affected
// detection consumes.
func tfStacksWithComponent(component map[string]any) map[string]any {
	return map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"vpc": component,
				},
			},
		},
	}
}

// runFindAffectedTF runs affected detection for a single terraform component whose
// local (current ref) and remote (base ref) sections are provided. No files are
// reported as changed, so only stack-section differences can mark the component affected.
func runFindAffectedTF(t *testing.T, atmosConfig *schema.AtmosConfiguration, local, remote map[string]any) []schema.Affected {
	t.Helper()

	if atmosConfig == nil {
		atmosConfig = &schema.AtmosConfiguration{
			Components: schema.Components{
				Terraform: schema.Terraform{BasePath: "components/terraform"},
			},
		}
	}

	current := tfStacksWithComponent(local)
	remoteStacks := tfStacksWithComponent(remote)

	affected, err := findAffected(
		&current,
		&remoteStacks,
		atmosConfig,
		nil,   // changedFiles - none, so only section diffs matter.
		false, // includeSpaceliftAdminStacks.
		false, // includeSettings.
		"",    // stackToFilter.
		false, // excludeLocked.
		"",    // gitRepoRoot.
	)
	require.NoError(t, err)
	return affected
}

// TestFindAffected_EvaluatedSections proves every section in componentSectionChecks is
// compared, including scalar sections, and reports the expected `affected` reason.
func TestFindAffected_EvaluatedSections(t *testing.T) {
	tests := []struct {
		name       string
		section    string
		localVal   any
		remoteVal  any
		wantReason string
	}{
		{"vars", "vars", map[string]any{"a": "1"}, map[string]any{"a": "2"}, affectedReasonStackVars},
		{"env", "env", map[string]any{"K": "1"}, map[string]any{"K": "2"}, affectedReasonStackEnv},
		{"providers", "providers", map[string]any{"aws": map[string]any{"region": "us-east-1"}}, map[string]any{"aws": map[string]any{"region": "us-west-2"}}, affectedReasonStackProviders},
		{"required_providers", "required_providers", map[string]any{"aws": map[string]any{"version": "5.0"}}, map[string]any{"aws": map[string]any{"version": "6.0"}}, affectedReasonStackRequiredProviders},
		{"required_version (scalar)", "required_version", "1.5.0", "1.6.0", affectedReasonStackRequiredVersion},
		{"generate", "generate", map[string]any{"backend": map[string]any{"enabled": true}}, map[string]any{"backend": map[string]any{"enabled": false}}, affectedReasonStackGenerate},
		{"backend", "backend", map[string]any{"bucket": "a"}, map[string]any{"bucket": "b"}, affectedReasonStackBackend},
		{"backend_type (scalar)", "backend_type", "s3", "local", affectedReasonStackBackendType},
		{"remote_state_backend", "remote_state_backend", map[string]any{"role_arn": "a"}, map[string]any{"role_arn": "b"}, affectedReasonStackRemoteStateBackend},
		{"remote_state_backend_type (scalar)", "remote_state_backend_type", "s3", "static", affectedReasonStackRemoteStateBackendType},
		{"auth", "auth", map[string]any{"role": "a"}, map[string]any{"role": "b"}, affectedReasonStackAuth},
		{"command (scalar)", "command", "terraform", "tofu", affectedReasonStackCommand},
		{"dependencies", "dependencies", map[string]any{"components": []any{"a"}}, map[string]any{"components": []any{"b"}}, affectedReasonStackDependencies},
		{"source", "source", map[string]any{"uri": "a"}, map[string]any{"uri": "b"}, affectedReasonStackSource},
		{"provision", "provision", map[string]any{"workdir": "a"}, map[string]any{"workdir": "b"}, affectedReasonStackProvision},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local := map[string]any{tt.section: tt.localVal}
			remote := map[string]any{tt.section: tt.remoteVal}

			affected := runFindAffectedTF(t, nil, local, remote)

			require.Len(t, affected, 1)
			assert.Equal(t, tt.wantReason, affected[0].Affected)
			assert.Contains(t, affected[0].AffectedAll, tt.wantReason)
		})
	}
}

// k8sTestStack and k8sTestComponent are the fixed stack/component names used by the
// kubernetes affected-detection unit tests.
const (
	k8sTestStack     = "dev"
	k8sTestComponent = "app"
)

// k8sRemoteStacksWith builds the nested remote-stacks map shape
// (stack -> components -> kubernetes -> component) consumed by the remote section
// locator used in addKubernetesSectionAffected. It always uses k8sTestStack/k8sTestComponent.
func k8sRemoteStacksWith(remoteComp map[string]any) map[string]any {
	return map[string]any{
		k8sTestStack: map[string]any{
			"components": map[string]any{
				cfg.KubernetesComponentType: map[string]any{
					k8sTestComponent: remoteComp,
				},
			},
		},
	}
}

// k8sAtmosConfig returns a minimal kubernetes-aware atmos configuration.
func k8sAtmosConfig() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		Components: schema.Components{
			Kubernetes: schema.Kubernetes{BasePath: "components/kubernetes"},
		},
	}
}

// TestAddKubernetesSectionAffected proves every kubernetes-specific section (including the
// native paths/manifests/render sections) is compared and reports the expected `affected`
// reason. One section differs per case; the local component carries only that section.
//
// The addKubernetesSectionAffected helper is also exercised end-to-end through the full
// processKubernetesComponentsIndexed pipeline by TestProcessKubernetesComponentsIndexed.
func TestAddKubernetesSectionAffected(t *testing.T) {
	const (
		stackName     = k8sTestStack
		componentName = k8sTestComponent
	)

	tests := []struct {
		name       string
		section    string
		localVal   any
		remoteVal  any
		wantReason string
	}{
		{"vars", sectionNameVars, map[string]any{"a": "1"}, map[string]any{"a": "2"}, affectedReasonStackVars},
		{"env", sectionNameEnv, map[string]any{"K": "1"}, map[string]any{"K": "2"}, affectedReasonStackEnv},
		{"source", sectionNameSource, map[string]any{"uri": "a"}, map[string]any{"uri": "b"}, affectedReasonStackSource},
		{"provision", sectionNameProvision, map[string]any{"workdir": "a"}, map[string]any{"workdir": "b"}, affectedReasonStackProvision},
		{"generate", sectionNameGenerate, map[string]any{"backend": map[string]any{"enabled": true}}, map[string]any{"backend": map[string]any{"enabled": false}}, affectedReasonStackGenerate},
		{"provider", cfg.ProviderSectionName, "kubectl", "kustomize", "stack.provider"},
		{"paths", sectionNamePaths, []any{"a/b.yaml"}, []any{"c/d.yaml"}, affectedReasonStackPaths},
		{"manifests", sectionNameManifests, map[string]any{"deployment": "a.yaml"}, map[string]any{"deployment": "b.yaml"}, affectedReasonStackManifests},
		{"render", sectionNameRender, map[string]any{"engine": "kustomize"}, map[string]any{"engine": "kubectl"}, affectedReasonStackRender},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			componentSection := map[string]any{tt.section: tt.localVal}
			remoteStacks := k8sRemoteStacksWith(map[string]any{tt.section: tt.remoteVal})

			var affected []schema.Affected
			err := addKubernetesSectionAffected(
				&affected, k8sAtmosConfig(), componentName, stackName,
				&componentSection, &remoteStacks, &remoteStacks,
				false, false,
			)
			require.NoError(t, err)

			require.Len(t, affected, 1)
			assert.Equal(t, componentName, affected[0].Component)
			assert.Equal(t, cfg.KubernetesComponentType, affected[0].ComponentType)
			assert.Equal(t, tt.wantReason, affected[0].Affected)
			assert.Contains(t, affected[0].AffectedAll, tt.wantReason)
		})
	}
}

func TestAddKubernetesSectionAffected_SectionsOverrideAddsCustomSection(t *testing.T) {
	componentSection := map[string]any{"hooks": map[string]any{"policy": map[string]any{"kind": "checkov"}}}
	remoteStacks := k8sRemoteStacksWith(map[string]any{"hooks": map[string]any{"policy": map[string]any{"kind": "trivy"}}})
	atmosConfig := k8sAtmosConfig()
	atmosConfig.Describe.Affected.Sections = []string{"hooks"}

	var affected []schema.Affected
	err := addKubernetesSectionAffected(
		&affected, atmosConfig, k8sTestComponent, k8sTestStack,
		&componentSection, &remoteStacks, &remoteStacks,
		false, false,
	)
	require.NoError(t, err)

	require.Len(t, affected, 1)
	assert.Equal(t, "stack.hooks", affected[0].Affected)
	assert.Contains(t, affected[0].AffectedAll, "stack.hooks")
}

// TestAddKubernetesSectionAffected_NoFalsePositives proves identical kubernetes sections do
// not mark the component affected.
func TestAddKubernetesSectionAffected_NoFalsePositives(t *testing.T) {
	const (
		stackName     = "dev"
		componentName = "app"
	)

	cases := []struct {
		name    string
		section string
		val     any
	}{
		{"identical manifests", sectionNameManifests, map[string]any{"deployment": "a.yaml"}},
		{"identical paths slice", sectionNamePaths, []any{"a/b.yaml"}},
		{"identical render", sectionNameRender, map[string]any{"engine": "kustomize"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			componentSection := map[string]any{tc.section: tc.val}
			remoteStacks := k8sRemoteStacksWith(map[string]any{tc.section: tc.val})

			var affected []schema.Affected
			err := addKubernetesSectionAffected(
				&affected, k8sAtmosConfig(), componentName, stackName,
				&componentSection, &remoteStacks, &remoteStacks,
				false, false,
			)
			require.NoError(t, err)
			assert.Empty(t, affected)
		})
	}

	t.Run("section absent locally is skipped", func(t *testing.T) {
		componentSection := map[string]any{}
		remoteStacks := k8sRemoteStacksWith(map[string]any{sectionNameManifests: map[string]any{"deployment": "a.yaml"}})

		var affected []schema.Affected
		err := addKubernetesSectionAffected(
			&affected, k8sAtmosConfig(), componentName, stackName,
			&componentSection, &remoteStacks, &remoteStacks,
			false, false,
		)
		require.NoError(t, err)
		assert.Empty(t, affected)
	})
}

// TestProcessKubernetesComponentsIndexed exercises the full kubernetes affected-detection
// pipeline: the component loop, the metadata-change branch, the indexed component-folder
// check (which resolves the kubernetes base path), and the kubernetes-specific section
// comparison. The local component differs from remote in both metadata.component and vars,
// so it is reported affected with both reasons accumulated into AffectedAll.
func TestProcessKubernetesComponentsIndexed(t *testing.T) {
	const (
		stackName     = k8sTestStack
		componentName = k8sTestComponent
	)

	atmosConfig := k8sAtmosConfig()
	kubernetesSection := map[string]any{
		componentName: map[string]any{
			sectionNameMetadata:     map[string]any{"component": "app-v1"},
			sectionNameVars:         map[string]any{"a": "1"},
			cfg.SettingsSectionName: map[string]any{"s": "1"},
		},
	}
	remoteStacks := k8sRemoteStacksWith(map[string]any{
		sectionNameMetadata:     map[string]any{"component": "app-v2"},
		sectionNameVars:         map[string]any{"a": "2"},
		cfg.SettingsSectionName: map[string]any{"s": "2"},
	})

	filesIndex := newChangedFilesIndex(atmosConfig, nil, "")
	patternCache := newComponentPathPatternCache()

	affected, err := processKubernetesComponentsIndexed(
		stackName, kubernetesSection, &remoteStacks, &remoteStacks,
		atmosConfig, filesIndex, patternCache,
		false, true, false,
	)
	require.NoError(t, err)

	require.Len(t, affected, 1)
	assert.Equal(t, componentName, affected[0].Component)
	assert.Equal(t, cfg.KubernetesComponentType, affected[0].ComponentType)
	assert.Contains(t, affected[0].AffectedAll, affectedReasonStackMetadata)
	assert.Contains(t, affected[0].AffectedAll, affectedReasonStackVars)
	assert.Contains(t, affected[0].AffectedAll, affectedReasonStackSettings)
}

// TestProcessKubernetesComponentsIndexed_NotAffected proves an identical component (same
// metadata and vars, no changed files) produces no affected results.
func TestProcessKubernetesComponentsIndexed_NotAffected(t *testing.T) {
	const (
		stackName     = k8sTestStack
		componentName = k8sTestComponent
	)

	atmosConfig := k8sAtmosConfig()
	identical := map[string]any{
		sectionNameMetadata: map[string]any{"component": "app-v1"},
		sectionNameVars:     map[string]any{"a": "1"},
	}
	kubernetesSection := map[string]any{componentName: identical}
	remoteStacks := k8sRemoteStacksWith(map[string]any{
		sectionNameMetadata: map[string]any{"component": "app-v1"},
		sectionNameVars:     map[string]any{"a": "1"},
	})

	filesIndex := newChangedFilesIndex(atmosConfig, nil, "")
	patternCache := newComponentPathPatternCache()

	affected, err := processKubernetesComponentsIndexed(
		stackName, kubernetesSection, &remoteStacks, &remoteStacks,
		atmosConfig, filesIndex, patternCache,
		false, false, false,
	)
	require.NoError(t, err)
	assert.Empty(t, affected)
}

// TestProcessKubernetesComponentsIndexed_FolderChanged proves a changed file inside the
// component's kubernetes folder marks the component affected with the component reason,
// even when its metadata and vars are unchanged. This exercises the indexed
// component-folder change branch (which depends on the kubernetes base-path resolution).
func TestProcessKubernetesComponentsIndexed_FolderChanged(t *testing.T) {
	const (
		stackName     = k8sTestStack
		componentName = k8sTestComponent
	)

	atmosConfig := k8sAtmosConfig()
	identical := map[string]any{
		sectionNameVars: map[string]any{"a": "1"},
	}
	kubernetesSection := map[string]any{componentName: identical}
	remoteStacks := k8sRemoteStacksWith(map[string]any{
		sectionNameVars: map[string]any{"a": "1"},
	})

	// A changed file under components/kubernetes/<component>. The component folder falls
	// back to the component name because the section has no explicit "component" field.
	changedFile, err := filepath.Abs(filepath.Join("components", "kubernetes", componentName, "deployment.yaml"))
	require.NoError(t, err)

	filesIndex := newChangedFilesIndex(atmosConfig, []string{changedFile}, "")
	patternCache := newComponentPathPatternCache()

	affected, err := processKubernetesComponentsIndexed(
		stackName, kubernetesSection, &remoteStacks, &remoteStacks,
		atmosConfig, filesIndex, patternCache,
		false, false, false,
	)
	require.NoError(t, err)

	require.Len(t, affected, 1)
	assert.Equal(t, componentName, affected[0].Component)
	assert.Equal(t, cfg.KubernetesComponentType, affected[0].ComponentType)
	assert.Contains(t, affected[0].AffectedAll, affectedReasonComponent)
}

// TestProcessKubernetesComponentsIndexed_SkipsAbstract proves an abstract component is
// skipped entirely (the shouldSkipComponent continue branch), even when it differs from
// remote.
func TestProcessKubernetesComponentsIndexed_SkipsAbstract(t *testing.T) {
	const (
		stackName     = k8sTestStack
		componentName = k8sTestComponent
	)

	atmosConfig := k8sAtmosConfig()
	kubernetesSection := map[string]any{
		componentName: map[string]any{
			sectionNameMetadata: map[string]any{"type": "abstract"},
			sectionNameVars:     map[string]any{"a": "1"},
		},
	}
	remoteStacks := k8sRemoteStacksWith(map[string]any{
		sectionNameVars: map[string]any{"a": "2"},
	})

	filesIndex := newChangedFilesIndex(atmosConfig, nil, "")
	patternCache := newComponentPathPatternCache()

	affected, err := processKubernetesComponentsIndexed(
		stackName, kubernetesSection, &remoteStacks, &remoteStacks,
		atmosConfig, filesIndex, patternCache,
		false, false, false,
	)
	require.NoError(t, err)
	assert.Empty(t, affected)
}

// TestFindAffected_NoFalsePositives guards against spurious affected results.
func TestFindAffected_NoFalsePositives(t *testing.T) {
	t.Run("identical section is not affected", func(t *testing.T) {
		local := map[string]any{"providers": map[string]any{"aws": map[string]any{"region": "us-east-1"}}}
		remote := map[string]any{"providers": map[string]any{"aws": map[string]any{"region": "us-east-1"}}}

		affected := runFindAffectedTF(t, nil, local, remote)
		assert.Empty(t, affected)
	})

	t.Run("identical scalar section is not affected", func(t *testing.T) {
		local := map[string]any{"backend_type": "s3"}
		remote := map[string]any{"backend_type": "s3"}

		affected := runFindAffectedTF(t, nil, local, remote)
		assert.Empty(t, affected)
	})

	t.Run("empty section on both sides is not affected", func(t *testing.T) {
		local := map[string]any{"providers": map[string]any{}}
		remote := map[string]any{"providers": map[string]any{}}

		affected := runFindAffectedTF(t, nil, local, remote)
		assert.Empty(t, affected)
	})

	t.Run("section absent on both sides is not affected", func(t *testing.T) {
		local := map[string]any{}
		remote := map[string]any{}

		affected := runFindAffectedTF(t, nil, local, remote)
		assert.Empty(t, affected)
	})

	t.Run("unknown custom section is not evaluated by default", func(t *testing.T) {
		local := map[string]any{"my_custom_section": map[string]any{"x": "1"}}
		remote := map[string]any{"my_custom_section": map[string]any{"x": "2"}}

		affected := runFindAffectedTF(t, nil, local, remote)
		assert.Empty(t, affected)
	})

	t.Run("hooks section is not evaluated by default", func(t *testing.T) {
		// `hooks` is operational/execution-time behavior, not provisioned infrastructure,
		// so a change to it must not mark a component as affected by default.
		local := map[string]any{"hooks": map[string]any{"policy": map[string]any{"kind": "checkov"}}}
		remote := map[string]any{"hooks": map[string]any{"policy": map[string]any{"kind": "trivy"}}}

		affected := runFindAffectedTF(t, nil, local, remote)
		assert.Empty(t, affected)
	})
}

// TestFindAffected_SectionsOverride proves describe.affected.sections fully replaces the
// built-in defaults: only listed sections are evaluated, custom sections are honored, and
// default sections that are not listed are ignored.
func TestFindAffected_SectionsOverride(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{BasePath: "components/terraform"},
		},
		Describe: schema.Describe{
			Affected: schema.DescribeAffected{
				Sections: []string{"providers", "hooks", "my_custom_section"},
			},
		},
	}

	t.Run("listed default section is evaluated", func(t *testing.T) {
		local := map[string]any{"providers": map[string]any{"aws": map[string]any{"region": "us-east-1"}}}
		remote := map[string]any{"providers": map[string]any{"aws": map[string]any{"region": "us-west-2"}}}

		affected := runFindAffectedTF(t, atmosConfig, local, remote)
		require.Len(t, affected, 1)
		assert.Equal(t, affectedReasonStackProviders, affected[0].Affected)
	})

	t.Run("hooks is evaluated when explicitly listed", func(t *testing.T) {
		// `hooks` is not a default, but opting in via the list evaluates it and reports
		// the `stack.hooks` reason (via the generic `stack.<name>` fallback).
		local := map[string]any{"hooks": map[string]any{"policy": map[string]any{"kind": "checkov"}}}
		remote := map[string]any{"hooks": map[string]any{"policy": map[string]any{"kind": "trivy"}}}

		affected := runFindAffectedTF(t, atmosConfig, local, remote)
		require.Len(t, affected, 1)
		assert.Equal(t, "stack.hooks", affected[0].Affected)
	})

	t.Run("listed custom section is evaluated with generic reason", func(t *testing.T) {
		local := map[string]any{"my_custom_section": map[string]any{"x": "1"}}
		remote := map[string]any{"my_custom_section": map[string]any{"x": "2"}}

		affected := runFindAffectedTF(t, atmosConfig, local, remote)
		require.Len(t, affected, 1)
		assert.Equal(t, "stack.my_custom_section", affected[0].Affected)
	})

	t.Run("default section not listed is ignored", func(t *testing.T) {
		// `vars` is a built-in default but is not in the override list.
		local := map[string]any{"vars": map[string]any{"a": "1"}}
		remote := map[string]any{"vars": map[string]any{"a": "2"}}

		affected := runFindAffectedTF(t, atmosConfig, local, remote)
		assert.Empty(t, affected)
	})

	t.Run("custom section is ignored without the override", func(t *testing.T) {
		// Same custom-section diff, but with default config (no override) -> not affected.
		local := map[string]any{"my_custom_section": map[string]any{"x": "1"}}
		remote := map[string]any{"my_custom_section": map[string]any{"x": "2"}}

		affected := runFindAffectedTF(t, nil, local, remote)
		assert.Empty(t, affected)
	})
}
