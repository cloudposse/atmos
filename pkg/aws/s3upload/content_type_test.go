package s3upload

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadContentTypesAndRepairHeaders(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	files := map[string]struct{ body, contentType string }{
		"index.HTML":            {"<!doctype html><title>Résumé</title>", "text/html; charset=utf-8"},
		"app.js":                {"console.log('hello');", "text/javascript; charset=utf-8"},
		"data.json":             {`{"message":"Résumé"}`, "application/json; charset=utf-8"},
		"feed.svg":              {`<svg xmlns="http://www.w3.org/2000/svg"></svg>`, "image/svg+xml; charset=utf-8"},
		"manifest.atmosunknown": {`{"message":"Résumé"}`, "application/json; charset=utf-8"},
		"binary.atmosunknown":   {"\x00\x01\x02\x03", "application/octet-stream"},
	}
	for name, file := range files {
		writeSource(t, filepath.Join(root, name), file.body)
	}
	client := newMemoryS3()
	opts := Options{Source: root, Destination: "s3://artifacts/site/"}
	result, err := Upload(t.Context(), client, opts)
	require.NoError(t, err)
	assert.Equal(t, len(files), result.Uploaded)
	for name, file := range files {
		assert.Equal(t, file.contentType, aws.ToString(client.objects["site/"+name].ContentType), name)
		assert.Equal(t, file.body, client.bodies["site/"+name], "detection must not truncate the uploaded body")
	}
	// Repair the old header even though the checksum and size still match.
	client.objects["site/data.json"].ContentType = aws.String("application/json")
	result, err = Upload(t.Context(), client, opts)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Uploaded)
	assert.Equal(t, len(files)-1, result.Unchanged)
	assert.Equal(t, "application/json; charset=utf-8", aws.ToString(client.objects["site/data.json"].ContentType))
	result, err = Upload(t.Context(), client, opts)
	require.NoError(t, err)
	assert.Zero(t, result.Uploaded)
	assert.Equal(t, len(files), result.Unchanged)

	// An explicit delivery header wins over normalization and inference.
	opts.ContentType = "text/plain; charset=iso-8859-1"
	_, err = Upload(t.Context(), client, opts)
	require.NoError(t, err)
	assert.Equal(t, opts.ContentType, aws.ToString(client.objects["site/data.json"].ContentType))
}

type contentTypeFailureReader struct {
	io.ReadSeeker
	readErr, seekErr error
}

func (r contentTypeFailureReader) Read(p []byte) (int, error) {
	return 0, r.readErr
}

func (r contentTypeFailureReader) Seek(offset int64, whence int) (int64, error) {
	return 0, r.seekErr
}

func TestContentTypeDetectionErrors(t *testing.T) {
	t.Parallel()
	failure := errors.New("source unavailable")
	for _, reader := range []io.ReadSeeker{
		contentTypeFailureReader{ReadSeeker: strings.NewReader(""), seekErr: failure},
		contentTypeFailureReader{ReadSeeker: strings.NewReader(""), readErr: failure},
	} {
		_, err := resolveContentType(reader, "unknown.atmosunknown", "")
		assert.ErrorIs(t, err, failure)
	}
}
