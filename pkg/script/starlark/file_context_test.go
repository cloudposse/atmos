package starlark

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/script"
)

func TestStandaloneFileContextAndLoad(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "deploy.star")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "helper.star"), []byte(`def args():
    return ctx.args
`), 0o600))
	result, err := New().Execute(context.Background(), script.Spec{
		Name: path, WorkingDirectory: t.TempDir(), File: &script.File{Path: path, Args: []string{"--help", "dev"}},
		Source: `#!/usr/bin/env atmos
load("helper.star", "args")
output = [args(), ctx.script.path, ctx.script.directory]
`,
	})
	require.NoError(t, err)
	assert.Contains(t, result.Value, `"--help","dev"`)
	assert.Contains(t, result.Value, "deploy.star")
	_, err = New().Execute(context.Background(), script.Spec{File: &script.File{}, Source: `ctx.args.append("mutate")`})
	require.ErrorContains(t, err, "frozen")
}
