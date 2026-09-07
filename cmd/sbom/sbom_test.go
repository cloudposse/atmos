package sbom

import (
	"bytes"
	"context"
	"errors"
	stdio "io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/providers/generic"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/sbom"
	"github.com/cloudposse/atmos/pkg/schema"
)

// --- sbomArtifactFilename -------------------------------------------------------

func TestSBOMArtifactFilename(t *testing.T) {
	tests := []struct {
		name   string
		format string
		output string
		want   string
	}{
		{name: "cyclonedx default", format: sbom.FormatCycloneDXJSON, output: "", want: "atmos-sbom.cyclonedx.json"},
		{name: "spdx default", format: sbom.FormatSPDXJSON, output: "", want: "atmos-sbom.spdx.json"},
		{name: "explicit output wins over format", format: sbom.FormatSPDXJSON, output: filepath.Join("nested", "dir", "custom.json"), want: "custom.json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, sbomArtifactFilename(tt.format, tt.output))
		})
	}
}

// --- SetAtmosConfig ---------------------------------------------------------------

func TestSetAtmosConfig(t *testing.T) {
	original := atmosConfig
	t.Cleanup(func() { atmosConfig = original })

	config := &schema.AtmosConfiguration{BasePath: "/example"}
	SetAtmosConfig(config)
	require.Same(t, config, atmosConfig)
}

// --- CommandProvider ---------------------------------------------------------------

func TestSBOMCommandProvider(t *testing.T) {
	provider := CommandProvider{}

	t.Run("GetCommand returns sbom command", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.Equal(t, "sbom", cmd.Use)
	})

	t.Run("GetName returns sbom", func(t *testing.T) {
		assert.Equal(t, "sbom", provider.GetName())
	})

	t.Run("GetGroup returns Security", func(t *testing.T) {
		assert.Equal(t, "Security", provider.GetGroup())
	})

	t.Run("GetFlagsBuilder returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetFlagsBuilder())
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetPositionalArgsBuilder())
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetCompatibilityFlags())
	})

	t.Run("GetAliases returns nil", func(t *testing.T) {
		assert.Nil(t, provider.GetAliases())
	})

	t.Run("IsExperimental returns true", func(t *testing.T) {
		assert.True(t, provider.IsExperimental())
	})

	t.Run("command has the generate subcommand", func(t *testing.T) {
		names := make(map[string]bool)
		for _, subcmd := range provider.GetCommand().Commands() {
			names[subcmd.Name()] = true
		}
		assert.True(t, names["generate"], "expected sbom generate subcommand to be registered")
	})
}

// --- generate RunE -----------------------------------------------------------------

// defaultGenerateFlags resets generateCmd's flags to known values before each
// test so state set by one test case can't leak into the next (generateCmd is
// a package-level var shared across the whole test binary).
func defaultGenerateFlags(t *testing.T, overrides map[string]string) {
	t.Helper()
	values := map[string]string{
		"format":           sbom.FormatCycloneDXJSON,
		"output":           "",
		"upload":           "false",
		"include-files":    "false",
		"scope":            sbom.ScopeTerraform,
		"mode":             sbom.ModeProvenance,
		"subject-name":     "",
		"subject-version":  "",
		"subject-supplier": "",
	}
	for name, value := range overrides {
		values[name] = value
	}
	for name, value := range values {
		require.NoError(t, generateCmd.Flags().Set(name, value))
	}
}

type testStreams struct {
	stdin  stdio.Reader
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func (ts *testStreams) Input() stdio.Reader     { return ts.stdin }
func (ts *testStreams) Output() stdio.Writer    { return ts.stdout }
func (ts *testStreams) Error() stdio.Writer     { return ts.stderr }
func (ts *testStreams) RawOutput() stdio.Writer { return ts.stdout }
func (ts *testStreams) RawError() stdio.Writer  { return ts.stderr }

// initSBOMTestWriter wires a data writer backed by a buffer so RunE calls
// that write the rendered SBOM to stdout can be asserted against.
func initSBOMTestWriter(t *testing.T) *bytes.Buffer {
	t.Helper()
	stdout := &bytes.Buffer{}
	streams := &testStreams{stdin: &bytes.Buffer{}, stdout: stdout, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)
	return stdout
}

func TestGenerateCmdRunEWritesRenderedSBOMToStdout(t *testing.T) {
	stdout := initSBOMTestWriter(t)
	defaultGenerateFlags(t, nil)
	original := atmosConfig
	t.Cleanup(func() { atmosConfig = original })
	SetAtmosConfig(&schema.AtmosConfiguration{BasePath: t.TempDir()})

	require.NoError(t, generateCmd.RunE(generateCmd, nil))
	assert.Contains(t, stdout.String(), `"bomFormat": "CycloneDX"`)
}

func TestGenerateCmdRunEWritesSBOMToOutputFile(t *testing.T) {
	defaultGenerateFlags(t, nil)
	original := atmosConfig
	t.Cleanup(func() { atmosConfig = original })
	SetAtmosConfig(&schema.AtmosConfiguration{BasePath: t.TempDir()})

	outputPath := filepath.Join(t.TempDir(), "sbom.json")
	defaultGenerateFlags(t, map[string]string{"output": outputPath})

	require.NoError(t, generateCmd.RunE(generateCmd, nil))
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), `"bomFormat": "CycloneDX"`)
}

// errorWriter always fails, used to exercise the stdout write-failure path
// (e.g. a broken pipe) without depending on any real OS-level stream state.
type errorWriter struct{}

func (errorWriter) Write([]byte) (int, error) { return 0, errors.New("stdout closed") }

type erroringStreams struct{ stderr *bytes.Buffer }

func (s *erroringStreams) Input() stdio.Reader     { return &bytes.Buffer{} }
func (s *erroringStreams) Output() stdio.Writer    { return errorWriter{} }
func (s *erroringStreams) Error() stdio.Writer     { return s.stderr }
func (s *erroringStreams) RawOutput() stdio.Writer { return errorWriter{} }
func (s *erroringStreams) RawError() stdio.Writer  { return s.stderr }

func TestGenerateCmdRunEReturnsErrorWhenStdoutWriteFails(t *testing.T) {
	ioCtx, err := iolib.NewContext(iolib.WithStreams(&erroringStreams{stderr: &bytes.Buffer{}}))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)

	original := atmosConfig
	t.Cleanup(func() { atmosConfig = original })
	SetAtmosConfig(&schema.AtmosConfiguration{BasePath: t.TempDir()})
	defaultGenerateFlags(t, nil)

	err = generateCmd.RunE(generateCmd, nil)
	require.Error(t, err)
}

func TestGenerateCmdRunEReturnsRenderErrorForUnsupportedFormat(t *testing.T) {
	original := atmosConfig
	t.Cleanup(func() { atmosConfig = original })
	SetAtmosConfig(&schema.AtmosConfiguration{BasePath: t.TempDir()})
	defaultGenerateFlags(t, map[string]string{"format": "xml", "output": filepath.Join(t.TempDir(), "sbom.xml")})

	err := generateCmd.RunE(generateCmd, nil)
	require.ErrorContains(t, err, "unsupported SBOM format")
}

func TestGenerateCmdRunEReturnsBuildErrorForUnsupportedMode(t *testing.T) {
	original := atmosConfig
	t.Cleanup(func() { atmosConfig = original })
	SetAtmosConfig(&schema.AtmosConfiguration{BasePath: t.TempDir()})
	defaultGenerateFlags(t, map[string]string{"mode": "bogus"})

	err := generateCmd.RunE(generateCmd, nil)
	require.ErrorContains(t, err, "unsupported SBOM mode")
}

func TestGenerateCmdRunEReturnsErrorWhenOutputFileCannotBeWritten(t *testing.T) {
	original := atmosConfig
	t.Cleanup(func() { atmosConfig = original })
	SetAtmosConfig(&schema.AtmosConfiguration{BasePath: t.TempDir()})
	// The parent directory doesn't exist, so os.WriteFile must fail.
	badOutput := filepath.Join(t.TempDir(), "does-not-exist", "sbom.json")
	defaultGenerateFlags(t, map[string]string{"output": badOutput})

	err := generateCmd.RunE(generateCmd, nil)
	require.ErrorContains(t, err, "write SBOM")
}

func TestGenerateCmdRunEUploadRequiresDetectedCIProvider(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	t.Cleanup(restore)

	original := atmosConfig
	t.Cleanup(func() { atmosConfig = original })
	SetAtmosConfig(&schema.AtmosConfiguration{BasePath: t.TempDir()})
	defaultGenerateFlags(t, map[string]string{"upload": "true", "output": filepath.Join(t.TempDir(), "sbom.json")})

	err := generateCmd.RunE(generateCmd, nil)
	require.ErrorIs(t, err, errSBOMUploadRequiresCI)
}

// fakeDetectableProvider wraps the real generic provider (which satisfies the
// full ci.Provider interface) and forces Detect() to true, so it can stand in
// for "a CI provider is active" without depending on any specific CI platform
// or environment variables.
type fakeDetectableProvider struct {
	*generic.Provider
	detect bool
}

func (f *fakeDetectableProvider) Detect() bool { return f.detect }

func TestGenerateCmdRunEUploadRejectsProviderWithoutSBOMSupport(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	t.Cleanup(restore)
	ci.Register(&fakeDetectableProvider{Provider: generic.NewProvider(), detect: true})

	original := atmosConfig
	t.Cleanup(func() { atmosConfig = original })
	SetAtmosConfig(&schema.AtmosConfiguration{BasePath: t.TempDir()})
	defaultGenerateFlags(t, map[string]string{"upload": "true", "output": filepath.Join(t.TempDir(), "sbom.json")})

	err := generateCmd.RunE(generateCmd, nil)
	require.ErrorIs(t, err, errSBOMUploadUnsupported)
	require.ErrorContains(t, err, generic.ProviderName)
}

// fakeSBOMUploaderProvider additionally implements ci.SBOMUploader so the
// upload success/failure paths can be exercised without any real CI platform.
type fakeSBOMUploaderProvider struct {
	*generic.Provider
	detect     bool
	uploadFunc func(ctx context.Context, report ci.SBOMReport) (*ci.SBOMUpload, error)
}

func (f *fakeSBOMUploaderProvider) Detect() bool { return f.detect }

func (f *fakeSBOMUploaderProvider) UploadSBOM(ctx context.Context, report ci.SBOMReport) (*ci.SBOMUpload, error) {
	return f.uploadFunc(ctx, report)
}

func TestGenerateCmdRunEUploadsThroughDetectedProvider(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	t.Cleanup(restore)

	var gotReport ci.SBOMReport
	ci.Register(&fakeSBOMUploaderProvider{
		Provider: generic.NewProvider(),
		detect:   true,
		uploadFunc: func(_ context.Context, report ci.SBOMReport) (*ci.SBOMUpload, error) {
			gotReport = report
			return &ci.SBOMUpload{Provider: "fake", Location: "fake://sbom"}, nil
		},
	})

	original := atmosConfig
	t.Cleanup(func() { atmosConfig = original })
	SetAtmosConfig(&schema.AtmosConfiguration{BasePath: t.TempDir()})
	outputPath := filepath.Join(t.TempDir(), "atmos.sbom.json")
	defaultGenerateFlags(t, map[string]string{"upload": "true", "output": outputPath, "format": sbom.FormatSPDXJSON})

	require.NoError(t, generateCmd.RunE(generateCmd, nil))
	assert.Equal(t, "atmos.sbom.json", gotReport.Filename)
	assert.Equal(t, sbom.FormatSPDXJSON, gotReport.Format)
	assert.Contains(t, string(gotReport.Content), `"spdxVersion"`)
}

func TestGenerateCmdRunEPropagatesUploadError(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	t.Cleanup(restore)

	ci.Register(&fakeSBOMUploaderProvider{
		Provider: generic.NewProvider(),
		detect:   true,
		uploadFunc: func(context.Context, ci.SBOMReport) (*ci.SBOMUpload, error) {
			return nil, assert.AnError
		},
	})

	original := atmosConfig
	t.Cleanup(func() { atmosConfig = original })
	SetAtmosConfig(&schema.AtmosConfiguration{BasePath: t.TempDir()})
	defaultGenerateFlags(t, map[string]string{"upload": "true", "output": filepath.Join(t.TempDir(), "sbom.json")})

	err := generateCmd.RunE(generateCmd, nil)
	require.ErrorIs(t, err, assert.AnError)
}
