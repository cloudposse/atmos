package github

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/data"
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
			stdout := &bytes.Buffer{}
			streams := &testStreams{stdin: &bytes.Buffer{}, stdout: stdout, stderr: &bytes.Buffer{}}
			ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
			require.NoError(t, err)
			data.InitWriter(ioCtx)

			require.NoError(t, NewProvider().MaskValue(tt.value))
			assert.Equal(t, tt.want, stdout.String())
		})
	}
}

func TestProvider_MaskValue_WriteErrorWrapsSentinel(t *testing.T) {
	streams := &testStreams{stdin: &bytes.Buffer{}, stdout: errWriter{}, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)

	err = NewProvider().MaskValue("secret")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrCIMaskFailed)
}
