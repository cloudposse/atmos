package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// Inject errors during evaluation, after discovery has begun, without global
// parser hooks or timing-dependent cancellation goroutines.
func TestDiscoveryEvaluationErrors(t *testing.T) {
	for _, stage := range []string{"generation", "stack", "unit", "child stack", "child unit"} {
		for _, cause := range []error{context.Canceled, context.DeadlineExceeded, errors.New("invalid unit")} {
			t.Run(stage+"/"+cause.Error(), func(t *testing.T) {
				root, err := filepath.EvalSymlinks(t.TempDir())
				require.NoError(t, err)
				first := filepath.Join(root, "a", "terragrunt.stack.hcl")
				if stage == "unit" {
					first = filepath.Join(root, "a", "terragrunt.hcl")
				}
				later := filepath.Join(root, "z", "terragrunt.hcl")
				for _, path := range []string{first, later} {
					require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
					require.NoError(t, os.WriteFile(path, nil, 0o644))
				}
				child := filepath.Join(root, "generated", "terragrunt.hcl")
				if stage == "child stack" {
					child = filepath.Join(root, "generated", "terragrunt.stack.hcl")
				}
				failed, continued := false, false
				evaluate := func(_ context.Context, req model.Request, _ afero.Fs) (*model.Resolution, error) {
					if req.Path == later {
						continued = true
						return &model.Resolution{Path: later, Source: "module"}, nil
					}
					target := req.Path == child || (req.Path == first && (stage == "unit" || (stage == "generation" && req.Command == "generate") || (stage == "stack" && req.Command != "generate")))
					if target {
						failed = true
						return nil, fmt.Errorf("evaluating %s: %w", req.Path, cause)
					}
					result := &model.Resolution{}
					if req.Command != "generate" {
						if stage == "child stack" {
							result.StackConfigs = map[string]string{"child": child}
							result.StackSources = map[string]string{"child": filepath.Join(root, "catalog")}
							result.Stacks = map[string]string{"child": "child"}
						} else if stage == "child unit" {
							result.UnitConfigs = map[string]string{"child": child}
							result.UnitSources = map[string]string{"child": filepath.Join(root, "catalog")}
						}
					}
					return result, nil
				}
				result, err := discoverProjectWithResolver(t.Context(), model.Request{BasePath: root}, evaluate)
				require.True(t, failed, "must reach the intended evaluation failure")
				if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
					require.ErrorIs(t, err, cause)
					require.Nil(t, result, "canceled discovery must not return partial inventory")
					require.False(t, continued, "must stop before evaluating subsequent units")
				} else {
					require.NoError(t, err)
					require.True(t, continued)
					require.Len(t, result.Failures, 1)
					require.Len(t, result.Inventory, 1)
					require.Equal(t, later, result.Inventory[0].Config.Path)
				}
			})
		}
	}
}

func TestDiscoveryContextCanceledDuringEvaluation(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "terragrunt.hcl"), nil, 0o644))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	result, err := discoverProjectWithResolver(ctx, model.Request{BasePath: root}, func(context.Context, model.Request, afero.Fs) (*model.Resolution, error) {
		cancel()
		return &model.Resolution{}, nil
	})
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, result)
}
