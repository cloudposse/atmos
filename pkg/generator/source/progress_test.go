package source

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

type progressRecorder struct {
	messages []string
}

func (p *progressRecorder) Update(message string) { p.messages = append(p.messages, message) }

func TestFetchDirectoryUpdatesSharedProgressBeforeDownload(t *testing.T) {
	progress := &progressRecorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		assert.Equal(t, []string{"Fetching source `example`"}, progress.messages)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	_, cleanup, err := FetchDirectory(&schema.AtmosConfiguration{}, "example", server.URL+"/example.zip", 0, WithProgress(progress))
	defer cleanup()
	require.Error(t, err)
	assert.Equal(t, []string{"Fetching source `example`"}, progress.messages)
}

func TestSharedFetchProgressPreservesOperationError(t *testing.T) {
	progress := &progressRecorder{}
	options := &fetchOptions{progress: progress}
	expected := errors.New("download failed")
	err := options.run("fetching", "fetched", func() error { return expected })
	assert.ErrorIs(t, err, expected)
	assert.Equal(t, []string{"fetching"}, progress.messages)
}
