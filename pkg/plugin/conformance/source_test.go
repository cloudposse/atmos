package conformance

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginplatform "github.com/cloudposse/atmos/pkg/plugin/platform"
	"github.com/cloudposse/atmos/pkg/process"
)

func TestSourceOptionsRejectUnexecutableTargetsAndEscapingCoverage(t *testing.T) {
	for _, options := range []Options{
		{Platform: "unsupported/amd64"}, {Platform: "invalid"}, {CoverProfile: "../outside.out"}, {CoverProfile: string(filepath.Separator) + "outside.out"},
	} {
		_, err := Run(t.Context(), t.TempDir(), options)
		require.Error(t, err, "options rejected before reading files or executing source")
	}
	require.NoError(t, validateSourceOptions(&Options{Platform: pluginplatform.Current(), CoverProfile: "coverage.out"}))
	require.NoError(t, validateSourceOptions(&Options{}))
}

func sourceManifest(t *testing.T) (string, *manifest.Config) {
	t.Helper()
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "plugin.yaml"), []byte(conformanceManifest+"  platforms:\n    "+pluginplatform.Current()+": {executable: bin/native}\n"), 0o600))
	definition, err := manifest.Load(directory)
	require.NoError(t, err)
	return directory, definition
}

func TestPrepareSourceSelectsDeclaredTargetAndRunsOnlyRequestedWork(t *testing.T) {
	directory, definition := sourceManifest(t)
	runner := NewMockSourceRunner(gomock.NewController(t))
	target := &suite{}
	options := &Options{Platform: pluginplatform.Current(), SourceRunner: runner}
	require.NoError(t, prepareSource(t.Context(), directory, definition, options, target))
	require.Equal(t, filepath.Join(directory, "bin", "native"), target.executable)
	options.Build, options.CoverProfile = true, "coverage.out"
	runner.EXPECT().Run(gomock.Any(), &SourceRequest{Directory: directory, Platform: pluginplatform.Current(), Build: true, CoverProfile: "coverage.out"}).Return(errUtils.ErrPluginBuild)
	require.ErrorIs(t, prepareSource(t.Context(), directory, definition, options, target), errUtils.ErrPluginBuild)
	options.Platform = "unsupported/amd64"
	require.ErrorIs(t, prepareSource(t.Context(), directory, definition, options, target), errUtils.ErrPluginUnsupportedPlatform)
	options.Platform, options.Build, options.CoverProfile = "", false, ""
	require.NoError(t, prepareSource(t.Context(), directory, definition, options, target))
	delete(definition.Spec.Platforms, pluginplatform.Current())
	require.NoError(t, prepareSource(t.Context(), directory, definition, options, target))
}

//go:generate go run go.uber.org/mock/mockgen -destination=mock_process_test.go -package=conformance github.com/cloudposse/atmos/pkg/process Runner

func TestGoCoveragePreservesTargetAndReportsBeforeSaving(t *testing.T) {
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module example.com/test\n"), 0o600))
	runner := NewMockRunner(gomock.NewController(t))
	var temporary string
	profile := "mode: atomic\nexample.com/test/main.go:1.1,1.2 1 1\n"
	gomock.InOrder(
		runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
			require.Equal(t, "go", task.Command)
			require.Equal(t, directory, task.Dir)
			require.Equal(t, []string{"test", "-count=1", "-covermode=atomic"}, task.Args[:3])
			require.Equal(t, "./...", task.Args[4])
			temporary = strings.TrimPrefix(task.Args[3], "-coverprofile=")
			require.Equal(t, directory, filepath.Dir(temporary))
			target, err := pluginplatform.Parse(pluginplatform.Current())
			require.NoError(t, err)
			require.Contains(t, task.Env, "GOOS="+target.OS)
			require.Contains(t, task.Env, "GOARCH="+target.Architecture)
			require.Contains(t, task.Env, "CGO_ENABLED=0")
			require.NoError(t, os.WriteFile(temporary, []byte(profile), 0o600))
			return process.Result{}
		}),
		runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, task process.TaskSpec) process.Result {
			require.Equal(t, []string{"tool", "cover", "-func=" + temporary}, task.Args)
			require.NoFileExists(t, filepath.Join(directory, "reports", "coverage.out"))
			return process.Result{}
		}),
	)
	source := GoSourceRunner{Runner: runner}
	require.NoError(t, source.Run(t.Context(), &SourceRequest{Directory: directory, CoverProfile: "reports/coverage.out"}))
	actual, err := os.ReadFile(filepath.Join(directory, "reports", "coverage.out"))
	require.NoError(t, err)
	require.Equal(t, profile, string(actual))
	require.NoFileExists(t, temporary)
}

func TestGoCoverageFailurePreservesPriorProfile(t *testing.T) {
	for _, phase := range []string{"tests", "summary", "cancel"} {
		t.Run(phase, func(t *testing.T) {
			directory := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(directory, "go.mod"), []byte("module example.com/test\n"), 0o600))
			require.NoError(t, os.WriteFile(filepath.Join(directory, "coverage.out"), []byte("previous"), 0o600))
			runner := NewMockRunner(gomock.NewController(t))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			testResult, summaryResult := process.Result{}, process.Result{}
			if phase == "tests" {
				testResult.ExitCode = 1
			}
			if phase == "summary" {
				summaryResult.ExitCode = 1
			}
			runner.EXPECT().Run(gomock.Any(), gomock.Any()).Return(testResult)
			if phase != "tests" {
				runner.EXPECT().Run(gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, process.TaskSpec) process.Result {
					if phase == "cancel" {
						cancel()
					}
					return summaryResult
				})
			}
			err := (GoSourceRunner{Runner: runner}).Run(ctx, &SourceRequest{Directory: directory, CoverProfile: "coverage.out"})
			if phase == "cancel" {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.ErrorIs(t, err, errUtils.ErrTestsFailed)
			}
			actual, err := os.ReadFile(filepath.Join(directory, "coverage.out"))
			require.NoError(t, err)
			require.Equal(t, "previous", string(actual))
			temporary, err := filepath.Glob(filepath.Join(directory, ".atmos-plugin-coverage-*"))
			require.NoError(t, err)
			require.Empty(t, temporary)
		})
	}
}

func TestGoSourceRejectsInvalidInputs(t *testing.T) {
	source := GoSourceRunner{Runner: NewMockRunner(gomock.NewController(t))}
	require.ErrorIs(t, source.Run(t.Context(), nil), errUtils.ErrPluginInvalidConfig)
	require.NoError(t, source.Run(t.Context(), &SourceRequest{}))
	require.ErrorIs(t, source.Run(t.Context(), &SourceRequest{CoverProfile: "../coverage.out"}), errUtils.ErrPluginInvalidConfig)
	require.ErrorIs(t, source.Run(t.Context(), &SourceRequest{Directory: t.TempDir(), CoverProfile: "coverage.out"}), errUtils.ErrPluginInvalidConfig)
	require.Error(t, source.Run(t.Context(), &SourceRequest{Directory: t.TempDir(), Build: true}))
	require.Error(t, collectGoCoverage(t.Context(), source.Runner, &SourceRequest{Platform: "invalid"}))
	require.Error(t, collectGoCoverage(t.Context(), source.Runner, &SourceRequest{Directory: filepath.Join(t.TempDir(), "absent")}))
}

func TestCoverageSaveRejectsMissingFilesAndEscapingSymlinks(t *testing.T) {
	directory := t.TempDir()
	require.Error(t, saveCoverage(filepath.Join(directory, "absent"), "coverage.out", "absent"))
	require.Error(t, saveCoverage(directory, "coverage.out", "absent"))
	source := filepath.Join(directory, "source.out")
	require.NoError(t, os.WriteFile(source, []byte("profile"), 0o600))
	require.Error(t, saveCoverage(directory, "source.out/child", source))
	require.Error(t, saveCoverage(directory, ".", source))
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(directory, "outside")); err != nil {
		t.Skip("symbolic links unavailable")
	}
	require.Error(t, saveCoverage(directory, "outside/coverage.out", source))
	require.NoFileExists(t, filepath.Join(outside, "coverage.out"))
}

func TestNativeTestPlatformHonorsVariantsWithoutCrossArchitectureFallback(t *testing.T) {
	for _, test := range []struct {
		requested, host string
		allowed         bool
	}{
		{"linux/amd64/v3", "linux/amd64", false},
		{"linux/amd64", "linux/amd64/v3", true},
		{"linux/arm64/v9.0", "linux/arm64", false},
		{"linux/arm/v7", "linux/arm/v6", false},
		{"linux/arm/v6", "linux/arm/v7", true},
		{"linux/amd64", "windows/amd64", false},
		{"linux/arm64", "linux/amd64", false},
	} {
		err := validateTestPlatform(test.requested, test.host)
		if test.allowed {
			require.NoError(t, err)
		} else {
			require.ErrorIs(t, err, errUtils.ErrPluginUnsupportedPlatform)
		}
	}
}
