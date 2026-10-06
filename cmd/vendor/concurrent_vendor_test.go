package vendor

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/vendoring"
	"github.com/cloudposse/atmos/pkg/vendoring/concurrency"
)

func concurrentVendorRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	chdirTest(t, root)
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", root)
	t.Setenv("ATMOS_BASE_PATH", root)
	t.Setenv(concurrency.Env, "")
	require.NoError(t, os.Unsetenv(concurrency.Env))
	require.NoError(t, os.WriteFile(filepath.Join(root, "atmos.yaml"), []byte("base_path: .\ncomponents:\n  terraform:\n    base_path: components/terraform\n  helmfile:\n    base_path: components/helmfile\n"), 0o644))
	return root
}

func TestVendorCommandsRejectInvalidConcurrencyBeforeDiscovery(t *testing.T) {
	for _, command := range []string{"pull", "update"} {
		for _, test := range []struct{ name, env, flag string }{{name: "zero flag", flag: "0"}, {name: "negative flag", flag: "-2"}, {name: "malformed environment", env: "many"}} {
			t.Run(command+"/"+test.name, func(t *testing.T) {
				root := concurrentVendorRoot(t)
				cmd := newVendorPullCmdWithRealFlags(t)
				if command == "update" {
					cmd = vendorUpdateCmd
					resetCommandFlags(t, cmd)
				}
				if test.env != "" {
					t.Setenv(concurrency.Env, test.env)
				}
				if test.flag != "" {
					require.NoError(t, cmd.Flags().Set(concurrency.Flag, test.flag))
				}
				err := cmd.RunE(cmd, nil)
				require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue, "reject settings before resolving the deliberately absent vendor manifest")
				assert.NoFileExists(t, filepath.Join(root, "vendor.lock.yaml"))
			})
		}
	}
}

func TestVendorUpdateConcurrentProgressPreservesJSON(t *testing.T) {
	root := concurrentVendorRoot(t)
	resetCommandFlags(t, vendorUpdateCmd)
	manifest := "spec:\n  sources:\n    - component: first\n      source: oci://example/first\n      version: '{{.Version}}'\n    - component: second\n      source: oci://example/second\n      version: '{{.Version}}'\n"
	file := filepath.Join(root, "vendor.yaml")
	require.NoError(t, os.WriteFile(file, []byte(manifest), 0o644))
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(&testStreams{stdin: strings.NewReader(""), stdout: stdout, stderr: stderr}))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)
	t.Cleanup(func() { data.Reset(); ui.Reset() })
	t.Setenv(concurrency.Env, "3")
	require.NoError(t, vendorUpdateCmd.Flags().Set(concurrency.Flag, "2"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("check", "true"))
	require.NoError(t, vendorUpdateCmd.Flags().Set("format", "json"))
	require.NoError(t, vendorUpdateCmd.RunE(vendorUpdateCmd, nil))
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &decoded), "progress must never corrupt structured stdout")
	assert.NotEmpty(t, decoded)
	assert.Contains(t, stderr.String(), "first")
	assert.Contains(t, stderr.String(), "second")
	assert.NotContains(t, stdout.String(), "Checking")
	after, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, manifest, string(after))
	assert.NoFileExists(t, filepath.Join(root, "vendor.lock.yaml"))
}

func TestUpdatePullGroupsShareSingleBatch(t *testing.T) {
	for _, mode := range []string{"success", "later invalid selection", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			root := concurrentVendorRoot(t)
			stderr := setupVendorUICapture(t)
			report := &vendoring.UpdateReport{}
			targets := map[string]string{}
			for _, typ := range []string{"terraform", "helmfile"} {
				source := t.TempDir()
				require.NoError(t, os.WriteFile(filepath.Join(source, "payload.txt"), []byte(typ), 0o644))
				target := writeLocalComponentManifestFixture(t, filepath.Join(root, "components", typ), "example", source)
				targets[typ] = target
				report.Results = append(report.Results, vendoring.SourceUpdateResult{Component: "example", File: filepath.Join(target, "component.yaml"), ComponentType: typ, Status: vendoring.StatusUpdated})
			}
			if mode == "later invalid selection" {
				report.Results[0].Component = "missing"
			}
			cmd := newVendorPullTestCmd()
			cmd.Flags().Int(concurrency.Flag, 0, "")
			require.NoError(t, cmd.Flags().Set(concurrency.Flag, "2"))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "canceled" {
				cancel()
			}
			cmd.SetContext(ctx)
			err := runVendorPull(cmd, nil, report, vendorPullParams{})
			if mode == "success" {
				require.NoError(t, err)
				for typ, target := range targets {
					content, readErr := os.ReadFile(filepath.Join(target, "payload.txt"))
					require.NoError(t, readErr)
					assert.Equal(t, typ, string(content))
				}
				assert.Equal(t, 1, strings.Count(stderr.String(), "Vendored 2 packages"), "all component types share one operation and summary")
			} else {
				require.Error(t, err)
				if mode == "canceled" {
					require.ErrorIs(t, err, context.Canceled)
				}
				for _, target := range targets {
					assert.NoFileExists(t, filepath.Join(target, "payload.txt"), "preflight or cancellation must prevent partial materialization")
				}
			}
		})
	}
}

func TestVendorCommandsFlagOverridesMalformedEnvironment(t *testing.T) {
	for _, command := range []string{"pull", "update"} {
		t.Run(command, func(t *testing.T) {
			root := concurrentVendorRoot(t)
			setupVendorUICapture(t)
			source := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(source, "main.tf"), []byte("# local\n"), 0o644))
			manifest := "apiVersion: atmos/v1\nkind: AtmosVendorConfig\nspec:\n  sources:\n    - component: vpc\n      source: '" + filepath.ToSlash(source) + "'\n      targets: [components/terraform/vpc]\n"
			require.NoError(t, os.WriteFile(filepath.Join(root, "vendor.yaml"), []byte(manifest), 0o644))
			t.Setenv(concurrency.Env, "garbage")
			cmd := newVendorPullCmdWithRealFlags(t)
			if command == "update" {
				cmd = vendorUpdateCmd
				resetCommandFlags(t, cmd)
				require.NoError(t, cmd.Flags().Set("check", "true"))
			}
			require.NoError(t, cmd.Flags().Set(concurrency.Flag, "2"))
			require.NoError(t, cmd.Flags().Set("dry-run", "true"))
			require.NoError(t, cmd.RunE(cmd, nil), "explicit valid flags must beat malformed environment values through real config loading")
			assert.NoFileExists(t, filepath.Join(root, "vendor.lock.yaml"))
			assert.NoDirExists(t, filepath.Join(root, "components", "terraform", "vpc"))
		})
	}
}
