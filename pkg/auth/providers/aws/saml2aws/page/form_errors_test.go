package page

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A GET form whose action cannot be parsed fails instead of sending a malformed request.
func TestFormBuildRequestGetInvalidURL(t *testing.T) {
	form := &Form{URL: "https://idp.example.com/%zz", Method: "GET", Values: &url.Values{"a": {"b"}}}

	req, err := form.BuildRequest()
	require.Error(t, err)
	assert.Nil(t, req)
	assert.Contains(t, err.Error(), "error building request")
}

func TestResolveFormActionInvalidAction(t *testing.T) {
	base, err := url.Parse("https://idp.example.com/login")
	require.NoError(t, err)

	got, err := resolveFormAction("http://[bad/submit", base)
	require.Error(t, err)
	assert.Empty(t, got)
	assert.Contains(t, err.Error(), "invalid form action")
}
