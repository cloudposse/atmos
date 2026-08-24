package target

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestSelectTarget(t *testing.T) {
	gitTarget := map[string]any{
		"kind":       "git",
		"repository": "deployments",
		"path":       "clusters/dev/argocd",
	}
	clusterTarget := map[string]any{"kind": "kubernetes"}

	withTargets := map[string]any{
		"default": "cluster",
		"targets": map[string]any{
			"cluster":         clusterTarget,
			"deployment-repo": gitTarget,
		},
	}

	tests := []struct {
		name       string
		section    map[string]any
		flagTarget string
		wantName   string
		wantKind   string
		wantCfgKey string // a key expected in the returned cfg, "" if cfg is nil
		wantErr    error
	}{
		{
			name:     "nil section falls back to implicit cluster",
			section:  nil,
			wantName: DefaultClusterTargetName,
			wantKind: KindKubernetes,
		},
		{
			name:     "no targets configured falls back to implicit cluster",
			section:  map[string]any{"workdir": map[string]any{"enabled": true}},
			wantName: DefaultClusterTargetName,
			wantKind: KindKubernetes,
		},
		{
			name:       "flag selects a configured git target",
			section:    withTargets,
			flagTarget: "deployment-repo",
			wantName:   "deployment-repo",
			wantKind:   KindGit,
			wantCfgKey: "repository",
		},
		{
			name:     "provision.default is used when no flag",
			section:  withTargets,
			wantName: "cluster",
			wantKind: KindKubernetes,
		},
		{
			name:       "flag overrides provision.default",
			section:    withTargets,
			flagTarget: "deployment-repo",
			wantName:   "deployment-repo",
			wantKind:   KindGit,
			wantCfgKey: "repository",
		},
		{
			name:       "explicit unknown target errors",
			section:    withTargets,
			flagTarget: "does-not-exist",
			wantErr:    errUtils.ErrProvisionTargetNotFound,
		},
		{
			name: "configured target missing kind errors",
			section: map[string]any{
				"targets": map[string]any{"broken": map[string]any{"repository": "x"}},
			},
			flagTarget: "broken",
			wantErr:    errUtils.ErrProvisionTargetKindMissing,
		},
		{
			name: "implicit cluster works even when other targets exist and default is unset",
			section: map[string]any{
				"targets": map[string]any{"deployment-repo": gitTarget},
			},
			wantName: DefaultClusterTargetName,
			wantKind: KindKubernetes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			selected, err := SelectTarget(tt.section, tt.flagTarget)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, selected)
			assert.Equal(t, tt.wantName, selected.Name)
			assert.Equal(t, tt.wantKind, selected.Kind)
			if tt.wantCfgKey != "" {
				require.NotNil(t, selected.Config)
				assert.Contains(t, selected.Config, tt.wantCfgKey)
			}
		})
	}
}
