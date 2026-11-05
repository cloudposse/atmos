package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	stackexec "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/naming"
	"github.com/cloudposse/atmos/pkg/schema"
)

func productionNamingReporter() StaticNamingReporter {
	return StaticNamingReporter{
		LoadStacks: func(ctx context.Context, config *schema.AtmosConfiguration, inputs StackInputs) (map[string]any, error) {
			return stackexec.LoadStaticStacks(ctx, config, stackexec.StaticStackInputs{Files: inputs.Files, Reader: inputs.Reader})
		},
		Resolver: stackexec.NamingResolver(),
	}
}

func staticNamingResults(ctx context.Context, config *schema.AtmosConfiguration, inputs stackexec.StaticStackInputs) (map[naming.Identity]naming.Result, error) {
	return productionNamingReporter().NamingResults(ctx, config, StackInputs{Files: inputs.Files, Reader: inputs.Reader})
}

func writeStackFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	return dir
}
