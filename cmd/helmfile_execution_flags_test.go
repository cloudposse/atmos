package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestHelmfileExecutionFlags(t *testing.T) {
	for _, mode := range []string{"dry-run", "redirect-stderr", "global-options"} {
		t.Run(mode, func(t *testing.T) {
			_ = NewTestKit(t)
			root := componentCommandFixture(t, "helmfile")
			binary, err := os.Executable()
			require.NoError(t, err)
			marker := filepath.Join(root, "subprocess-ran")
			stderrFile := filepath.Join(root, "stderr.log")
			t.Setenv("_ATMOS_TEST_COMPONENT_ARGS", marker)
			t.Setenv("_ATMOS_TEST_COMPONENT_STDERR", "helmfile stderr control\n")
			args := []string{"helmfile", "apply", "app", "-s", "dev", "--command", binary}
			switch mode {
			case "global-options":
				args = append(args, "--global-options=--no-color --namespace=test")
			case "dry-run":
				args = append(args, "--dry-run")
			default:
				args = append(args, "--redirect-stderr", stderrFile)
			}
			setComponentCommandArgs(t, args)
			require.NoError(t, Execute())
			switch mode {
			case "global-options":
				actual, err := os.ReadFile(marker)
				require.NoError(t, err)
				require.Contains(t, string(actual), "--no-color\n--namespace=test\napply")
			case "dry-run":
				require.NoFileExists(t, marker, "dry-run must not start the component executable")
			default:
				require.FileExists(t, marker)
				actual, err := os.ReadFile(stderrFile)
				require.NoError(t, err)
				require.Equal(t, "helmfile stderr control\n", string(actual))
			}
		})
	}
}

func TestHelmfileVersionDryRun(t *testing.T) {
	for _, tc := range []struct {
		name   string
		flags  []string
		dryRun bool
	}{
		{name: "execute"},
		{name: "dry-run", flags: []string{"--dry-run"}, dryRun: true},
		{name: "explicit-false", flags: []string{"--dry-run=false"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = NewTestKit(t)
			root := componentCommandFixture(t, "helmfile")
			t.Setenv("ATMOS_LOGS_LEVEL", "Error")
			output := captureHelmfilePreview(t)
			binary, err := os.Executable()
			require.NoError(t, err)
			marker := filepath.Join(root, "subprocess-ran")
			t.Setenv("_ATMOS_TEST_COMPONENT_ARGS", marker)
			args := []string{"helmfile", "version", "--command", binary}
			setComponentCommandArgs(t, append(args, tc.flags...))

			require.NoError(t, Execute())
			if tc.dryRun {
				require.NoFileExists(t, marker, "dry-run must not start the version executable")
				require.Contains(t, output(), binary)
				require.Contains(t, output(), "version")
				return
			}
			actual, err := os.ReadFile(marker)
			require.NoError(t, err)
			require.Equal(t, "version", string(actual))
		})
	}
}

func TestHelmfileDryRunPreservesFiles(t *testing.T) {
	for _, command := range []string{"version", "apply"} {
		for _, existingRedirect := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/existing-redirect=%t", command, existingRedirect), func(t *testing.T) {
				_ = NewTestKit(t)
				root := componentCommandFixture(t, "helmfile")
				binary, err := os.Executable()
				require.NoError(t, err)
				marker := filepath.Join(root, "subprocess-ran")
				t.Setenv("_ATMOS_TEST_COMPONENT_ARGS", marker)
				varfile := filepath.Join(root, "components", "app", "dev-app.helmfile.vars.yaml")
				require.NoError(t, os.WriteFile(varfile, []byte("preserve variables\n"), 0o600))
				redirect := filepath.Join(root, "stderr.log")
				if existingRedirect {
					require.NoError(t, os.WriteFile(redirect, []byte("preserve stderr\n"), 0o600))
				}
				args := []string{"helmfile", command, "--command", binary, "--dry-run", "--redirect-stderr", redirect}
				if command == "apply" {
					args = append(args, "app", "-s", "dev")
				}
				setComponentCommandArgs(t, args)
				require.NoError(t, Execute())
				require.NoFileExists(t, marker)
				actual, err := os.ReadFile(varfile)
				require.NoError(t, err)
				require.Equal(t, "preserve variables\n", string(actual))
				if existingRedirect {
					actual, err = os.ReadFile(redirect)
					require.NoError(t, err)
					require.Equal(t, "preserve stderr\n", string(actual))
				} else {
					require.NoFileExists(t, redirect)
				}
			})
		}
	}
}

func TestHelmfileDryRunPreviewsWithoutPreparation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		section string
		args    []string
		want    string
	}{
		{name: "arguments", args: []string{"--global-options=--no-color --namespace=test", "--", "--selector=app=test"}, want: "--no-color --namespace=test apply --selector=app=test"},
		{name: "authentication", section: "      auth:\n        identities:\n          unavailable:\n            kind: aws/assume-role\n            default: true\n            via: {provider: missing}\n", want: "apply"},
		{name: "dependencies", section: "      dependencies:\n        tools:\n          invalid/tool/name: v999\n", want: "apply"},
		{name: "source", section: "      source: ./missing-source\n", want: "apply"},
		{name: "plugins", section: "      plugins: [./missing-plugin]\n", want: "apply"},
		{name: "target", section: "      provision:\n        targets:\n          gitops:\n            kind: git\n            url: ./missing-repository\n", args: []string{"--", "--target=gitops"}, want: "template"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = NewTestKit(t)
			root := componentCommandFixture(t, "helmfile")
			t.Setenv("ATMOS_LOGS_LEVEL", "Error")
			output := captureHelmfilePreview(t)
			stack := "name: dev\ncomponents:\n  helmfile:\n    app:\n      command: preview-helmfile\n" + tc.section
			require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "dev.yaml"), []byte(stack), 0o600))
			args := []string{"helmfile", "apply", "app", "-s", "dev", "--dry-run"}
			setComponentCommandArgs(t, append(args, tc.args...))
			require.NoError(t, Execute())
			require.Contains(t, output(), "preview-helmfile")
			require.Contains(t, output(), "--state-values-file dev-app.helmfile.vars.yaml")
			require.Contains(t, output(), tc.want)
			require.NotContains(t, output(), "--target")
			if tc.name == "target" {
				require.Contains(t, output(), "gitops")
			}
			require.NoFileExists(t, filepath.Join(root, "components", "app", "dev-app.helmfile.vars.yaml"))
		})
	}
}

// Capture the real UI stream because command startup reinitializes the formatter.
func captureHelmfilePreview(t *testing.T) func() string {
	t.Helper()
	original := os.Stderr
	file, err := os.Create(filepath.Join(t.TempDir(), "preview.log"))
	require.NoError(t, err)
	os.Stderr = file
	ui.ReinitFormatter()
	t.Cleanup(func() {
		os.Stderr = original
		ui.ReinitFormatter()
		require.NoError(t, file.Close())
	})
	return func() string {
		content, err := os.ReadFile(file.Name())
		require.NoError(t, err)
		return string(content)
	}
}

func TestHelmfileDryRunEKS(t *testing.T) {
	for _, tc := range []struct {
		name          string
		identity      string
		componentAuth string
		wantProfile   bool
	}{
		{name: "profile", wantProfile: true},
		{name: "explicit identity", identity: "unavailable"},
		{name: "default identity", componentAuth: "      auth:\n        identities:\n          unavailable:\n            kind: aws/assume-role\n            default: true\n            via: {provider: missing}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = NewTestKit(t)
			root := componentCommandFixture(t, "helmfile")
			kubeconfigDir := filepath.Join(root, "kubeconfigs")
			configureHelmfileEKSPreview(t, root, kubeconfigDir, tc.componentAuth)
			t.Setenv("ATMOS_LOGS_LEVEL", "Error")
			output := captureHelmfilePreview(t)
			marker := filepath.Join(root, "subprocess-ran")
			t.Setenv("_ATMOS_TEST_COMPONENT_ARGS", marker)
			args := []string{"helmfile", "apply", "app", "-s", "dev", "--dry-run"}
			if tc.identity != "" {
				args = append(args, "--identity="+tc.identity)
			}
			setComponentCommandArgs(t, args)
			require.NoError(t, Execute())
			preview := output()
			require.Contains(t, preview, "eks update-kubeconfig --name=preview-cluster --region=us-east-1")
			require.Contains(t, preview, "--kubeconfig=")
			require.Contains(t, preview, "dev-kubecfg")
			require.Contains(t, preview, "preview-helmfile --state-values-file dev-app.helmfile.vars.yaml apply")
			if tc.wantProfile {
				require.Contains(t, preview, "--profile preview-profile")
			} else {
				require.NotContains(t, preview, "--profile")
			}
			require.NoDirExists(t, kubeconfigDir)
			require.NoFileExists(t, marker)
			require.NoFileExists(t, filepath.Join(root, "components", "app", "dev-app.helmfile.vars.yaml"))
		})
	}
}

func configureHelmfileEKSPreview(t *testing.T, root, kubeconfigDir, componentAuth string) {
	t.Helper()
	configPath := filepath.Join(root, "atmos.yaml")
	config, err := os.ReadFile(configPath)
	require.NoError(t, err)
	eksConfig := fmt.Sprintf("use_eks: true\n    kubeconfig_path: %q\n    cluster_name: preview-cluster\n    helm_aws_profile_pattern: preview-profile", kubeconfigDir)
	require.Contains(t, string(config), "use_eks: false")
	require.NoError(t, os.WriteFile(configPath, []byte(strings.Replace(string(config), "use_eks: false", eksConfig, 1)), 0o600))
	stack := "name: dev\ncomponents:\n  helmfile:\n    app:\n      command: preview-helmfile\n      vars:\n        region: us-east-1\n" + componentAuth
	require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "dev.yaml"), []byte(stack), 0o600))
}

func TestHelmfileDryRunRestrictions(t *testing.T) {
	for _, tc := range []struct {
		name      string
		metadata  string
		component string
		args      []string
		wantErr   error
	}{
		{name: "disabled", metadata: "enabled: false", component: "app"},
		{name: "abstract", metadata: "type: abstract", component: "app", wantErr: errUtils.ErrAbstractComponentCantBeProvisioned},
		{name: "locked", metadata: "locked: true", component: "app", wantErr: errUtils.ErrLockedComponentCantBeProvisioned},
		{name: "unknown target", component: "app", args: []string{"--", "--target=missing"}, wantErr: errUtils.ErrProvisionTargetNotFound},
		{name: "missing component", component: "missing", wantErr: errUtils.ErrInvalidComponent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_ = NewTestKit(t)
			root := componentCommandFixture(t, "helmfile")
			stack := "name: dev\ncomponents:\n  helmfile:\n    app:\n      command: preview-helmfile\n"
			if tc.metadata != "" {
				stack += "      metadata:\n        " + tc.metadata + "\n"
			}
			require.NoError(t, os.WriteFile(filepath.Join(root, "stacks", "dev.yaml"), []byte(stack), 0o600))
			t.Setenv("ATMOS_LOGS_LEVEL", "Error")
			output := captureHelmfilePreview(t)
			marker := filepath.Join(root, "subprocess-ran")
			t.Setenv("_ATMOS_TEST_COMPONENT_ARGS", marker)
			args := []string{"helmfile", "apply", tc.component, "-s", "dev", "--dry-run"}
			setComponentCommandArgs(t, append(args, tc.args...))
			err := Execute()
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				require.Contains(t, output(), "not enabled and skipped")
			}
			require.NotContains(t, output(), "Dry run:")
			require.NoFileExists(t, marker)
			require.NoFileExists(t, filepath.Join(root, "components", "app", "dev-app.helmfile.vars.yaml"))
		})
	}
}

func TestHelmfileDryRunMasksBeforeQuoting(t *testing.T) {
	const secret = "quoted'credential"
	for _, mask := range []bool{true, false} {
		t.Run(fmt.Sprintf("mask=%t", mask), func(t *testing.T) {
			_ = NewTestKit(t)
			root := componentCommandFixture(t, "helmfile")
			configPath := filepath.Join(root, "atmos.yaml")
			config, err := os.ReadFile(configPath)
			require.NoError(t, err)
			config = append(config, []byte(fmt.Sprintf("\nsettings:\n  terminal:\n    mask:\n      replacement: MASKED\n      literals: [%q]\n", secret))...)
			require.NoError(t, os.WriteFile(configPath, config, 0o600))
			output := captureHelmfilePreview(t)
			setComponentCommandArgs(t, []string{"helmfile", "apply", "app", "-s", "dev", "--command=helmfile", "--dry-run", fmt.Sprintf("--mask=%t", mask), "--", "--set=password=" + secret})
			require.NoError(t, Execute())
			if mask {
				require.Contains(t, output(), "--set=password=MASKED")
				require.NotContains(t, output(), "quoted")
				require.NotContains(t, output(), "credential")
			} else {
				require.Contains(t, output(), "quoted")
				require.Contains(t, output(), "credential")
				require.NotContains(t, output(), "MASKED")
			}
		})
	}
}
