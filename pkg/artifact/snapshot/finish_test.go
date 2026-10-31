package snapshot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

func TestFinish_RejectsDuplicatesAfterUnicodeNormalization(t *testing.T) {
	const digestOfEmpty = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	composed := "café.txt"
	decomposed := "café.txt"
	require.NotEqual(t, composed, decomposed)

	c := &capturer{entries: []artifact.Entry{
		{Path: composed, Type: artifact.EntryFile, Mode: 0o644, Digest: digestOfEmpty},
		{Path: decomposed, Type: artifact.EntryFile, Mode: 0o644, Digest: digestOfEmpty},
	}}
	err := c.finish()
	require.ErrorIs(t, err, errUtils.ErrArtifactPackagingInvalid)
	assert.Contains(t, err.Error(), "normalization")

	c.entries = c.entries[:1]
	require.NoError(t, c.finish())
}
