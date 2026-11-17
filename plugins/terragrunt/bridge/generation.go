package atmosbridge

import (
	"context"
	"io"
	"path/filepath"

	"github.com/gruntwork-io/terragrunt/internal/codegen"
	"github.com/gruntwork-io/terragrunt/internal/remotestate"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/spf13/afero"
)

const generatedDirectoryMode = 0o700

// RenderGenerated uses upstream signature and formatting semantics in memory.
// The host applies ownership policies later, in the provisioned working directory.
//
//nolint:gocritic,lintroller // Copy before clearing Disable; this separate module cannot import host perf.
func RenderGenerated(ctx context.Context, logger log.Logger, config codegen.GenerateConfig) ([]byte, error) {
	fs := afero.NewMemMapFs()
	directory := filepath.Join(string(filepath.Separator), "generated")
	if err := fs.MkdirAll(filepath.Join(directory, filepath.Dir(config.Path)), generatedDirectoryMode); err != nil {
		return nil, err
	}
	config.Disable = false
	env := Environment(map[string]string{}, io.Discard, io.Discard).WithFS(fs)
	if err := codegen.WriteToFile(ctx, logger, env, directory, &config); err != nil {
		return nil, err
	}
	return afero.ReadFile(fs, filepath.Join(directory, config.Path))
}

// RenderBackend preserves upstream backend syntax and signatures without writing
// files or provisioning remote backend resources.
//
//nolint:lintroller // The bridge is a separate module and cannot import the host performance tracker.
func RenderBackend(ctx context.Context, logger log.Logger, remote *remotestate.Config, backend map[string]any) ([]byte, error) {
	fs := afero.NewMemMapFs()
	directory := filepath.Join(string(filepath.Separator), "generated")
	if err := fs.MkdirAll(filepath.Join(directory, filepath.Dir(remote.Generate.Path)), generatedDirectoryMode); err != nil {
		return nil, err
	}
	resolved := *remote
	resolved.BackendConfig = backend
	env := Environment(map[string]string{}, io.Discard, io.Discard).WithFS(fs)
	if err := remotestate.New(&resolved).GenerateOpenTofuCode(ctx, logger, env, directory); err != nil {
		return nil, err
	}
	return afero.ReadFile(fs, filepath.Join(directory, remote.Generate.Path))
}
