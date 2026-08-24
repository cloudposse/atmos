package mirror

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestBuildMirrorArgs(t *testing.T) {
	dir := filepath.Join("root", "providers")
	args := buildMirrorArgs([]string{"linux_amd64", "darwin_arm64"}, dir)
	assert.Equal(t, []string{"-platform=linux_amd64", "-platform=darwin_arm64", dir}, args)

	// No platforms: just the target directory.
	assert.Equal(t, []string{dir}, buildMirrorArgs(nil, dir))
}

func TestOptions_HasSelector(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want bool
	}{
		{name: "none", opts: Options{}, want: false},
		{name: "all", opts: Options{All: true}, want: true},
		{name: "components", opts: Options{Components: []string{"vpc"}}, want: true},
		{name: "query", opts: Options{Query: ".vars.enabled == true"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.opts.hasSelector())
		})
	}
}

func TestResolveTargets(t *testing.T) {
	cfg := &schema.AtmosConfiguration{}

	t.Run("single component requires a stack", func(t *testing.T) {
		_, err := resolveTargets(cfg, &Options{Component: "vpc"})
		require.ErrorIs(t, err, errUtils.ErrMissingStack)
	})

	t.Run("single component with stack", func(t *testing.T) {
		targets, err := resolveTargets(cfg, &Options{Component: "vpc", Stack: "plat-ue2-prod"})
		require.NoError(t, err)
		assert.Equal(t, []Target{{Component: "vpc", Stack: "plat-ue2-prod"}}, targets)
	})

	t.Run("no component, no selector, no stack", func(t *testing.T) {
		_, err := resolveTargets(cfg, &Options{})
		require.ErrorIs(t, err, errUtils.ErrMissingComponent)
	})
}

func TestRunMirror_StructuredFormat(t *testing.T) {
	orig := executeTerraform
	t.Cleanup(func() { executeTerraform = orig })

	var calls []schema.ConfigAndStacksInfo
	executeTerraform = func(info schema.ConfigAndStacksInfo, _ ...e.ShellCommandOption) error {
		calls = append(calls, info)
		return nil
	}

	targets := []Target{
		{Component: "vpc", Stack: "prod"},
		{Component: "rds", Stack: "prod"},
	}
	args := []string{"-platform=linux_amd64", filepath.Join("root", "providers")}
	require.NoError(t, runMirror("json", targets, args, nil, 1))

	require.Len(t, calls, 2)
	assert.Equal(t, "vpc", calls[0].ComponentFromArg)
	assert.Equal(t, "providers mirror", calls[0].SubCommand)
	assert.True(t, calls[0].SkipInit)
	assert.True(t, calls[0].TerraformCacheExternal)
	assert.Equal(t, "rds", calls[1].ComponentFromArg)
}

func TestRunMirror_StructuredFormatPropagatesError(t *testing.T) {
	orig := executeTerraform
	t.Cleanup(func() { executeTerraform = orig })
	executeTerraform = func(schema.ConfigAndStacksInfo, ...e.ShellCommandOption) error {
		return errors.New("mirror failed")
	}

	err := runMirror("yaml", []Target{{Component: "vpc", Stack: "prod"}}, nil, nil, 1)
	require.ErrorIs(t, err, errUtils.ErrTerraformExecFailed)
}

func TestStartSharedCache_DisabledNoop(t *testing.T) {
	// With caching disabled, tfcache.Start returns nil, so startSharedCache returns a
	// nil setup and a no-op cleanup that is safe to call.
	setup, cleanup, err := startSharedCache(t.Context(), &schema.AtmosConfiguration{})
	require.NoError(t, err)
	assert.Nil(t, setup)
	require.NotNil(t, cleanup)
	cleanup()
}

func TestStartSharedCache_EnabledStartsAndCloses(t *testing.T) {
	// With caching enabled, startSharedCache starts the loopback proxy, returns a live
	// Setup, and the cleanup closes it. Trust verification is a no-op on Linux/BSD.
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Cache = &schema.TerraformCacheConfig{
		Enabled:  true,
		Location: t.TempDir(),
	}

	setup, cleanup, err := startSharedCache(t.Context(), atmosConfig)
	require.NotNil(t, cleanup)
	t.Cleanup(cleanup)

	if err != nil {
		// On platforms that install trust in the OS store (macOS/Windows), the freshly
		// generated loopback cert is not yet trusted, so VerifyTrust rejects it and the
		// proxy is torn down. On Linux/BSD trust comes from SSL_CERT_FILE, so this path
		// is not taken and the happy path below runs (the case Codecov's Linux runner hits).
		require.ErrorIs(t, err, errUtils.ErrCacheCertUntrusted)
		assert.Nil(t, setup)
		return
	}

	require.NotNil(t, setup)
	assert.NotEmpty(t, setup.CertPath())
}

func TestEmitResult(t *testing.T) {
	// emitResult writes structured output via the data package, which needs a writer.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	root := t.TempDir()
	targets := []Target{{Component: "vpc", Stack: "prod"}}
	platforms := []string{"linux_amd64"}

	// Each format path renders without error against an empty cache root.
	for _, format := range []string{"json", "yaml", ""} {
		t.Run("format="+format, func(t *testing.T) {
			require.NoError(t, emitResult(format, root, targets, platforms))
		})
	}
}
