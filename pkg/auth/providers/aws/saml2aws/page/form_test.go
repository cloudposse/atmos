package page

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/provider"
)

var errRead = errors.New("read failure")

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errRead }

func TestNewFormFromDocument(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("example", "multi-form.html"))
	require.Nil(t, err)

	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(data))
	require.Nil(t, err)

	// assert the default behavior is to extract the first form with an action
	form, err := NewFormFromDocument(doc, "")
	require.Nil(t, err)
	require.Equal(t, "/form_b", form.URL)
	require.Equal(t, url.Values{"b1": []string{"rock"}, "b2": []string{"paper"}}, *form.Values)

	// assert we can provide a specific form filter
	form, err = NewFormFromDocument(doc, "#c")
	require.Nil(t, err)
	require.Equal(t, "/form_c", form.URL)
	require.Equal(t, url.Values{"c1": []string{"now"}}, *form.Values)
}

func TestNewFormFromDocumentNoMatchingForm(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<form id="a"><input name="x" value="1"/></form>`))
	require.NoError(t, err)

	// The default filter requires an action attribute.
	_, err = NewFormFromDocument(doc, "")
	require.Error(t, err)

	_, err = NewFormFromDocument(doc, "#missing")
	require.Error(t, err)
}

func TestNewFormFromDocumentDefaultsAndMethod(t *testing.T) {
	docURL, err := url.Parse("https://idp.example.com/login?step=1")
	require.NoError(t, err)

	tests := []struct {
		name       string
		html       string
		wantURL    string
		wantMethod string
	}{
		{
			name:       "no action falls back to document URL",
			html:       `<form id="f"><input name="a" value="1"/></form>`,
			wantURL:    docURL.String(),
			wantMethod: http.MethodPost,
		},
		{
			name:       "method attribute is upper-cased",
			html:       `<form id="f" action="/next" method="get"><input name="a" value="1"/></form>`,
			wantURL:    "/next",
			wantMethod: http.MethodGet,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tt.html))
			require.NoError(t, err)
			doc.Url = docURL

			form, err := NewFormFromDocument(doc, "#f")
			require.NoError(t, err)
			assert.Equal(t, tt.wantURL, form.URL)
			assert.Equal(t, tt.wantMethod, form.Method)
			assert.Equal(t, url.Values{"a": {"1"}}, *form.Values)
		})
	}
}

func TestNewFormFromDocumentSkipsInputsWithoutNameOrValue(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<form action="/x">
<input name="keep" value="yes"/>
<input name="novalue"/>
<input value="noname"/>
<input name="empty" value=""/>
</form>`))
	require.NoError(t, err)

	form, err := NewFormFromDocument(doc, "")
	require.NoError(t, err)
	assert.Equal(t, url.Values{"keep": {"yes"}, "empty": {""}}, *form.Values)
}

func TestNewFormFromResponse(t *testing.T) {
	res := &http.Response{Body: io.NopCloser(strings.NewReader(`<form action="/go"><input name="k" value="v"/></form>`))}
	form, err := NewFormFromResponse(res, "")
	require.NoError(t, err)
	assert.Equal(t, "/go", form.URL)
	assert.Equal(t, url.Values{"k": {"v"}}, *form.Values)

	res = &http.Response{Body: io.NopCloser(failingReader{})}
	_, err = NewFormFromResponse(res, "")
	require.Error(t, err)
	assert.ErrorIs(t, err, errRead)
}

func TestFormBuildRequest(t *testing.T) {
	form := &Form{URL: "https://idp.example.com/submit", Method: http.MethodPost, Values: &url.Values{"user": {"alice"}}}

	req, err := form.BuildRequest()
	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, req.Method)
	assert.Equal(t, "https://idp.example.com/submit", req.URL.String())
	assert.Equal(t, "application/x-www-form-urlencoded", req.Header.Get("Content-Type"))
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	assert.Equal(t, "user=alice", string(body))

	form.Method = "BAD METHOD"
	_, err = form.BuildRequest()
	require.Error(t, err)
}

func TestFormSubmit(t *testing.T) {
	var gotForm url.Values
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseForm())
		gotForm = r.PostForm
		_, _ = w.Write([]byte("submitted"))
	}))
	defer ts.Close()

	client, err := provider.NewHTTPClient(http.DefaultTransport, &provider.HTTPClientOptions{})
	require.NoError(t, err)

	form := &Form{URL: ts.URL, Method: http.MethodPost, Values: &url.Values{"token": {"123"}}}
	res, err := form.Submit(client)
	require.NoError(t, err)
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	assert.Equal(t, "submitted", string(body))
	assert.Equal(t, url.Values{"token": {"123"}}, gotForm)

	// Invalid method fails while building the request.
	_, err = (&Form{URL: ts.URL, Method: "BAD METHOD", Values: &url.Values{}}).Submit(client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error building request")

	// A server that is no longer listening fails while submitting.
	ts.Close()
	_, err = form.Submit(client)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error submitting form")
}
