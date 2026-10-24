package release

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReleaseCacheProducerAndConsumers(t *testing.T) {
	var warmup pipelineWorkflow
	pipelineYAML(t, ".github/workflows/release-cache-warmup.yml", &warmup)
	require.Equal(t, map[string]string{"contents": "read"}, warmup.Permissions)
	require.Equal(t, "release-cache-warmup", warmup.Concurrency.Group)
	require.False(t, warmup.Concurrency.Cancel)
	require.Len(t, warmup.Jobs, 1)
	job := warmup.Jobs["warmup"]
	require.Equal(t, "github.ref == 'refs/heads/main'", job.If)
	require.Equal(t, 180, job.Timeout)
	require.Contains(t, warmup.On, "workflow_dispatch")
	require.Equal(t, []any{map[string]any{"cron": "35 20 * * *"}}, warmup.On["schedule"])
	push := warmup.On["push"].(map[string]any)
	require.Equal(t, []any{"main"}, push["branches"])
	for _, path := range []string{"go.mod", "go.sum", ".goreleaser.yml", ".github/workflows/release-*.yml", ".github/actions/release-*/**", ".github/actions/setup-go-cache/**"} {
		require.Contains(t, push["paths"], path)
	}

	consumer := pipelineAction(t, "release-build")
	producerSetup := pipelineUses(t, job.Steps, "./.github/actions/setup-go-cache")
	consumerSetup := pipelineUses(t, consumer, "./.github/actions/setup-go-cache")
	for _, setup := range []pipelineStep{producerSetup, consumerSetup} {
		require.Equal(t, "release", setup.With["cache"])
	}
	// Only the checkout prefix differs: both hash exactly the source go.sum,
	// rather than the producer accidentally including nested tools' go.sum files.
	require.Equal(t, producerSetup.With["cache-dependency-path"], strings.TrimPrefix(consumerSetup.With["cache-dependency-path"], "source/"))
	require.Equal(t, "go.sum", producerSetup.With["cache-dependency-path"])
	require.Equal(t, producerSetup.With["go-version-file"], strings.TrimPrefix(consumerSetup.With["go-version-file"], "source/"))

	cacheSteps := pipelineAction(t, "setup-go-cache")
	var releaseRestore pipelineStep
	for _, step := range cacheSteps {
		require.NotContains(t, step.Uses, "actions/cache/save")
		if step.ID == "restore-release" {
			releaseRestore = step
		}
	}
	require.Equal(t, "inputs.cache == 'release'", releaseRestore.If)
	require.True(t, strings.HasPrefix(releaseRestore.Uses, "actions/cache/restore@"))
	boundary := "release-go-cache-v1-${{ runner.os }}-${{ runner.arch }}-go${{ steps.go.outputs.go-version }}-${{ hashFiles('.goreleaser.yml') }}-"
	dependency := "${{ hashFiles(inputs.cache-dependency-path) }}"
	require.Equal(t, boundary+dependency, releaseRestore.With["key"])
	require.Equal(t, []string{boundary + dependency + "-", boundary}, strings.Split(strings.TrimSpace(releaseRestore.With["restore-keys"]), "\n"))
	require.Equal(t, "${{ env.GOCACHE }}\n${{ env.GOMODCACHE }}\n", releaseRestore.With["path"])

	build := pipelineUses(t, job.Steps, "goreleaser/goreleaser-action")
	require.Equal(t, "build --snapshot --clean --skip=before --timeout 150m --config .goreleaser.yml", build.With["args"])
	require.Equal(t, pipelineUses(t, consumer, "goreleaser/goreleaser-action").With["version"], build.With["version"])
	save := pipelineUses(t, job.Steps, "actions/cache/save")
	require.Empty(t, save.If, "default success() must prevent failed/cancelled builds from saving")
	require.Equal(t, releaseRestore.With["path"], save.With["path"])
	require.Equal(t, "${{ steps.go-cache.outputs.cache-primary-key }}-${{ github.run_id }}-${{ github.run_attempt }}", save.With["key"])
	buildIndex, saveIndex := -1, -1
	for i, step := range job.Steps {
		if step.Uses == build.Uses {
			buildIndex = i
		}
		if step.Uses == save.Uses {
			saveIndex = i
		}
		require.False(t, step.ContinueOnError)
		require.NotContains(t, step.Uses, "release-publish")
		require.NotContains(t, step.Uses, "actions/attest")
	}
	require.Less(t, buildIndex, saveIndex)
	for _, step := range consumer {
		require.NotContains(t, step.Uses, "actions/cache/save")
	}
}

func TestReleaseToolsAreInstalledBeforeGoReleaser(t *testing.T) {
	steps := pipelineAction(t, "release-build")
	cosign := pipelineUses(t, steps, "sigstore/cosign-installer")
	syft := pipelineUses(t, steps, "anchore/sbom-action/download-syft")
	require.Equal(t, "v3.1.3", cosign.With["cosign-release"])
	require.Equal(t, "v1.51.1", syft.With["syft-version"])
	require.Equal(t, syft.With["syft-version"], pipelineUses(t, steps, "anchore/sbom-action").With["syft-version"])
	seen := map[string]bool{}
	for _, step := range steps {
		if strings.HasPrefix(step.Uses, "goreleaser/goreleaser-action@") {
			require.True(t, seen[cosign.Uses])
			require.True(t, seen[syft.Uses])
		}
		seen[step.Uses] = true
	}
}

func TestReleaseToolInstallHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("production hooks execute in a Linux POSIX shell")
	}
	var config struct {
		Before struct {
			Hooks []string `yaml:"hooks"`
		} `yaml:"before"`
	}
	pipelineYAML(t, ".goreleaser.yml", &config)
	for _, tool := range []struct{ name, version, module string }{
		{"cosign", "GitVersion: v3.1.3", "github.com/sigstore/cosign/v3/cmd/cosign@v3.1.3"},
		{"syft", "Version: 1.51.1", "github.com/anchore/syft/cmd/syft@v1.51.1"},
	} {
		for _, scenario := range []string{"matching", "missing", "mismatched", "install-fails", "outside-actions"} {
			t.Run(tool.name+"/"+scenario, func(t *testing.T) {
				bin := t.TempDir()
				installed := t.TempDir()
				// Hermetic PATH: a developer's real Go, sudo, cosign, or syft
				// can never run, even in the missing-tool scenario.
				for _, name := range []string{"sh", "grep", "cp"} {
					path, err := exec.LookPath(name)
					require.NoError(t, err)
					require.NoError(t, os.Symlink(path, filepath.Join(bin, name)))
				}
				writeExecutable := func(path, script string) {
					require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o700))
				}
				template := filepath.Join(installed, "template")
				writeExecutable(template, "echo '"+tool.version+"'")
				switch scenario {
				case "matching":
					writeExecutable(filepath.Join(bin, tool.name), "echo '"+tool.version+"'")
				case "mismatched", "install-fails":
					writeExecutable(filepath.Join(bin, tool.name), "echo 'Version: wrong'")
				}
				writeExecutable(filepath.Join(bin, "go"), `
echo "$*" >> "$INSTALL_LOG"
case "$1" in
  install)
    if [ "$INSTALL_FAIL" = true ]; then exit 23; fi
    cp "$TOOL_TEMPLATE" "$INSTALL_BIN/$TOOL_NAME"
    ;;
  env)
    if [ "$2" = GOBIN ]; then echo "$INSTALL_BIN"; else exit 24; fi
    ;;
  *) exit 25 ;;
esac`)
				writeExecutable(filepath.Join(bin, "sudo"), `
test "$1" = cp || exit 26
shift
exec cp "$@"`)
				var hook string
				for _, candidate := range config.Before.Hooks {
					if strings.Contains(candidate, "go install "+tool.module) {
						hook = candidate
					}
				}
				require.NotEmpty(t, hook)
				// Redirect the missing-tool installation away from /usr/local.
				hook = strings.ReplaceAll(hook, "/usr/local/bin", filepath.ToSlash(bin))
				logPath := filepath.Join(installed, "install.log")
				cmd := exec.Command(filepath.Join(bin, "sh"), "-c", hook)
				cmd.Env = []string{
					"PATH=" + bin, "GITHUB_ACTIONS=" + strconv.FormatBool(scenario != "outside-actions"),
					"INSTALL_FAIL=" + strconv.FormatBool(scenario == "install-fails"),
					"INSTALL_LOG=" + logPath, "INSTALL_BIN=" + installed,
					"TOOL_TEMPLATE=" + template, "TOOL_NAME=" + tool.name,
				}
				output, err := cmd.CombinedOutput()
				if scenario == "install-fails" {
					require.Error(t, err, "%s", output)
				} else {
					require.NoError(t, err, "%s", output)
				}
				log, readErr := os.ReadFile(logPath)
				if scenario == "matching" || scenario == "outside-actions" {
					require.True(t, os.IsNotExist(readErr), "unexpected Go invocation: %s", log)
					return
				}
				require.NoError(t, readErr)
				require.Contains(t, string(log), "install "+tool.module)
				if scenario == "install-fails" {
					require.NotContains(t, string(log), "env ", "installation failure must stop the hook immediately")
					return
				}
				// A mismatched earlier PATH entry must be replaced, otherwise
				// GoReleaser would still sign/catalog with the old executable.
				version := exec.Command(filepath.Join(bin, tool.name), "version")
				output, err = version.CombinedOutput()
				require.NoError(t, err)
				require.Equal(t, tool.version+"\n", string(output))
			})
		}
	}
}
