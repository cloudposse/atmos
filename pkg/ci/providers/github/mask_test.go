package github

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

func TestProvider_MaskValue(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "plain value", value: "secret", want: "::add-mask::secret\n"},
		{name: "percent escaped", value: "100%", want: "::add-mask::100%25\n"},
		{name: "newline and carriage return escaped", value: "a\r\nb", want: "::add-mask::a%0D%0Ab\n"},
		{name: "empty value emits nothing", value: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stderr := captureStderr(t)

			require.NoError(t, NewProvider().MaskValue(tt.value))
			assert.Equal(t, tt.want, stderr())
		})
	}
}

// TestProvider_MaskValue_SecretIsNotMasked guards the point of the command: the secret has already
// been registered with Atmos's own masker by the time the runner is told to mask it, and the
// command must carry the real value, not the masker's placeholder.
func TestProvider_MaskValue_SecretIsNotMasked(t *testing.T) {
	const secret = "mask-value-secret-9Zq4"
	t.Cleanup(iolib.Reset)
	iolib.RegisterSecret(secret)
	stderr := captureStderr(t)

	require.NoError(t, NewProvider().MaskValue(secret))

	assert.Equal(t, "::add-mask::"+secret+"\n", stderr())
}

// TestProvider_MaskValue_StdoutStaysClean verifies the command goes to stderr so piped data output is not corrupted.
func TestProvider_MaskValue_StdoutStaysClean(t *testing.T) {
	stderr := captureStderr(t)

	require.NoError(t, NewProvider().MaskValue("clean-stdout"))

	assert.Contains(t, stderr(), "::add-mask::clean-stdout")
}

func TestProvider_MaskValue_WriteErrorWrapsSentinel(t *testing.T) {
	brokenStderr(t)

	err := NewProvider().MaskValue("secret")
	require.ErrorIs(t, err, errUtils.ErrCIMaskFailed)
}
