package backend

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/ui"
)

// captureDeleteUI returns what fn writes to the UI channel (stderr), with ANSI
// styling removed. The formatter is initialized against the redirected stream.
func captureDeleteUI(t *testing.T, fn func()) string {
	t.Helper()

	oldStderr := os.Stderr
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = oldStderr })

	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)
	ui.InitFormatter(ioCtx)

	fn()
	require.NoError(t, w.Close())
	os.Stderr = oldStderr

	out, err := io.ReadAll(r)
	require.NoError(t, err)
	return ansi.Strip(string(out))
}

// Deleting a backend states the object count once, with a single warning icon,
// and leaves the success line to the caller's spinner (provisioner
// DeleteBackendWithParams). It used to print a doubled "⚠ ⚠" icon plus two
// extra success lines that repeated the outcome.
func TestDeleteS3Backend_PrintsOneWarningAndNoRedundantSuccess(t *testing.T) {
	fake := newFakeS3(t)
	setupFakeS3Factory(t, fake)
	ctx := context.Background()

	bucketName := "delete-output-bucket"
	require.NoError(t, createBucket(ctx, fake.client, bucketName, "us-east-1"))
	require.NoError(t, enableVersioning(ctx, fake.client, bucketName))
	_, err := fake.client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucketName), Key: aws.String("template.yaml")})
	require.NoError(t, err)

	out := captureDeleteUI(t, func() {
		require.NoError(t, DeleteS3Backend(ctx, nil, map[string]any{"bucket": bucketName, "region": "us-east-1"}, nil, true))
	})

	assert.Contains(t, out, "Deleting backend will permanently remove 1 object(s)")
	assert.NotContains(t, out, "⚠ ⚠", "the warning icon must not be doubled")
	assert.Equal(t, 1, strings.Count(out, "Deleting backend will permanently remove"))
	assert.NotContains(t, out, "Backend deleted", "the caller's spinner reports success")
	assert.NotContains(t, out, "Deleted 1 object(s)", "the per-object success line repeats the warning")
}
