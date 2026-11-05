package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

//go:generate go run go.uber.org/mock/mockgen -source=browser_completion_test.go -destination=mock_callback_response_test.go -package=api

type callbackResponseWriter interface {
	http.ResponseWriter
	http.Flusher
}

var _ callbackResponseWriter = (*MockcallbackResponseWriter)(nil)

func TestBrowserPublishesCallbackAfterResponseIsFlushed(t *testing.T) {
	t.Parallel()

	callbacks := make(chan Callback, 1)
	writer := NewMockcallbackResponseWriter(gomock.NewController(t))
	headers := http.Header{}
	var response bytes.Buffer
	writer.EXPECT().Header().Return(headers).AnyTimes()
	writer.EXPECT().Write(gomock.Any()).DoAndReturn(func(data []byte) (int, error) {
		require.Empty(t, callbacks, "Receive must not let the bridge close the listener before its response is written")
		return response.Write(data)
	})
	writer.EXPECT().Flush().Do(func() {
		require.Empty(t, callbacks, "Receive must not let the bridge close the listener before its response is flushed")
	})

	callbackHandler("/callback", "expected", callbacks).ServeHTTP(writer,
		httptest.NewRequest(http.MethodGet, "/callback?state=expected&code=valid", nil))

	require.Equal(t, Callback{Code: "valid", State: "expected"}, <-callbacks)
	require.Equal(t, "Authorization response received. You can close this tab.", response.String())
	require.Equal(t, strconv.Itoa(response.Len()), headers.Get("Content-Length"))
}

func TestBrowserCallbackResponseSurvivesImmediateClose(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 5 * time.Second}

	// The bridge closes the listener as soon as Receive returns. The browser
	// must still receive a complete HTTP response when that happens.
	for range 50 {
		listener, err := (LocalBrowser{Interactive: true}).Listen(ctx, "/callback", "expected")
		require.NoError(t, err)
		finished := make(chan error, 1)
		go func() {
			_, receiveErr := listener.Receive(ctx)
			closeErr := listener.Close()
			if receiveErr != nil {
				finished <- receiveErr
			} else {
				finished <- closeErr
			}
		}()

		response, requestErr := client.Get(listener.RedirectURI() + "?state=expected&code=valid")
		var body []byte
		var readErr error
		if response != nil {
			body, readErr = io.ReadAll(response.Body)
			require.NoError(t, response.Body.Close())
		}
		require.NoError(t, <-finished)
		require.NoError(t, requestErr, "publishing a callback must not truncate its browser response")
		require.NoError(t, readErr)
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.Equal(t, "Authorization response received. You can close this tab.", string(body))
	}
}
