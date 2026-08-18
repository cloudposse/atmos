package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// collidingSection returns a nested map[any]any with two keys that both normalize to the string
// "1" (the int 1 and the string "1") but whose values aren't both maps — pkg/merge's
// normalizeMapReflect treats this as ambiguous (there is no safe way to combine two non-map
// colliding values) and aborts the merge with errUtils.ErrMergeKeyCollision. Embedding this inside
// an otherwise ordinary map[string]any lets a single, targeted section trip a real structural
// merge failure without disturbing any other section's inputs.
func collidingSection() map[string]any {
	return map[string]any{
		"conflict": map[any]any{1: "a", "1": "b"},
	}
}

// validTerraformMergeOpts returns a fully populated, happy-path
// ComponentProcessorOptions/ComponentProcessorResult pair for a Terraform component — every
// section mergeComponentConfigurations touches has valid, non-colliding, non-empty-where-required
// inputs. Each error-injection test case starts from this baseline and mutates exactly one field
// to introduce a single collision, isolating which merge call the resulting error came from.
func validTerraformMergeOpts(atmosCfg *schema.AtmosConfiguration) (*ComponentProcessorOptions, *ComponentProcessorResult) {
	opts := &ComponentProcessorOptions{
		ComponentType:                   cfg.TerraformComponentType,
		Component:                       "vpc",
		GlobalVars:                      map[string]any{"a": "1"},
		GlobalSettings:                  map[string]any{"b": "2"},
		GlobalEnv:                       map[string]any{"C": "3"},
		GlobalAuth:                      map[string]any{"d": "4"},
		GlobalSecrets:                   map[string]any{"e": "5"},
		GlobalDependencies:              map[string]any{"f": "6"},
		GlobalMetadata:                  map[string]any{"g": "7"},
		AtmosGlobalAuthMap:              map[string]any{"h": "8"},
		TerraformProviders:              map[string]any{"i": "9"},
		TerraformRequiredProviders:      map[string]any{"j": "10"},
		GlobalAndTerraformHooks:         map[string]any{"k": "11"},
		GlobalAndTerraformGenerate:      map[string]any{"l": "12"},
		GlobalBackendType:               "s3",
		GlobalBackendSection:            map[string]any{"s3": map[string]any{"bucket": "test"}},
		GlobalRemoteStateBackendSection: map[string]any{},
		GlobalSourceSection:             map[string]any{"m": "13"},
		GlobalProvisionSection:          map[string]any{"n": "14"},
		AtmosConfig:                     atmosCfg,
	}
	res := minimalComponentResult()
	res.ComponentRequiredProviders = map[string]any{}
	res.ComponentOverridesRequiredProviders = map[string]any{}
	res.BaseComponentRequiredProviders = map[string]any{}
	res.ComponentSecrets = map[string]any{}
	res.ComponentOverridesSecrets = map[string]any{}
	res.BaseComponentSecrets = map[string]any{}
	res.ComponentGenerate = map[string]any{}
	res.ComponentOverridesGenerate = map[string]any{}
	res.BaseComponentGenerate = map[string]any{}
	res.ComponentSourceSection = map[string]any{}
	res.BaseComponentSourceSection = map[string]any{}
	res.ComponentProvision = map[string]any{}
	res.BaseComponentProvisionSection = map[string]any{}
	res.ComponentDependencies = map[string]any{}
	res.BaseComponentDependencies = map[string]any{}
	res.ComponentLocals = map[string]any{}
	res.BaseComponentLocals = map[string]any{}
	res.ComponentRetry = map[string]any{}
	res.BaseComponentRetry = map[string]any{}
	res.ComponentOverridesRetry = map[string]any{}
	return opts, res
}

// TestMergeComponentConfigurations_SectionMergeErrors verifies that a structural-merge failure
// (a genuinely ambiguous, colliding YAML-normalized map) in any single section of a Terraform
// component is surfaced as a real error — never silently dropped or merged into the wrong
// section — by isolating one colliding field at a time against an otherwise valid component.
func TestMergeComponentConfigurations_SectionMergeErrors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(opts *ComponentProcessorOptions, res *ComponentProcessorResult)
		wantErr error
	}{
		{
			name: "settings",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.GlobalSettings = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "env",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.GlobalEnv = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "auth (first pass)",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.GlobalAuth = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "terraform providers",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.TerraformProviders = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "terraform required_providers",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.TerraformRequiredProviders = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "hooks",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.GlobalAndTerraformHooks = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "terraform test section",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				res.BaseComponentTest = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "terraform mocks",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				res.BaseComponentMocks = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "secrets declarations",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.GlobalSecrets = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "generate section",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.GlobalAndTerraformGenerate = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "kubernetes paths (shared helper, not kubernetes-only)",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.GlobalKubernetesPaths = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "kubernetes manifests (shared helper, not kubernetes-only)",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.GlobalKubernetesManifests = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "metadata",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.GlobalMetadata = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "dependencies",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.GlobalDependencies = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "locals",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				res.BaseComponentLocals = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "retry",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				res.BaseComponentRetry = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "backend section",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.GlobalBackendSection = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "auth (second, Terraform-specific pass)",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.AtmosGlobalAuthMap = collidingSection()
			},
			wantErr: errUtils.ErrInvalidAuthConfig,
		},
		{
			name: "source section",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.GlobalSourceSection = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
		{
			name: "provision section",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.GlobalProvisionSection = collidingSection()
			},
			wantErr: errUtils.ErrMergeKeyCollision,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosCfg := &schema.AtmosConfiguration{}
			opts, res := validTerraformMergeOpts(atmosCfg)
			tt.mutate(opts, res)

			comp, deferredContexts, err := mergeComponentConfigurations(atmosCfg, opts, res)

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Nil(t, comp)
			assert.Nil(t, deferredContexts)
		})
	}
}

// TestMergeComponentConfigurations_KubernetesRenderMergeError verifies the Kubernetes-only
// `render:` section (gated behind ComponentType == kubernetes, unlike paths/manifests above)
// surfaces a structural-merge failure the same way.
func TestMergeComponentConfigurations_KubernetesRenderMergeError(t *testing.T) {
	atmosCfg := &schema.AtmosConfiguration{}
	opts := ComponentProcessorOptions{
		ComponentType:           cfg.KubernetesComponentType,
		Component:               "app",
		GlobalVars:              map[string]any{},
		GlobalSettings:          map[string]any{},
		GlobalEnv:               map[string]any{},
		GlobalKubernetesRender:  collidingSection(),
		GlobalAndTerraformHooks: map[string]any{},
		AtmosConfig:             atmosCfg,
	}
	res := minimalComponentResult()

	comp, deferredContexts, err := mergeComponentConfigurations(atmosCfg, &opts, res)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMergeKeyCollision)
	assert.Nil(t, comp)
	assert.Nil(t, deferredContexts)
}

// TestMergeComponentConfigurations_HelmMergeErrors verifies the Helm-only native-fields merge and
// the shared Helm CLI plugins merge (supportsPlugins) surface structural-merge failures.
func TestMergeComponentConfigurations_HelmMergeErrors(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(opts *ComponentProcessorOptions, res *ComponentProcessorResult)
	}{
		{
			name: "helm native fields",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				res.BaseComponentHelm = collidingSection()
			},
		},
		{
			name: "helm CLI plugins",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				res.BaseComponentPlugins = collidingSection()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosCfg := &schema.AtmosConfiguration{}
			opts := ComponentProcessorOptions{
				ComponentType:  cfg.HelmComponentType,
				Component:      "app",
				GlobalVars:     map[string]any{},
				GlobalSettings: map[string]any{},
				GlobalEnv:      map[string]any{},
				AtmosConfig:    atmosCfg,
			}
			res := minimalComponentResult()
			tt.mutate(&opts, res)

			comp, deferredContexts, err := mergeComponentConfigurations(atmosCfg, &opts, res)

			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrMergeKeyCollision)
			assert.Nil(t, comp)
			assert.Nil(t, deferredContexts)
		})
	}
}

// TestMergeComponentConfigurations_SettingsIntegrationsGithubMergeError verifies that a colliding
// `integrations.github` section in atmos.yaml itself (not the stack manifest's `settings:`, which
// is merged earlier and independently) surfaces as a real error from
// processSettingsIntegrationsGithub, rather than being silently dropped.
func TestMergeComponentConfigurations_SettingsIntegrationsGithubMergeError(t *testing.T) {
	atmosCfg := &schema.AtmosConfiguration{}
	atmosCfg.Integrations.GitHub = collidingSection()

	opts := ComponentProcessorOptions{
		ComponentType:  cfg.TerraformComponentType,
		Component:      "vpc",
		GlobalVars:     map[string]any{},
		GlobalSettings: map[string]any{},
		GlobalEnv:      map[string]any{},
		AtmosConfig:    atmosCfg,
	}
	res := minimalComponentResult()

	comp, deferredContexts, err := mergeComponentConfigurations(atmosCfg, &opts, res)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMergeKeyCollision)
	assert.Nil(t, comp)
	assert.Nil(t, deferredContexts)
}

// TestMergeComponentConfigurations_DeferredWriteBackNavigationError verifies that when a
// higher-precedence layer overrides a deferred function's PARENT path with a scalar (not a map),
// the Stage-2 write-back (ApplyDeferredMerges with a nil processor) fails loudly with
// ErrCannotNavigatePath instead of silently discarding the deferred function's placement — this
// is a distinct failure mode from the raw-merge collision cases above: the raw structural merge
// succeeds (the scalar cleanly wins over the nil placeholder), but writing the still-unresolved
// function string back into the merged result cannot navigate through the now-scalar parent.
func TestMergeComponentConfigurations_DeferredWriteBackNavigationError(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(opts *ComponentProcessorOptions, res *ComponentProcessorResult)
		wantErr error
	}{
		{
			name: "vars",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.GlobalVars = map[string]any{"nested": map[string]any{"leaf": "!env FOO"}}
				res.ComponentOverridesVars = map[string]any{"nested": "scalar-override"}
			},
			wantErr: errUtils.ErrCannotNavigatePath,
		},
		{
			name: "settings",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.GlobalSettings = map[string]any{"nested": map[string]any{"leaf": "!env FOO"}}
				res.ComponentOverridesSettings = map[string]any{"nested": "scalar-override"}
			},
			wantErr: errUtils.ErrCannotNavigatePath,
		},
		{
			name: "env",
			mutate: func(opts *ComponentProcessorOptions, res *ComponentProcessorResult) {
				opts.GlobalEnv = map[string]any{"nested": map[string]any{"leaf": "!env FOO"}}
				res.ComponentOverridesEnv = map[string]any{"nested": "scalar-override"}
			},
			wantErr: errUtils.ErrCannotNavigatePath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosCfg := &schema.AtmosConfiguration{}
			opts, res := validTerraformMergeOpts(atmosCfg)
			tt.mutate(opts, res)

			comp, deferredContexts, err := mergeComponentConfigurations(atmosCfg, opts, res)

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.Nil(t, comp)
			assert.Nil(t, deferredContexts)
		})
	}
}
