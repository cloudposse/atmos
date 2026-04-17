package cmd

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestIsNoAuthConfigError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"unrelated error", errors.New("boom"), false},
		{"ErrNoProvidersAvailable direct", errUtils.ErrNoProvidersAvailable, true},
		{"ErrNoIdentitiesAvailable direct", errUtils.ErrNoIdentitiesAvailable, true},
		{"ErrNoDefaultIdentity direct", errUtils.ErrNoDefaultIdentity, true},
		{
			name: "ErrNoDefaultIdentity wrapped with ErrWrapFormat",
			err:  fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrNoDefaultIdentity, errors.New("details")),
			want: true,
		},
		{
			name: "ErrNoProvidersAvailable wrapped with %w",
			err:  fmt.Errorf("wrap: %w", errUtils.ErrNoProvidersAvailable),
			want: true,
		},
		{
			name: "deeply wrapped sentinel",
			err:  fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", errUtils.ErrNoIdentitiesAvailable)),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isNoAuthConfigError(tt.err))
		})
	}
}

func TestMaybeOfferProfileFallbackOnAuthConfigError(t *testing.T) {
	ctx := context.Background()
	unrelated := errors.New("unrelated")
	fallbackSentinel := errors.New("fallback sentinel")
	wrappedNoIdentity := fmt.Errorf(errUtils.ErrWrapFormat, errUtils.ErrNoDefaultIdentity, errors.New("x"))

	tests := []struct {
		name       string
		err        error
		setupMocks func(*types.MockAuthManager)
		wantErr    error
	}{
		{
			name:       "nil error is passed through without invoking fallback",
			err:        nil,
			setupMocks: func(m *types.MockAuthManager) {},
			wantErr:    nil,
		},
		{
			name:       "unrelated error is passed through without invoking fallback",
			err:        unrelated,
			setupMocks: func(m *types.MockAuthManager) {},
			wantErr:    unrelated,
		},
		{
			name: "auth-config error triggers fallback; fallback error propagates",
			err:  wrappedNoIdentity,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().MaybeOfferAnyProfileFallback(ctx).Return(fallbackSentinel)
			},
			wantErr: fallbackSentinel,
		},
		{
			name: "auth-config error with nil fallback result returns original error",
			err:  wrappedNoIdentity,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().MaybeOfferAnyProfileFallback(ctx).Return(nil)
			},
			wantErr: wrappedNoIdentity,
		},
		{
			name: "ErrNoProvidersAvailable triggers fallback",
			err:  fmt.Errorf("wrap: %w", errUtils.ErrNoProvidersAvailable),
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().MaybeOfferAnyProfileFallback(ctx).Return(fallbackSentinel)
			},
			wantErr: fallbackSentinel,
		},
		{
			name: "ErrNoIdentitiesAvailable triggers fallback",
			err:  fmt.Errorf("wrap: %w", errUtils.ErrNoIdentitiesAvailable),
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().MaybeOfferAnyProfileFallback(ctx).Return(nil)
			},
			wantErr: fmt.Errorf("wrap: %w", errUtils.ErrNoIdentitiesAvailable),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			m := types.NewMockAuthManager(ctrl)
			tt.setupMocks(m)

			got := maybeOfferProfileFallbackOnAuthConfigError(ctx, m, tt.err)

			if tt.wantErr == nil {
				require.NoError(t, got)
				return
			}
			require.Error(t, got)
			assert.Equal(t, tt.wantErr.Error(), got.Error())
		})
	}
}
