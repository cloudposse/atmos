package auth

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

func deviceInteraction() *pluginv1.Interaction {
	return &pluginv1.Interaction{Kind: deviceCodeInteractionKind, Title: "synthetic-private-title", Fields: map[string]string{
		"verification_uri": "https://issuer.example/device", "user_code": "ABCD-EFGH",
	}}
}

func TestDeviceInteractionOnlyPresentsValidatedFields(t *testing.T) {
	t.Parallel()
	var output strings.Builder
	handler := interactionHandler(t.Context(), true, func(uri, code string) { fmt.Fprintf(&output, "%s %s", uri, code) })
	require.NoError(t, handler(deviceInteraction()))
	assert.Equal(t, "https://issuer.example/device ABCD-EFGH", output.String())
	assert.NotContains(t, output.String(), "synthetic-private")
}

func TestDeviceInteractionRejectsUnsafeOrUnknownData(t *testing.T) {
	t.Parallel()
	for _, change := range []func(*pluginv1.Interaction){
		func(p *pluginv1.Interaction) { p.Kind = "synthetic-private-kind" },
		func(p *pluginv1.Interaction) { p.Fields["debug"] = "synthetic-private-debug" },
		func(p *pluginv1.Interaction) { delete(p.Fields, "user_code") },
		func(p *pluginv1.Interaction) { p.Fields["user_code"] = "BAD\nCODE" },
		func(p *pluginv1.Interaction) { p.Fields["user_code"] = strings.Repeat("A", 33) },
		func(p *pluginv1.Interaction) { p.Fields["verification_uri"] = "http://issuer.example/device" },
		func(p *pluginv1.Interaction) {
			p.Fields["verification_uri"] = "https://user:synthetic-private@issuer.example/device"
		},
		func(p *pluginv1.Interaction) {
			p.Fields["verification_uri"] = "https://issuer.example/device?token=synthetic-private"
		},
		func(p *pluginv1.Interaction) {
			p.Fields["verification_uri"] = "https://issuer.example/device#synthetic-private"
		},
		func(p *pluginv1.Interaction) { p.Fields["verification_uri"] = "https://issuer.example/device\x1b[31m" },
		func(p *pluginv1.Interaction) {
			p.Fields["verification_uri"] = strings.Repeat("x", maxVerificationURILength+1)
		},
	} {
		prompt := deviceInteraction()
		change(prompt)
		handler := interactionHandler(t.Context(), true, func(string, string) { t.Error("invalid prompt reached UI") })
		err := handler(prompt)
		require.Error(t, err)
		assert.NotContains(t, err.Error(), "synthetic-private")
	}
}

func TestDeviceInteractionRequiresAnActiveInteractiveInvocation(t *testing.T) {
	t.Parallel()
	never := func(string, string) { t.Error("unavailable invocation reached UI") }
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, interactionHandler(ctx, true, never)(deviceInteraction()), context.Canceled)
	require.ErrorIs(t, interactionHandler(t.Context(), false, never)(deviceInteraction()), errUtils.ErrPluginInteractionRequired)
	require.ErrorIs(t, interactionHandler(t.Context(), true, nil)(deviceInteraction()), errUtils.ErrPluginInteractionRequired)
}

func TestCredentialStreamPresentsDeviceCodeBeforeReturningCredentials(t *testing.T) {
	t.Parallel()
	prompt := &pluginv1.AuthenticateEvent{Event: &pluginv1.AuthenticateEvent_Interaction{Interaction: deviceInteraction()}}
	credentials := &pluginv1.Credentials{Type: "oidc/v1"}
	result := &pluginv1.AuthenticateEvent{Event: &pluginv1.AuthenticateEvent_Result{Result: credentials}}
	stream := NewMockcredentialStream(gomock.NewController(t))
	gomock.InOrder(
		stream.EXPECT().Recv().Return(prompt, nil),
		stream.EXPECT().Recv().Return(result, nil),
		stream.EXPECT().Recv().Return(nil, io.EOF),
	)
	presented := false
	actual, err := receiveCredentials(stream, interactionHandler(t.Context(), true, func(string, string) { presented = true }))
	require.NoError(t, err)
	assert.True(t, presented)
	assert.Equal(t, credentials, actual)
}

func TestCredentialStreamRejectsUnrenderableOrRepeatedInteractions(t *testing.T) {
	t.Parallel()
	for _, interactive := range []bool{false, true} {
		stream := NewMockcredentialStream(gomock.NewController(t))
		prompt := &pluginv1.AuthenticateEvent{Event: &pluginv1.AuthenticateEvent_Interaction{Interaction: deviceInteraction()}}
		stream.EXPECT().Recv().Return(prompt, nil).Times(map[bool]int{false: 1, true: 2}[interactive])
		actual, err := receiveCredentials(stream, interactionHandler(t.Context(), interactive, func(string, string) {}))
		require.ErrorIs(t, err, errUtils.ErrPluginInteractionRequired)
		assert.Nil(t, actual)
	}
}
