package stack

import (
	"bytes"
	stdio "io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	listpkg "github.com/cloudposse/atmos/pkg/list"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stackConfigTestStreams is a minimal io.Streams implementation for capturing
// data/UI output in tests, mirroring cmd/vendor's setupVendorUICapture.
type stackConfigTestStreams struct {
	stdin  stdio.Reader
	stdout *bytes.Buffer
	stderr *bytes.Buffer
}

func (ts *stackConfigTestStreams) Input() stdio.Reader     { return ts.stdin }
func (ts *stackConfigTestStreams) Output() stdio.Writer    { return ts.stdout }
func (ts *stackConfigTestStreams) Error() stdio.Writer     { return ts.stderr }
func (ts *stackConfigTestStreams) RawOutput() stdio.Writer { return ts.stdout }
func (ts *stackConfigTestStreams) RawError() stdio.Writer  { return ts.stderr }

// initStackConfigTestWriter wires a fresh data writer that captures stdout,
// cleaning up afterward. Needed because runStack* functions write through
// data.Writeln/data.Write, which panics unless data.InitWriter was called.
func initStackConfigTestWriter(t *testing.T) *bytes.Buffer {
	t.Helper()

	streams := &stackConfigTestStreams{stdin: &bytes.Buffer{}, stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)
	return streams.stdout
}

func TestStackPathPatternArg(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "nil args", args: nil, want: ""},
		{name: "empty args", args: []string{}, want: ""},
		{name: "present arg", args: []string{"vars.*"}, want: "vars.*"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, stackPathPatternArg(tt.args))
		})
	}
}

func TestBuildStackConfigRowsFromFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sub", "prod.yaml")
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
	require.NoError(t, os.WriteFile(file, []byte(`vars:
  region: us-east-1
settings:
  enabled: true
`), 0o644))

	rows, err := buildStackConfigRowsFromFile(file, dir)
	require.NoError(t, err)
	require.Contains(t, rows, listpkg.PathRow{File: filepath.ToSlash(filepath.Join("sub", "prod.yaml")), Path: "vars.region", Type: "string", Value: "us-east-1"})
	require.Contains(t, rows, listpkg.PathRow{File: filepath.ToSlash(filepath.Join("sub", "prod.yaml")), Path: "settings.enabled", Type: "bool", Value: "true"})
}

func TestBuildStackConfigRowsFromFile_MissingFile(t *testing.T) {
	_, err := buildStackConfigRowsFromFile(filepath.Join(t.TempDir(), "does-not-exist.yaml"), "")
	require.Error(t, err)
}

func TestBuildStackConfigRowsFromFile_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "bad.yaml")
	require.NoError(t, os.WriteFile(file, []byte("["), 0o644))

	_, err := buildStackConfigRowsFromFile(file, "")
	require.Error(t, err)
}

func TestProvenanceFileForComponentPath(t *testing.T) {
	mctx := merge.NewMergeContext()
	mctx.EnableProvenance()
	mctx.RecordProvenance("components.terraform.vpc.vars.region", merge.ProvenanceEntry{File: "deploy/prod.yaml", Line: 5})

	atmosConfig := &schema.AtmosConfiguration{StacksBaseAbsolutePath: "/repo/stacks"}
	result := &exec.DescribeComponentResult{MergeContext: mctx}

	file, ok := provenanceFileForComponentPath(atmosConfig, result, "terraform", "vpc", "vars.region")
	assert.True(t, ok)
	assert.Equal(t, "deploy/prod.yaml", file)
}

func TestProvenanceFileForComponentPath_NilMergeContext(t *testing.T) {
	atmosConfig := &schema.AtmosConfiguration{}
	result := &exec.DescribeComponentResult{MergeContext: nil}

	file, ok := provenanceFileForComponentPath(atmosConfig, result, "terraform", "vpc", "vars.region")
	assert.False(t, ok)
	assert.Empty(t, file)
}

func TestProvenanceFileForComponentPath_NoProvenanceForPath(t *testing.T) {
	mctx := merge.NewMergeContext()
	mctx.EnableProvenance()
	// Record provenance for a different path only.
	mctx.RecordProvenance("components.terraform.vpc.vars.other", merge.ProvenanceEntry{File: "deploy/prod.yaml", Line: 5})

	atmosConfig := &schema.AtmosConfiguration{}
	result := &exec.DescribeComponentResult{MergeContext: mctx}

	file, ok := provenanceFileForComponentPath(atmosConfig, result, "terraform", "vpc", "vars.region")
	assert.False(t, ok)
	assert.Empty(t, file)
}

func TestRelativePathForStackDisplay(t *testing.T) {
	// Use a real, OS-native absolute directory (with a drive letter on
	// Windows) rather than a synthetic filepath.Separator-prefixed string:
	// on Windows, filepath.IsAbs requires a volume name, so a drive-less
	// rooted path like `\repo\stacks` is not actually "absolute" and would
	// hit the early-return branch instead of exercising filepath.Rel.
	base := t.TempDir()
	abs := filepath.Join(base, "deploy", "prod.yaml")

	tests := []struct {
		name     string
		file     string
		basePath string
		want     string
	}{
		{
			name:     "empty basePath returns file as-is",
			file:     abs,
			basePath: "",
			want:     filepath.ToSlash(abs),
		},
		{
			name:     "normal relative case",
			file:     abs,
			basePath: base,
			want:     filepath.ToSlash(filepath.Join("deploy", "prod.yaml")),
		},
		{
			name:     "rel is dot returns file as-is",
			file:     base,
			basePath: base,
			want:     filepath.ToSlash(base),
		},
		{
			name:     "rel escapes basePath (..) returns file as-is",
			file:     filepath.Join(filepath.Dir(base), "other", "prod.yaml"),
			basePath: base,
			want:     filepath.ToSlash(filepath.Join(filepath.Dir(base), "other", "prod.yaml")),
		},
		{
			name:     "non-absolute file returns as-is even with basePath set",
			file:     filepath.Join("relative", "prod.yaml"),
			basePath: base,
			want:     filepath.ToSlash(filepath.Join("relative", "prod.yaml")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, relativePathForStackDisplay(tt.file, tt.basePath))
		})
	}
}

func TestRunStackConfigList_FileBranch(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "prod.yaml")
	require.NoError(t, os.WriteFile(file, []byte(`vars:
  region: us-east-1
`), 0o644))

	oldFile := flagFile
	flagFile = file
	t.Cleanup(func() {
		flagFile = oldFile
	})

	rows, err := runStackConfigList()
	require.NoError(t, err)
	require.Contains(t, rows, listpkg.PathRow{File: filepath.ToSlash(file), Path: "vars.region", Type: "string", Value: "us-east-1"})
}

func TestRunStackConfigList_DescribeBranch(t *testing.T) {
	chdirToValidAtmosProject(t)

	flagComponent = "mycomponent"
	flagStack = "nonprod"
	oldFile := flagFile
	flagFile = ""
	t.Cleanup(func() {
		flagComponent = ""
		flagStack = ""
		flagFile = oldFile
	})

	rows, err := runStackConfigList()
	require.NoError(t, err)
	require.Contains(t, rows, listpkg.PathRow{File: filepath.ToSlash(filepath.Join("deploy", "nonprod.yaml")), Path: "vars.foo", Type: "string", Value: "foo nonprod override"})
}

func TestRunStackConfigList_DescribeBranch_Error(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	t.Setenv("ATMOS_BASE_PATH", ".")

	oldFile := flagFile
	flagFile = ""
	flagComponent = "does-not-matter"
	flagStack = "does-not-matter"
	t.Cleanup(func() {
		flagFile = oldFile
		flagComponent = ""
		flagStack = ""
	})

	_, err := runStackConfigList()
	require.Error(t, err)
}

func TestStackConfigListCmd_RunE(t *testing.T) {
	chdirToValidAtmosProject(t)

	stdout := initStackConfigTestWriter(t)

	flagComponent = "mycomponent"
	flagStack = "nonprod"
	oldFile := flagFile
	oldFormat := flagFormat
	oldDelimiter := flagDelimiter
	flagFile = ""
	flagFormat = "paths"
	flagDelimiter = ""
	t.Cleanup(func() {
		flagComponent = ""
		flagStack = ""
		flagFile = oldFile
		flagFormat = oldFormat
		flagDelimiter = oldDelimiter
	})

	require.NoError(t, stackConfigListCmd.RunE(stackConfigListCmd, []string{"vars.foo"}))
	assert.Contains(t, stdout.String(), "vars.foo")
	assert.NotContains(t, stdout.String(), "vars.bar")
}

// Compile-time sentinel: fail loudly if cfg.ComponentTypeSectionName is renamed,
// since buildStackConfigRowsFromDescribe/provenanceFileForComponentPath depend on it.
var _ = cfg.ComponentTypeSectionName
