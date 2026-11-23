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
	doc.Url = &url.URL{Scheme: "https", Host: "idp.example.com", Path: "/start"}

	// assert the default behavior is to extract the first form with an action
	form, err := NewFormFromDocument(doc, "")
	require.Nil(t, err)
	require.Equal(t, "https://idp.example.com/form_b", form.URL)
	require.Equal(t, url.Values{"b1": []string{"rock"}, "b2": []string{"paper"}}, *form.Values)

	// assert we can provide a specific form filter
	form, err = NewFormFromDocument(doc, "#c")
	require.Nil(t, err)
	require.Equal(t, "https://idp.example.com/form_c", form.URL)
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
			wantURL:    "https://idp.example.com/next",
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
	doc.Url = &url.URL{Scheme: "https", Host: "idp.example.com"}

	form, err := NewFormFromDocument(doc, "")
	require.NoError(t, err)
	assert.Equal(t, url.Values{"keep": {"yes"}, "empty": {""}}, *form.Values)
}

func TestNewFormFromResponse(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://idp.example.com/login", nil)
	require.NoError(t, err)
	res := &http.Response{Body: io.NopCloser(strings.NewReader(`<form action="/go"><input name="k" value="v"/></form>`)), Request: req}
	form, err := NewFormFromResponse(res, "")
	require.NoError(t, err)
	assert.Equal(t, "https://idp.example.com/go", form.URL)
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
	// The buffer lets the handler finish even if Submit fails and the test exits early.
	gotFormCh := make(chan url.Values, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.NoError(t, r.ParseForm())
		gotFormCh <- r.PostForm
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
	assert.Equal(t, url.Values{"token": {"123"}}, <-gotFormCh)

	// Invalid method fails while building the request.
	badRes, err := (&Form{URL: ts.URL, Method: "BAD METHOD", Values: &url.Values{}}).Submit(client)
	if badRes != nil {
		_ = badRes.Body.Close()
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error building request")

	// A server that is no longer listening fails while submitting.
	ts.Close()
	closedRes, err := form.Submit(client)
	if closedRes != nil {
		_ = closedRes.Body.Close()
	}
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error submitting form")
}

// Regression: a GET form submits its fields in the URL query, not in the body.
func TestFormSubmitGetUsesQuery(t *testing.T) {
	type captured struct {
		query url.Values
		body  []byte
	}
	// The buffer lets the handler finish even if Submit fails and the test exits early.
	gotCh := make(chan captured, 1)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotCh <- captured{query: r.URL.Query(), body: body}
		_, _ = w.Write([]byte("ok"))
	}))
	defer ts.Close()

	client, err := provider.NewHTTPClient(http.DefaultTransport, &provider.HTTPClientOptions{})
	require.NoError(t, err)

	form := &Form{URL: ts.URL + "/go?keep=1&token=old", Method: http.MethodGet, Values: &url.Values{"token": {"123"}}}
	res, err := form.Submit(client)
	require.NoError(t, err)
	defer res.Body.Close()

	got := <-gotCh
	// Atmos fork: the form fields replace the action query, so neither "keep" nor the old token survives.
	assert.Equal(t, url.Values{"token": {"123"}}, got.query)
	assert.Empty(t, got.body)
}

// Regression: a form without an action on a reader-based document must not panic on a nil document URL.
func TestNewFormFromResponseMissingDocumentURL(t *testing.T) {
	const body = `<form id="f"><input name="k" value="v"/></form>`

	res := &http.Response{Body: io.NopCloser(strings.NewReader(body))}
	_, err := NewFormFromResponse(res, "#f")
	require.Error(t, err)

	reqURL, perr := url.Parse("https://idp.example.com/login")
	require.NoError(t, perr)
	res = &http.Response{Body: io.NopCloser(strings.NewReader(body)), Request: &http.Request{URL: reqURL}}
	form, err := NewFormFromResponse(res, "#f")
	require.NoError(t, err)
	assert.Equal(t, "https://idp.example.com/login", form.URL)
}

// Regression: a relative form action was stored verbatim, so the request had no scheme or host.
func TestNewFormFromDocumentResolvesRelativeAction(t *testing.T) {
	const html = `<form action="/next"><input name="x" value="1"/></form>`
	newDoc := func(t *testing.T, base string) *goquery.Document {
		t.Helper()
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
		require.NoError(t, err)
		if base != "" {
			u, err := url.Parse(base)
			require.NoError(t, err)
			doc.Url = u
		}
		return doc
	}

	t.Run("relative action resolves against the document URL", func(t *testing.T) {
		form, err := NewFormFromDocument(newDoc(t, "https://idp.example.com/a/b?q=1"), "")
		require.NoError(t, err)
		assert.Equal(t, "https://idp.example.com/next", form.URL)
		req, err := form.BuildRequest()
		require.NoError(t, err)
		assert.Equal(t, "idp.example.com", req.URL.Host)
	})

	t.Run("absolute action is unchanged", func(t *testing.T) {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<form action="https://other.example.com/saml"></form>`))
		require.NoError(t, err)
		doc.Url = &url.URL{Scheme: "https", Host: "idp.example.com", Path: "/a"}
		form, err := NewFormFromDocument(doc, "")
		require.NoError(t, err)
		assert.Equal(t, "https://other.example.com/saml", form.URL)
	})

	t.Run("relative action without a document URL is an error", func(t *testing.T) {
		_, err := NewFormFromDocument(newDoc(t, ""), "")
		require.Error(t, err)
	})

	t.Run("response URL is used to resolve the action", func(t *testing.T) {
		req, err := http.NewRequest(http.MethodGet, "https://idp.example.com/login", nil)
		require.NoError(t, err)
		res := &http.Response{Body: io.NopCloser(strings.NewReader(html)), Request: req}
		form, err := NewFormFromResponse(res, "")
		require.NoError(t, err)
		assert.Equal(t, "https://idp.example.com/next", form.URL)
	})
}

// Regression: a relative form action resolves against the first base[href], not the document URL.
func TestNewFormFromDocumentUsesBaseHref(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{"absolute base", `<base href="https://sso.example.com/sso/"><form action="resume"></form>`, "https://sso.example.com/sso/resume"},
		{"relative base", `<base href="/sso/"><form action="resume"></form>`, "https://idp.example.com/sso/resume"},
		{"no base falls back to document URL", `<form action="resume"></form>`, "https://idp.example.com/login/resume"},
		{"absolute action ignores base", `<base href="/sso/"><form action="https://other.example.com/x"></form>`, "https://other.example.com/x"},
		{"unparsable base falls back to document URL", `<base href=":bad"><form action="resume"></form>`, "https://idp.example.com/login/resume"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := goquery.NewDocumentFromReader(strings.NewReader(tt.html))
			require.NoError(t, err)
			doc.Url = &url.URL{Scheme: "https", Host: "idp.example.com", Path: "/login/start"}
			form, err := NewFormFromDocument(doc, "")
			require.NoError(t, err)
			assert.Equal(t, tt.want, form.URL)
		})
	}

	// Regression: an empty action resolved against base[href] and posted the form to the base path.
	t.Run("empty action uses the document URL, not the base", func(t *testing.T) {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<base href="/assets/"><form action=""><input name="x" value="1"/></form>`))
		require.NoError(t, err)
		doc.Url = &url.URL{Scheme: "https", Host: "idp.example.com", Path: "/login", RawQuery: "state=1"}
		form, err := NewFormFromDocument(doc, "")
		require.NoError(t, err)
		assert.Equal(t, "https://idp.example.com/login?state=1", form.URL)
	})

	t.Run("empty action without document URL is an error", func(t *testing.T) {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<base href="https://sso.example.com/sso/"><form action=""></form>`))
		require.NoError(t, err)
		_, err = NewFormFromDocument(doc, "")
		require.Error(t, err)
	})

	t.Run("relative base without document URL is an error", func(t *testing.T) {
		doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<base href="/sso/"><form action="resume"></form>`))
		require.NoError(t, err)
		_, err = NewFormFromDocument(doc, "")
		require.Error(t, err)
	})
}
