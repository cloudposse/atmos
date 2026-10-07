package initcmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/source"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCopyPreflightRejectsInvalidSourcesAndOptions(t *testing.T) {
	for _, tc := range []struct {
		name, src string
		opts      initOptions
	}{
		{"scaffold-option", "https://example.com/project.zip", initOptions{copy: true, update: true}},
		{"invalid-url", "https://[invalid", initOptions{copy: true}},
		{"unnamed-source", string(filepath.Separator), initOptions{copy: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			err := preflightInitTarget(&tc.opts, tc.src)
			require.Error(t, err)
			entries, readErr := os.ReadDir(".")
			require.NoError(t, readErr)
			assert.Empty(t, entries, "validation must not create a destination")
		})
	}
}

func TestPrepareCopySourceRejectsInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name     string
		selected templates.Configuration
		depth    int
	}{
		{name: "embedded-template", selected: templates.Configuration{Name: "embedded"}},
		{name: "malformed-url", selected: templates.Configuration{Source: "https://[invalid"}},
		{name: "invalid-depth", selected: templates.Configuration{Source: "github.com/acme/project"}, depth: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := &initOptions{copy: true, atmosConfig: &schema.AtmosConfiguration{Init: schema.InitConfig{Depth: tc.depth}}}
			prepared, err := prepareInitSource(opts, &tc.selected, nil)
			require.Error(t, err)
			assert.Nil(t, prepared)
		})
	}
}

func TestRunCopyRejectsMissingTargetNameAndCancellation(t *testing.T) {
	t.Chdir(t.TempDir())
	prepared := &preparedInitSource{directory: &source.Directory{Path: t.TempDir()}}
	require.ErrorIs(t, runCopyInit(context.Background(), &initOptions{}, prepared), errUtils.ErrInvalidFlagValue)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dst := filepath.Join(t.TempDir(), "target")
	require.ErrorIs(t, runCopyInit(ctx, &initOptions{targetDir: dst}, prepared), context.Canceled)
	assert.NoDirExists(t, dst)
}

func TestCopiedProjectReadmeReadError(t *testing.T) {
	src := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(src, "README.md"), 0o755))
	err := displayCopiedProject(&preparedInitSource{name: "example", directory: &source.Directory{Path: src}}, t.TempDir())
	var pathErr *os.PathError
	require.ErrorAs(t, err, &pathErr)
	assert.Equal(t, filepath.Join(src, "README.md"), pathErr.Path)
}

func TestNormalizeInitArgumentRejectsInvalidRepositoryAndURL(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, opts := range []*initOptions{
		{templateName: "https://github.com/owner"},
		{templateName: "examples/demo", atmosConfig: &schema.AtmosConfiguration{Init: schema.InitConfig{Repository: "./local"}}},
	} {
		require.Error(t, normalizeInitArgument(opts, nil))
	}
}
