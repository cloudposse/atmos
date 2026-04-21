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
	// Kept as a single instance so the pass-through assertion can verify
	// identity equality rather than format-equivalent equality.
	wrappedNoIdentitiesAvailable := fmt.Errorf("wrap: %w", errUtils.ErrNoIdentitiesAvailable)

	// assertMode expresses what the test expects from the returned error
	// without conflating "same instance passed through" and "fallback
	// sentinel produced".
	type assertMode int
	const (
		expectNil            assertMode = iota // got == nil
		expectPassthrough                      // got is the SAME instance as err (no wrap, no replace)
		expectFallbackReturn                   // got is the SAME instance as fallback sentinel
	)

	tests := []struct {
		name       string
		err        error
		setupMocks func(*types.MockAuthManager)
		mode       assertMode
		// wantFallback is only consulted when mode == expectFallbackReturn.
		wantFallback error
	}{
		{
			name:       "nil error is passed through without invoking fallback",
			err:        nil,
			setupMocks: func(m *types.MockAuthManager) {},
			mode:       expectNil,
		},
		{
			name:       "unrelated error is passed through without invoking fallback",
			err:        unrelated,
			setupMocks: func(m *types.MockAuthManager) {},
			mode:       expectPassthrough,
		},
		{
			name: "auth-config error triggers fallback; fallback error propagates",
			err:  wrappedNoIdentity,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().MaybeOfferAnyProfileFallback(ctx).Return(fallbackSentinel)
			},
			mode:         expectFallbackReturn,
			wantFallback: fallbackSentinel,
		},
		{
			name: "auth-config error with nil fallback result returns original error",
			err:  wrappedNoIdentity,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().MaybeOfferAnyProfileFallback(ctx).Return(nil)
			},
			mode: expectPassthrough,
		},
		{
			name: "ErrNoProvidersAvailable triggers fallback",
			err:  fmt.Errorf("wrap: %w", errUtils.ErrNoProvidersAvailable),
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().MaybeOfferAnyProfileFallback(ctx).Return(fallbackSentinel)
			},
			mode:         expectFallbackReturn,
			wantFallback: fallbackSentinel,
		},
		{
			name: "ErrNoIdentitiesAvailable triggers fallback; nil fallback → original err",
			err:  wrappedNoIdentitiesAvailable,
			setupMocks: func(m *types.MockAuthManager) {
				m.EXPECT().MaybeOfferAnyProfileFallback(ctx).Return(nil)
			},
			mode: expectPassthrough,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			m := types.NewMockAuthManager(ctrl)
			tt.setupMocks(m)

			got := maybeOfferProfileFallbackOnAuthConfigError(ctx, m, tt.err)

			switch tt.mode {
			case expectNil:
				require.NoError(t, got)
			case expectPassthrough:
				require.Error(t, got)
				// Identity equality — the dispatcher must return the
				// caller's err instance verbatim (no wrap, no copy).
				// A silent regression that re-wrapped the error would
				// pass a .Error()-string check but fail this one.
				assert.Same(t, tt.err, got,
					"passthrough cases must return the exact same error instance")
				// Also pin the ErrNoAuth* sentinels via errors.Is where
				// applicable — guards against a future refactor that
				// erases the sentinel while keeping the text identical.
				if errors.Is(tt.err, errUtils.ErrNoIdentitiesAvailable) {
					assert.ErrorIs(t, got, errUtils.ErrNoIdentitiesAvailable)
				}
				if errors.Is(tt.err, errUtils.ErrNoProvidersAvailable) {
					assert.ErrorIs(t, got, errUtils.ErrNoProvidersAvailable)
				}
				if errors.Is(tt.err, errUtils.ErrNoDefaultIdentity) {
					assert.ErrorIs(t, got, errUtils.ErrNoDefaultIdentity)
				}
			case expectFallbackReturn:
				require.Error(t, got)
				// The fallback's error instance must propagate unchanged.
				assert.Same(t, tt.wantFallback, got,
					"fallback-return cases must propagate the fallback's error instance unchanged")
			}
		})
	}
}
