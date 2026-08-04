package store

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

func TestValidateGetFlags(t *testing.T) {
	require.NoError(t, validateGetFlags(false, false, ""))
	require.NoError(t, validateGetFlags(true, false, "text"))
	require.NoError(t, validateGetFlags(true, true, "text"))
	require.ErrorIs(t, validateGetFlags(true, true, "json"), ErrRawFormatConflict)
}

func TestRenderStoreValue(t *testing.T) {
	content, newline, err := renderStoreValue("k", "v1", "text", false)
	require.NoError(t, err)
	assert.Equal(t, "v1", content)
	assert.True(t, newline)

	content, newline, err = renderStoreValue("k", "v1", "", true)
	require.NoError(t, err)
	assert.Equal(t, "v1", content)
	assert.False(t, newline)

	content, _, err = renderStoreValue("k", "v1", "env", false)
	require.NoError(t, err)
	assert.Equal(t, "k=v1", content)

	content, _, err = renderStoreValue("k", "v1", "json", false)
	require.NoError(t, err)
	assert.Equal(t, `"v1"`, content)
}

func captureStdout(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	streams := &testStreams{stdin: &bytes.Buffer{}, stdout: buf, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	t.Cleanup(func() { data.Reset() })
	return buf
}

func TestRunStoreGet(t *testing.T) {
	stdout := captureStdout(t)
	svc := newFakeStoreService()
	svc.getValues["image_tag"] = "v1.2.3"
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "get", "app-metadata", "image_tag", "--stack", "dev", "--component", "vpc")
	require.NoError(t, err)

	require.Len(t, svc.getCalls, 1)
	assert.Equal(t, "app-metadata", svc.getCalls[0].name)
	assert.Equal(t, "dev", svc.getCalls[0].stack)
	assert.Equal(t, "vpc", svc.getCalls[0].component)
	assert.Equal(t, "image_tag", svc.getCalls[0].key)
	assert.Equal(t, "v1.2.3\n", stdout.String())
}

func TestRunStoreGet_SecretStoreRegistersMaskedValue(t *testing.T) {
	captureStdout(t)
	registered := overrideRegisterSecretValue(t)
	svc := newFakeStoreService()
	svc.secret = true
	svc.getValues["password"] = "hunter2"
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "get", "app-secrets", "password")
	require.NoError(t, err)
	assert.Equal(t, []any{"hunter2"}, *registered)
}

func TestRunStoreGet_NonSecretStoreDoesNotRegisterValue(t *testing.T) {
	captureStdout(t)
	registered := overrideRegisterSecretValue(t)
	svc := newFakeStoreService()
	svc.getValues["image_tag"] = "v1.2.3"
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "get", "app-metadata", "image_tag")
	require.NoError(t, err)
	assert.Empty(t, *registered)
}

func TestRunStoreGet_NotFound(t *testing.T) {
	captureStdout(t)
	svc := newFakeStoreService()
	svc.getErrs["missing"] = assert.AnError
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "get", "app-metadata", "missing")
	require.ErrorIs(t, err, assert.AnError)
}

func TestRunStoreGet_JSONFormat(t *testing.T) {
	stdout := captureStdout(t)
	svc := newFakeStoreService()
	svc.getValues["image_tag"] = "v1.2.3"
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "get", "app-metadata", "image_tag", "--format", "json")
	require.NoError(t, err)
	assert.Equal(t, "\"v1.2.3\"\n", stdout.String())
}

func TestRunStoreGet_RawFormatConflict(t *testing.T) {
	captureStdout(t)
	svc := newFakeStoreService()
	installService(t, svc, nil)

	err := runStoreSubcommand(t, "get", "app-metadata", "key", "--raw", "--format", "json")
	require.ErrorIs(t, err, ErrRawFormatConflict)
}
