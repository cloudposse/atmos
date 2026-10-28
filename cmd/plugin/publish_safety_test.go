package plugin

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
)

//go:generate go run go.uber.org/mock/mockgen -destination=mock_registry_test.go -package=plugin github.com/cloudposse/atmos/pkg/oci Registry

func TestOrphanSigningFlagsAreRejected(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		conflict  string
	}{
		{"cosign version without sign", []string{"--cosign-version=3.0.0"}, "--cosign-version"},
		{"yes without sign", []string{"--yes"}, "--yes"},
		{"short yes without sign", []string{"-y"}, "--yes"},
		{"both with sign", []string{"--sign", "--yes", "--cosign-version=3.0.0"}, ""},
		{"neither", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command := publishCommand()
			require.NoError(t, command.ParseFlags(tt.arguments))
			values := viper.New()
			values.Set(signFlag, command.Flags().Changed(signFlag))
			err := rejectOrphanSigningFlags(command, values)
			if tt.conflict == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrPluginFlagConflict)
			require.Contains(t, err.Error(), tt.conflict)
			require.Contains(t, strings.Join(errUtils.AllHints(err), " "), "--sign")
		})
	}
}

func digestReference(t *testing.T) name.Digest {
	t.Helper()
	reference, err := name.NewDigest("ghcr.io/acme/plugin@sha256:"+strings.Repeat("a", 64), name.StrictValidation)
	require.NoError(t, err)
	return reference
}

func hashOf(t *testing.T, value string) v1.Hash {
	t.Helper()
	hash, err := v1.NewHash(value)
	require.NoError(t, err)
	return hash
}

func TestDeleteReleaseConfirmation(t *testing.T) {
	reference := digestReference(t)
	matching := &v1.Descriptor{Digest: hashOf(t, reference.DigestStr())}
	neverInteractive := func() bool { return false }
	alwaysInteractive := func() bool { return true }
	noConfirm := func(string, string) (bool, error) {
		t.Fatal("the prompt must not be shown")
		return false, nil
	}

	t.Run("non-interactive without --yes is refused before any registry call", func(t *testing.T) {
		client := NewMockRegistry(gomock.NewController(t))
		err := deleteRelease(t.Context(), client, reference, false, deletePrompts{interactive: neverInteractive, confirm: noConfirm})
		require.ErrorIs(t, err, errUtils.ErrPluginConfirmationRequired)
		require.Contains(t, strings.Join(errUtils.AllHints(err), " "), "--yes")
		require.Contains(t, err.Error(), reference.Name())
	})

	t.Run("--yes deletes without prompting", func(t *testing.T) {
		client := NewMockRegistry(gomock.NewController(t))
		client.EXPECT().Tags(gomock.Any(), reference.Context()).Return([]string{"1.0.0"}, nil)
		client.EXPECT().Head(gomock.Any(), reference.Context().Tag("1.0.0")).Return(matching, nil)
		client.EXPECT().Delete(gomock.Any(), reference).Return(nil)
		require.NoError(t, deleteRelease(t.Context(), client, reference, true, deletePrompts{interactive: neverInteractive, confirm: noConfirm}))
	})

	t.Run("confirmation shows repository digest and tags", func(t *testing.T) {
		client := NewMockRegistry(gomock.NewController(t))
		client.EXPECT().Tags(gomock.Any(), reference.Context()).Return([]string{"stable", "1.0.0"}, nil)
		client.EXPECT().Head(gomock.Any(), gomock.Any()).Return(matching, nil).Times(2)
		client.EXPECT().Delete(gomock.Any(), reference).Return(nil)
		var title, description string
		confirm := func(promptTitle, promptDescription string) (bool, error) {
			title, description = promptTitle, promptDescription
			return true, nil
		}
		require.NoError(t, deleteRelease(t.Context(), client, reference, false, deletePrompts{interactive: alwaysInteractive, confirm: confirm}))
		require.NotEmpty(t, title)
		require.Contains(t, description, "ghcr.io/acme/plugin")
		require.Contains(t, description, reference.DigestStr())
		require.Contains(t, description, "1.0.0, stable")
	})

	t.Run("declining keeps the release", func(t *testing.T) {
		client := NewMockRegistry(gomock.NewController(t))
		client.EXPECT().Tags(gomock.Any(), reference.Context()).Return(nil, errors.New("unsupported"))
		decline := func(string, string) (bool, error) { return false, nil }
		require.NoError(t, deleteRelease(t.Context(), client, reference, false, deletePrompts{interactive: alwaysInteractive, confirm: decline}))
	})

	t.Run("prompt failures and registry failures propagate", func(t *testing.T) {
		client := NewMockRegistry(gomock.NewController(t))
		client.EXPECT().Tags(gomock.Any(), reference.Context()).Return(nil, errors.New("unsupported")).Times(2)
		failure := errors.New("prompt failed")
		fail := func(string, string) (bool, error) { return false, failure }
		require.ErrorIs(t, deleteRelease(t.Context(), client, reference, false, deletePrompts{interactive: alwaysInteractive, confirm: fail}), failure)
		denied := errors.New("denied")
		client.EXPECT().Delete(gomock.Any(), reference).Return(denied)
		require.ErrorIs(t, deleteRelease(t.Context(), client, reference, true, deletePrompts{interactive: neverInteractive, confirm: noConfirm}), denied)
	})
}
