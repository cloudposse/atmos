package gitenv

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// composerFor returns a Composer whose auth manager is the given mock.
func composerFor(m *MockIdentityManager) *Composer {
	return NewComposer(WithManagerFactory(func(*schema.AtmosConfiguration) (IdentityManager, error) { return m, nil }))
}

func TestCompose(t *testing.T) {
	base := []string{"PATH=/bin", "SSH_AUTH_SOCK=/ambient.sock"}
	envs := map[string]map[string]string{
		"repo-id": {"SSH_AUTH_SOCK": "/repo.sock", "GIT_CONFIG_GLOBAL": "/repo.cfg"},
		"flag-id": {"SSH_AUTH_SOCK": "/flag.sock"},
	}

	tests := []struct {
		name         string
		flag, repo   string
		wantEnv      []string
		wantEnsured  []string
		defaultID    string
		wantEnsureOf string
	}{
		{name: "no identity leaves the process environment exactly", wantEnv: base},
		{
			name: "repository identity reaches the subprocess environment", repo: "repo-id",
			wantEnv:     []string{"PATH=/bin", "GIT_CONFIG_GLOBAL=/repo.cfg", "SSH_AUTH_SOCK=/repo.sock"},
			wantEnsured: []string{"repo-id"},
		},
		{
			name: "flag overrides the repository identity", flag: "flag-id", repo: "repo-id",
			wantEnv:     []string{"PATH=/bin", "SSH_AUTH_SOCK=/flag.sock"},
			wantEnsured: []string{"flag-id"},
		},
		{name: "identity=false disables the repository identity", flag: "false", repo: "repo-id", wantEnv: base},
		{
			name: "identity without a value selects the default", flag: cfg.IdentityFlagSelectValue, repo: "repo-id",
			defaultID: "flag-id", wantEnv: []string{"PATH=/bin", "SSH_AUTH_SOCK=/flag.sock"}, wantEnsured: []string{"flag-id"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// The mock fails the test on any call that is not expected, so these expectations also
			// assert which identity is authenticated, and that none is when no identity applies.
			m := NewMockIdentityManager(gomock.NewController(t))
			if tt.defaultID != "" {
				m.EXPECT().GetDefaultIdentity(true).Return(tt.defaultID, nil).Times(1)
			}
			for _, identity := range tt.wantEnsured {
				m.EXPECT().EnsureIdentityEnvironment(gomock.Any(), identity).Return(envs[identity], nil).Times(1)
			}

			got, err := composerFor(m).Compose(context.Background(), &schema.AtmosConfiguration{}, base, tt.flag, tt.repo)

			require.NoError(t, err)
			assert.Equal(t, tt.wantEnv, got)
		})
	}
}

func TestCompose_Errors(t *testing.T) {
	t.Run("authentication failure is returned", func(t *testing.T) {
		boom := errors.New("boom")
		m := NewMockIdentityManager(gomock.NewController(t))
		m.EXPECT().EnsureIdentityEnvironment(gomock.Any(), "repo-id").Return(nil, boom)
		_, err := composerFor(m).Compose(context.Background(), &schema.AtmosConfiguration{}, nil, "", "repo-id")
		require.ErrorIs(t, err, boom)
	})
	t.Run("no default identity", func(t *testing.T) {
		m := NewMockIdentityManager(gomock.NewController(t))
		m.EXPECT().GetDefaultIdentity(true).Return("", errors.New("none"))
		_, err := composerFor(m).Compose(context.Background(), &schema.AtmosConfiguration{}, nil, cfg.IdentityFlagSelectValue, "")
		require.ErrorIs(t, err, errUtils.ErrNoDefaultIdentity)
	})
	t.Run("manager construction failure", func(t *testing.T) {
		c := NewComposer(WithManagerFactory(func(*schema.AtmosConfiguration) (IdentityManager, error) {
			return nil, errors.New("nope")
		}))
		_, err := c.Compose(context.Background(), &schema.AtmosConfiguration{}, nil, "", "repo-id")
		require.ErrorIs(t, err, errUtils.ErrAuthManager)
	})
	t.Run("default factory rejects a nil configuration", func(t *testing.T) {
		_, err := NewComposer().Compose(context.Background(), nil, nil, "", "repo-id")
		require.ErrorIs(t, err, errUtils.ErrInvalidAuthConfig)
	})
}
