package terraformadoption

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/charmbracelet/huh"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

var (
	buildVersionProbe sync.Once
	probeBytes        []byte
	probeBuildErr     error
)

// Build a tiny stand-in once, then write it into each test's isolated PATH.
func versionProbe(t *testing.T) []byte {
	t.Helper()
	buildVersionProbe.Do(func() {
		path := filepath.Join(t.TempDir(), "probe.exe")
		cmd := exec.CommandContext(t.Context(), "go", "build", "-o", path, "./testdata/versionprobe/main.go")
		cmd.Stderr = os.Stderr
		if probeBuildErr = cmd.Run(); probeBuildErr == nil {
			probeBytes, probeBuildErr = os.ReadFile(path)
		}
	})
	require.NoError(t, probeBuildErr)
	return probeBytes
}

func write(t *testing.T, path, text string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(text), 0o600))
}

func binaries(t *testing.T, commands ...string) string {
	t.Helper()
	bin := t.TempDir()
	for _, name := range commands {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		require.NoError(t, os.WriteFile(filepath.Join(bin, name), versionProbe(t), 0o700))
	}
	t.Setenv("PATH", bin)
	t.Setenv("ATMOS_COMPONENTS_TERRAFORM_COMMAND", "")
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	return bin
}

func TestSelectEngine(t *testing.T) {
	for _, tt := range []struct {
		name, files, commands, explicit, configured, pins, expected, failure string
	}{
		{name: "only Terraform", files: "main.tf", commands: "terraform", expected: "terraform"},
		{name: "only OpenTofu", files: "main.tf.json", commands: "tofu", expected: "tofu"},
		{name: "both installed", files: "main.tf", commands: "tofu terraform", failure: choiceHint},
		{name: "neither installed", files: "main.tf", failure: "no usable engine"},
		{name: "tofu syntax", files: "main.tofu.json", commands: "terraform tofu", expected: "tofu"},
		{name: "tofu unavailable", files: "main.tofu", commands: "terraform", failure: "OpenTofu-only root"},
		{name: "intentional alternative", files: "main.tofu main.tf", commands: "terraform", failure: choiceHint},
		{name: "explicit overrides config", files: "main.tf", commands: "terraform tofu", explicit: "tofu", configured: "terraform", expected: "tofu"},
		{name: "configured", files: "main.tf", commands: "terraform tofu", configured: "terraform", expected: "terraform"},
		{name: "pin tofu", files: "main.tf", commands: "terraform tofu", pins: "opentofu/opentofu 8.7.6", expected: "tofu"},
		{name: "two pins", files: "main.tf", commands: "terraform tofu", pins: "hashicorp/terraform 8.7.6\ntofu 8.7.6", failure: choiceHint},
		{name: "conflicting aliases", files: "main.tf", commands: "tofu", pins: "tofu 8.7.6\nopentofu 8.8.8", failure: "conflicting tofu versions"},
		{name: "wrong version", files: "main.tf", commands: "terraform", pins: "terraform 8.7.5", failure: "reports"},
		{name: "unavailable pin", files: "main.tf", commands: "terraform", pins: "opentofu 8.7.5", failure: "unavailable"},
		{name: "conflicting root and pin", files: "main.tofu", commands: "terraform tofu", pins: "terraform 8.7.6", failure: "conflicting engine evidence"},
		{name: "malformed pin", files: "main.tf", pins: "terraform", failure: "missing version"},
		{name: "invalid selector", files: "main.tf", commands: "terraform", pins: "terraform path:/missing", failure: "unsupported engine version"},
		{name: "unrelated pin", files: "main.tf", commands: "tofu", pins: "go 1.27.0", expected: "tofu"},
		{name: "vars only", files: "dev.tfvars .hidden.tf ~temp.tf", commands: "terraform", failure: "no Terraform"},
		{name: "Terragrunt precedence", files: "main.tf terragrunt.hcl", commands: "terraform", failure: "Terragrunt"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			binaries(t, strings.Fields(tt.commands)...)
			for _, file := range strings.Fields(tt.files) {
				write(t, filepath.Join(root, file), "")
			}
			if tt.pins != "" {
				write(t, filepath.Join(root, ".tool-versions"), tt.pins)
			}
			e, err := Select(t.Context(), root, root, Selection{Explicit: tt.explicit, Configured: tt.configured})
			if tt.failure != "" {
				require.ErrorContains(t, err, tt.failure)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expected, e.Command)
			require.NotEmpty(t, e.Path)
		})
	}
}

func TestEvidenceScope(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "infra", "net")
	write(t, filepath.Join(dir, "main.tf"), "")
	write(t, filepath.Join(root, ".git"), "gitdir: example")
	write(t, filepath.Join(root, ".tool-versions"), "terraform 8.7.6")
	write(t, filepath.Join(dir, ".tool-versions"), "opentofu 8.7.6")
	binaries(t, "terraform", "tofu")
	selectEngine := func() (Engine, error) { return Select(t.Context(), dir, dir, Selection{}) }
	e, err := selectEngine()
	require.NoError(t, err)
	require.Equal(t, "tofu", e.Command)
	require.NoError(t, os.Remove(filepath.Join(dir, ".tool-versions")))
	require.NoError(t, os.Remove(filepath.Join(root, ".tool-versions")))
	atlantis := filepath.Join(root, "atlantis.yaml")
	write(t, atlantis, "projects:\n  - name: other\n    dir: elsewhere\n    terraform_distribution: terraform\n  - name: dev\n    dir: infra/net\n    terraform_distribution: opentofu")
	e, err = selectEngine()
	require.NoError(t, err)
	require.Contains(t, e.Reason, "dev")
	workflows := filepath.Join(root, ".github", "workflows")
	file := filepath.Join(workflows, "build.yml")
	write(t, filepath.Join(workflows, "ignore.txt"), "malformed")
	write(t, file, "defaults:\n  run:\n    working-directory: infra/net\njobs:\n  test:\n    steps:\n      - run: terraform plan")
	_, err = selectEngine()
	require.ErrorContains(t, err, "conflicting engine evidence")
	require.NoError(t, os.Remove(atlantis))
	e, err = selectEngine()
	require.NoError(t, err)
	require.Equal(t, "terraform", e.Command)
	write(t, file, "jobs:\n  test:\n    defaults:\n      run:\n        working-directory: infra/net\n    steps:\n      - run: tofu plan -refresh=false\n      - run: terraform plan && echo x\n      - working-directory: other\n        run: terraform plan")
	e, err = selectEngine()
	require.NoError(t, err)
	require.Equal(t, "tofu", e.Command)
	write(t, file, "jobs: [")
	_, err = selectEngine()
	require.ErrorContains(t, err, "build.yml")
	require.NoError(t, os.Remove(file))
	write(t, atlantis, "projects:\n  - dir: infra/net\n    terraform_distribution: opentofu\n    workflow: custom")
	_, err = selectEngine()
	require.ErrorContains(t, err, choiceHint)
	write(t, atlantis, "projects: [")
	_, err = selectEngine()
	require.ErrorContains(t, err, "atlantis.yaml")
}

func TestSelectionErrors(t *testing.T) {
	dir := t.TempDir()
	bins := binaries(t, "terraform", "tofu")
	_, _, err := sourceFiles(filepath.Join(dir, "absent"))
	require.Error(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "module.tf"), 0o700))
	write(t, filepath.Join(dir, "main.tf"), "")
	_, err = Select(t.Context(), dir, dir, Selection{Interactive: true})
	require.Error(t, err)
	// A named wrapper relative to the module keeps its spelling for persistence.
	e, err := resolve(t.Context(), bins, "./terraform", "explicit", "")
	if runtime.GOOS != "windows" {
		require.NoError(t, err)
		require.Equal(t, "./terraform", e.Command)
	}
	require.Equal(t, "version unavailable", versionLine(context.Background(), dir, filepath.Join(dir, "absent")))
	require.Equal(t, []string{dir}, evidenceDirectories(dir, filepath.Join(dir, "outside")))
}

func TestEnginePicker(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "main.tf"), "")
	binaries(t, "terraform", "tofu")
	preserved := telemetry.PreserveCIEnvVars()
	t.Cleanup(func() { telemetry.RestoreCIEnvVars(preserved) })
	t.Setenv("ATMOS_FORCE_TTY", "true")
	old := viper.GetBool("interactive")
	viper.Set("interactive", true)
	t.Cleanup(func() { viper.Set("interactive", old) })
	var output bytes.Buffer
	t.Cleanup(flags.SetFormRunnerForTest(func(f *huh.Form) error {
		return f.WithAccessible(true).WithInput(strings.NewReader("2\n")).WithOutput(&output).Run()
	}))
	e, err := Select(t.Context(), root, root, Selection{Interactive: true})
	require.NoError(t, err)
	require.Equal(t, "tofu", e.Command)
	require.Equal(t, "interactive selection", e.Reason)
	require.Contains(t, output.String(), "OpenTofu v8.7.6")
}
