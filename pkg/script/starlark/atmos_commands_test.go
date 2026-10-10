package starlark

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/process"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestAtmosBuiltinCommands(t *testing.T) {
	t.Parallel()
	// This public command inventory deliberately does not derive from the registration constant.
	commands := strings.Fields(`about ai ansible atlantis auth aws azure cast ci completion
 composition config container describe devcontainer docs emulator env gcp git helmfile
 help init kubernetes list lsp mcp packer pro profile sbom scaffold secret stack store
 support theme validate vendor version workflow`)
	for _, command := range commands {
		t.Run(command, func(t *testing.T) {
			t.Parallel()
			runner := NewMockRunner(gomock.NewController(t))
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
				assert.Equal(t, []string{command, "--help"}, spec.Args)
				return process.Result{}
			})
			_, err := runSource(t, fmt.Sprintf(`atmos.%s("--help")`, command), WithProcessRunner(runner), WithAtmosCommands(testAtmosCatalog()))
			require.NoError(t, err)
		})
	}
}

func TestAtmosCommandArgumentsAndResults(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		source string
		argv   []string
	}{
		{`atmos.version()`, []string{"version"}},
		{`atmos.scaffold("generate", "example", "target with spaces", flags={"dry-run": True})`, []string{"scaffold", "generate", "example", "target with spaces", "--dry-run"}},
		{`atmos.list("components", flags={"stack":"dev", "format":"json"})`, []string{"list", "components", "--format=json", "--stack=dev"}},
		{`atmos.describe("component", "api", flags={"-s":"dev"})`, []string{"describe", "component", "api", "-s=dev"}},
		{`atmos.config("get", args=["base_path"])`, []string{"config", "get", "base_path"}},
		{`atmos.vendor("pull", flags={"component":["api","worker"],"dry-run":False})`, []string{"vendor", "pull", "--component=api", "--component=worker", "--dry-run=false"}},
		{`atmos.store("set", "store", "key", "$(touch nope); two words")`, []string{"store", "set", "store", "key", "$(touch nope); two words"}},
	} {
		t.Run(tc.source, func(t *testing.T) {
			t.Parallel()
			runner := NewMockRunner(gomock.NewController(t))
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
				assert.Equal(t, tc.argv, spec.Args)
				return process.Result{}
			})
			result, err := runSource(t, "output = "+tc.source+".exit_code", WithProcessRunner(runner), WithAtmosCommands(testAtmosCatalog()))
			require.NoError(t, err)
			assert.Equal(t, "0", result.Value)
		})
	}
}

func TestAtmosCommandPolicies(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	runner := NewMockRunner(gomock.NewController(t))
	runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, spec process.TaskSpec) process.Result {
		assert.Equal(t, filepath.Join(dir, "child"), spec.Dir)
		assert.Equal(t, "scoped", envpkg.SliceToMap(spec.Env)["KEY"])
		_, _ = io.WriteString(spec.Streams.Stdout, "captured")
		_, _ = io.WriteString(spec.Streams.Stderr, "diagnostic")
		return process.Result{Started: true, ExitCode: 7, Err: errUtils.ErrProcessWaitFailed}
	})
	result, err := New(WithProcessRunner(runner), WithAtmosCommands(testAtmosCatalog())).Execute(context.Background(), script.Spec{
		AtmosWorkingDirectory: dir, Stdout: &stdout, Stderr: &stderr,
		Source: `r = atmos.vendor("pull", working_directory="child", env={"KEY":"scoped"}, output="capture", check=False)
output = [r.stdout, r.stderr, r.exit_code]`,
	})
	require.NoError(t, err)
	assert.JSONEq(t, `["captured","diagnostic",7]`, result.Value)
	assert.Empty(t, stdout.String())
	assert.Empty(t, stderr.String())
}

func TestAtmosCommandValidationAndFailure(t *testing.T) {
	t.Parallel()
	for _, source := range []string{`atmos.config(1)`, `atmos.list(args="components")`, `atmos.describe(flags={1:True})`, `atmos.vendor(component="api")`, `atmos.version(output="quiet")`} {
		_, err := runSource(t, source, WithProcessRunner(NewMockRunner(gomock.NewController(t))), WithAtmosCommands(testAtmosCatalog()))
		require.ErrorIs(t, err, errUtils.ErrStarlark, source)
	}
	for _, source := range []string{`atmos.vendor("pull")`, `atmos.vendor("pull",check=False)`} {
		runner := NewMockRunner(gomock.NewController(t))
		runner.EXPECT().Run(gomock.Any(), gomock.Any()).Return(process.Result{ExitCode: -1, Err: errUtils.ErrProcessStartFailed})
		_, err := runSource(t, source, WithProcessRunner(runner), WithAtmosCommands(testAtmosCatalog()))
		require.ErrorIs(t, err, errUtils.ErrStarlarkProcessFailed)
	}
}

func testAtmosCatalog() *flags.CommandCatalog {
	root := &cobra.Command{Use: "atmos"}
	for _, name := range strings.Fields("about ai ansible atlantis auth aws azure cast ci completion composition config container describe devcontainer docs emulator env gcp git helmfile help init kubernetes list lsp mcp packer pro profile sbom scaffold secret stack store support theme validate vendor version workflow terraform helm toolchain") {
		root.AddCommand(&cobra.Command{Use: name})
	}
	return flags.NewCommandCatalog(root, func(path []string) map[string]compat.CompatibilityFlag {
		if path[0] == "terraform" {
			return map[string]compat.CompatibilityFlag{"-detailed-exitcode": {Behavior: compat.AppendToSeparated}}
		}
		return nil
	})
}
