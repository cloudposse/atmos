package vendor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/vendoring"
	"github.com/cloudposse/atmos/pkg/vendoring/updater"
	"github.com/cloudposse/atmos/pkg/vendoring/version"
)

type componentUpdaterFailingContext struct {
	iolib.Context
	err error
}

func (c componentUpdaterFailingContext) Write(iolib.Stream, string) error {
	return c.err
}

type componentUpdaterLister struct {
	tags  []string
	err   error
	calls int
}

func (l *componentUpdaterLister) ListTags(context.Context, string) ([]string, error) {
	l.calls++
	if l.err != nil {
		return nil, l.err
	}
	return l.tags, nil
}

func TestNormalizeComponentSelectors(t *testing.T) {
	assert.Nil(t, normalizeComponentSelectors(nil))
	assert.Equal(t, []string{"vpc", "eks"}, normalizeComponentSelectors([]string{" vpc ", "[]", "", "eks"}))
}

func TestRunVendorUpdateDoesNotWidenEmptyGroupSelection(t *testing.T) {
	report, err := runVendorUpdate(&vendorUpdateParams{viper: viper.New(), componentType: "terraform", components: []string{}, group: "platform", check: false})
	require.NoError(t, err)
	assert.Empty(t, report.Results)
}

func TestRunVendorUpdateUpdatesSelectedAndGroupedComponents(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), "vendor.yaml")
	require.NoError(t, os.WriteFile(manifest, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-vpc
      version: 1.0.0
      targets: [components/terraform/vpc]
`), 0o644))

	lister := &componentUpdaterLister{tags: []string{"1.0.0", "1.1.0"}}
	previous := version.DefaultLister
	version.DefaultLister = lister
	t.Cleanup(func() { version.DefaultLister = previous })

	v := viper.New()
	v.Set("file", manifest)
	report, err := runVendorUpdate(&vendorUpdateParams{viper: v, componentType: "terraform", components: []string{"vpc"}, check: true})
	require.NoError(t, err)
	require.Len(t, report.Results, 1)
	assert.Equal(t, vendoring.StatusUpdated, report.Results[0].Status)

	v.Set("vendor.update.groups.platform.include", []string{"vpc"})
	report, err = runVendorUpdate(&vendorUpdateParams{viper: v, componentType: "terraform", group: "platform", check: false})
	require.NoError(t, err)
	assert.Equal(t, 1, report.UpdatedCount())
	assert.Equal(t, 3, lister.calls, "group discovery occurs once and selected mutation runs once")
}

// TestRunVendorUpdateGroupDiscoveryError proves a discovery-phase failure inside the
// group-scoped branch (no manifest resolvable at all) is returned as-is rather than being
// swallowed or panicking.
func TestRunVendorUpdateGroupDiscoveryError(t *testing.T) {
	chdirTest(t, t.TempDir()) // no vendor.yaml or component.yaml anywhere.

	v := viper.New()
	v.Set("file", filepath.Join(t.TempDir(), "missing-vendor.yaml"))
	v.Set("vendor.update.groups.platform.include", []string{"vpc"})

	_, err := runVendorUpdate(&vendorUpdateParams{viper: v, componentType: "terraform", group: "platform", check: false})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read vendor manifest")
}

// TestRunVendorUpdateComponentResolvedError proves a per-component UpdateResolved failure (here,
// the remote tag lister erroring) during the explicit --component selector path is returned as-is.
func TestRunVendorUpdateComponentResolvedError(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), "vendor.yaml")
	require.NoError(t, os.WriteFile(manifest, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-vpc
      version: 1.0.0
      targets: [components/terraform/vpc]
`), 0o644))

	lister := &componentUpdaterLister{err: errors.New("remote unavailable")}
	previous := version.DefaultLister
	version.DefaultLister = lister
	t.Cleanup(func() { version.DefaultLister = previous })

	v := viper.New()
	v.Set("file", manifest)
	_, err := runVendorUpdate(&vendorUpdateParams{viper: v, componentType: "terraform", components: []string{"vpc"}, check: true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "remote unavailable")
}

func TestRunVendorUpdateGroupCheckIsDryRun(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), "vendor.yaml")
	require.NoError(t, os.WriteFile(manifest, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-vpc
      version: 1.0.0
      targets: [components/terraform/vpc]
`), 0o644))

	lister := &componentUpdaterLister{tags: []string{"1.0.0", "1.1.0"}}
	previous := version.DefaultLister
	version.DefaultLister = lister
	t.Cleanup(func() { version.DefaultLister = previous })

	v := viper.New()
	v.Set("file", manifest)
	v.Set("vendor.update.groups.platform.include", []string{"vpc"})
	report, err := runVendorUpdate(&vendorUpdateParams{viper: v, componentType: "terraform", group: "platform", check: true})
	require.NoError(t, err)
	require.Len(t, report.Results, 1)
	assert.Equal(t, "vpc", report.Results[0].Component)
	assert.Equal(t, vendoring.StatusUpdated, report.Results[0].Status)
	assert.Equal(t, 1, lister.calls, "a --group --check dry run must stop after discovery and never mutate")
}

func TestRunVendorUpdateReturnsNoOpForGroupWithoutUpdates(t *testing.T) {
	manifest := filepath.Join(t.TempDir(), "vendor.yaml")
	require.NoError(t, os.WriteFile(manifest, []byte(`apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: vpc
      source: github.com/cloudposse/terraform-aws-vpc
      version: 1.1.0
      targets: [components/terraform/vpc]
`), 0o644))
	lister := &componentUpdaterLister{tags: []string{"1.1.0"}}
	previous := version.DefaultLister
	version.DefaultLister = lister
	t.Cleanup(func() { version.DefaultLister = previous })

	v := viper.New()
	v.Set("file", manifest)
	v.Set("vendor.update.groups.platform.include", []string{"vpc"})
	report, err := runVendorUpdate(&vendorUpdateParams{viper: v, componentType: "terraform", group: "platform", check: false})
	require.NoError(t, err)
	assert.Empty(t, report.Results)
	assert.Equal(t, 1, lister.calls, "an empty group must stop after discovery")
}

// TestValidateUpdateInvocationExtractsViperValues proves validateUpdateInvocation correctly wires
// cmd/vendor's viper/cobra state into updater.ValidationConfig/updater.Invocation -- the actual
// validation branch coverage lives in pkg/vendoring/updater's own TestValidateInvocation, so this
// only needs to prove the extraction, not re-cover every validation rule.
func TestValidateUpdateInvocationExtractsViperValues(t *testing.T) {
	newCommand := func() *cobra.Command {
		cmd := &cobra.Command{}
		cmd.Flags().Bool("check", false, "")
		return cmd
	}

	t.Run("propagates a validation failure from an extracted value", func(t *testing.T) {
		v, cmd := viper.New(), newCommand()
		v.Set("format", "yaml")
		err := validateUpdateInvocation(v, cmd, updater.Invocation{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--format")
	})

	t.Run("group existence check uses vendor.update.groups.<group>", func(t *testing.T) {
		v, cmd := viper.New(), newCommand()
		v.Set("format", "table")
		v.Set("vendor.update.groups.platform.include", []string{"vpc"})
		require.NoError(t, validateUpdateInvocation(v, cmd, updater.Invocation{Group: "platform"}))
	})

	t.Run("pull request templates are extracted and validated", func(t *testing.T) {
		v, cmd := viper.New(), newCommand()
		v.Set("format", "table")
		v.Set("vendor.ci.pull_request.title", "{{")
		err := validateUpdateInvocation(v, cmd, updater.Invocation{PullRequest: true})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid pull request template")
	})
}

// TestValidateUpdateInvocationPullRequestCheckIsDryRun proves validateUpdateInvocation's
// checkExplicitlyRequested extraction (cmd.Flags().Changed("check") && v.GetBool("check")) is
// wired correctly: an invalid PR template must not surface on a --pull-request --check dry run,
// since that path never renders (or needs to validate) PR templates.
func TestValidateUpdateInvocationPullRequestCheckIsDryRun(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("check", false, "")
	require.NoError(t, cmd.Flags().Set("check", "true"))

	v := viper.New()
	v.Set("format", "table")
	v.Set("check", true)
	// An invalid template must NOT surface here: a dry-run PR request never
	// renders (or validates) PR templates, since it will never publish.
	v.Set("vendor.ci.pull_request.title", "{{")

	assert.NoError(t, validateUpdateInvocation(v, cmd, updater.Invocation{PullRequest: true}))
}

func TestApplyComponentUpdaterReport(t *testing.T) {
	result := updater.Result{Status: "updated"}
	applyComponentUpdaterReport(&result, &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", Status: vendoring.StatusUpToDate}}})
	assert.Equal(t, "no_updates", result.Status)
	assert.Zero(t, result.Updated)

	applyComponentUpdaterReport(&result, &vendoring.UpdateReport{Results: []vendoring.SourceUpdateResult{{Component: "vpc", Status: vendoring.StatusUpdated}}})
	assert.Equal(t, "updated", result.Status)
	assert.Equal(t, 1, result.Updated)
}

func TestVendorUpdateCommandReportsNoUpdatesAsJSON(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	resetCommandFlags(t, vendorUpdateCmd)
	file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: mock
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: [components/terraform/mock]
`)
	stdout := captureVendorStdout(t)
	require.NoError(t, vendorUpdateCmd.Flags().Set("file", file))
	require.NoError(t, vendorUpdateCmd.Flags().Set("check", "true"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("format", "json"))

	require.NoError(t, vendorUpdateCmd.RunE(vendorUpdateCmd, nil))
	assert.Contains(t, stdout.String(), `"status": "no_updates"`)
}

func TestVendorUpdatePullRequestNoUpdatesDoesNotPublish(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	resetCommandFlags(t, vendorUpdateCmd)
	file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: mock
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: [components/terraform/mock]
`)
	stdout := captureVendorStdout(t)
	require.NoError(t, vendorUpdateCmd.Flags().Set("file", file))
	require.NoError(t, vendorUpdateCmd.Flags().Set("pull-request", "true"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("format", "json"))

	require.NoError(t, vendorUpdateCmd.RunE(vendorUpdateCmd, nil))
	assert.Contains(t, stdout.String(), `"status": "no_updates"`)
	assert.NotContains(t, stdout.String(), `"branch":`)
}

// TestVendorUpdatePullRequestDiscoveryError proves a discovery-phase failure (the initial dry
// run runVendorUpdate call --pull-request makes before ever touching git) is returned as-is,
// without attempting to prepare a branch or publish anything.
func TestVendorUpdatePullRequestDiscoveryError(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	resetCommandFlags(t, vendorUpdateCmd)
	chdirTest(t, t.TempDir()) // no vendor.yaml or component.yaml anywhere.

	_ = captureVendorStdout(t)
	require.NoError(t, vendorUpdateCmd.Flags().Set("file", filepath.Join(t.TempDir(), "missing-vendor.yaml")))
	require.NoError(t, vendorUpdateCmd.Flags().Set("pull-request", "true"))

	err := vendorUpdateCmd.RunE(vendorUpdateCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read vendor manifest")
}

// NOTE on the --pull-request "found updates" success path (vendorUpdateCmd's RunE, update.go
// branch preparation and publish steps): --pull-request unconditionally forces v.Set("pull",
// true), so exercising that branch through vendorUpdateCmd.RunE end-to-end also drives the
// auto-pull's real ExecuteVendorPullCmd materialization step for the "updated" component. Doing
// that without a real network-hosted Git source would require standing up a second,
// separately-tagged local bare Git repo to serve as the vendored *component* source (on top of
// the "origin" repo already used for the PR branch/commit/push flow), purely to satisfy this one
// branch - a disproportionate amount of integration scaffolding for coverage that's already
// exercised at the unit level: updater.PrepareBranch, updater.CommitAndPushComponentUpdate, and
// updater.PublishComponentUpdate (the actual logic these RunE lines call into) are each already
// covered directly in pkg/vendoring/updater's branch_test.go/publish_test.go (100% coverage per
// `go tool cover -func`). Skipped here rather than forcing a flaky or disproportionately heavy
// test; see TestVendorUpdatePullRequestDiscoveryError and
// TestVendorUpdatePullRequestNoUpdatesDoesNotPublish above for the branches of this same RunE
// block that are covered.

// TestVendorUpdateCommand_JSONRenderErrorPropagates proves a data-writer failure while rendering
// the final JSON summary surfaces as a command error instead of being silently swallowed.
func TestVendorUpdateCommand_JSONRenderErrorPropagates(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	resetCommandFlags(t, vendorUpdateCmd)
	file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: mock
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: [components/terraform/mock]
`)
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(componentUpdaterFailingContext{Context: ioCtx, err: errors.New("broken pipe")})
	t.Cleanup(data.Reset)

	require.NoError(t, vendorUpdateCmd.Flags().Set("file", file))
	require.NoError(t, vendorUpdateCmd.Flags().Set("check", "true"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("format", "json"))

	err = vendorUpdateCmd.RunE(vendorUpdateCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken pipe")
}

// TestVendorUpdatePullRequestNoUpdatesJSONRenderErrorPropagates proves a data-writer failure
// while rendering the discovery-phase "no updates" JSON summary (the --pull-request early-return
// branch, distinct from the final renderComponentUpdaterJSON call covered by
// TestVendorUpdateCommand_JSONRenderErrorPropagates above) also surfaces as a command error
// instead of being silently swallowed.
func TestVendorUpdatePullRequestNoUpdatesJSONRenderErrorPropagates(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	resetCommandFlags(t, vendorUpdateCmd)
	file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: mock
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: [components/terraform/mock]
`)
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(componentUpdaterFailingContext{Context: ioCtx, err: errors.New("broken pipe")})
	t.Cleanup(data.Reset)

	require.NoError(t, vendorUpdateCmd.Flags().Set("file", file))
	require.NoError(t, vendorUpdateCmd.Flags().Set("pull-request", "true"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("format", "json"))

	err = vendorUpdateCmd.RunE(vendorUpdateCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "broken pipe")
}

func TestVendorUpdateCommandSurfacesValidationError(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	resetCommandFlags(t, vendorUpdateCmd)
	file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: mock
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: [components/terraform/mock]
`)
	_ = captureVendorStdout(t)
	require.NoError(t, vendorUpdateCmd.Flags().Set("file", file))
	require.NoError(t, vendorUpdateCmd.Flags().Set("all", "true"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("group", "platform"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("format", "json"))

	err := vendorUpdateCmd.RunE(vendorUpdateCmd, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--all cannot")
}

func TestVendorUpdateSummaryRespectsEnabledFlag(t *testing.T) {
	tests := []struct {
		name        string
		enabled     bool
		wantWritten bool
	}{
		{name: "enabled by default", enabled: true, wantWritten: true},
		{name: "disabled", enabled: false, wantWritten: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			t.Cleanup(viper.Reset)
			resetCommandFlags(t, vendorUpdateCmd)
			file := writeCommandVendorManifest(t, `apiVersion: atmos/v1
kind: AtmosVendorConfig
spec:
  sources:
    - component: mock
      source: oci://ghcr.io/cloudposse/mock:{{.Version}}
      version: v0.1.0
      targets: [components/terraform/mock]
`)
			_ = captureVendorStdout(t)
			require.NoError(t, vendorUpdateCmd.Flags().Set("file", file))
			require.NoError(t, vendorUpdateCmd.Flags().Set("check", "true"))
			require.NoError(t, vendorUpdateCmd.Flags().Set("format", "json"))

			summaryPath := filepath.Join(t.TempDir(), "summary.md")
			t.Setenv("GITHUB_ACTIONS", "true")
			t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)
			viper.GetViper().Set("vendor.ci.summary.enabled", tt.enabled)

			require.NoError(t, vendorUpdateCmd.RunE(vendorUpdateCmd, nil))

			content, readErr := os.ReadFile(summaryPath)
			if tt.wantWritten {
				require.NoError(t, readErr)
				assert.NotEmpty(t, content)
				return
			}
			if readErr == nil {
				assert.Empty(t, content, "summary must not be written when vendor.ci.summary.enabled=false")
			}
		})
	}
}

func TestRenderComponentUpdaterJSON(t *testing.T) {
	t.Run("writes JSON", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		ioCtx, err := iolib.NewContext(iolib.WithStreams(&testStreams{stdin: &bytes.Buffer{}, stdout: stdout, stderr: &bytes.Buffer{}}))
		require.NoError(t, err)
		data.InitWriter(ioCtx)
		t.Cleanup(data.Reset)

		require.NoError(t, renderComponentUpdaterJSON(&updater.Result{Status: "no_updates"}, "json"))
		assert.Contains(t, stdout.String(), `"status": "no_updates"`)
		assert.NoError(t, renderComponentUpdaterJSON(&updater.Result{}, "table"))
	})

	t.Run("propagates write error", func(t *testing.T) {
		ioCtx, err := iolib.NewContext()
		require.NoError(t, err)
		data.InitWriter(componentUpdaterFailingContext{Context: ioCtx, err: errors.New("broken pipe")})
		t.Cleanup(data.Reset)

		assert.EqualError(t, renderComponentUpdaterJSON(&updater.Result{}, "json"), "broken pipe")
	})
}
