package utils

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

const redactTestSecret = "hunter2"

func TestRedactIncludeArgument(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "local path", value: "./stacks/base.yaml", want: "./stacks/base.yaml"},
		{name: "local path with query", value: "./base.yaml .a.b | eval", want: "./base.yaml .a.b | eval"},
		{name: "scp style git address", value: "git@github.com:org/repo.git", want: "git@github.com:org/repo.git"},
		{name: "url without credentials", value: "https://example.com/base.yaml .a", want: "https://example.com/base.yaml .a"},
		{name: "url with userinfo", value: "https://user:" + redactTestSecret + "@example.com/base.yaml", want: "https://example.com/base.yaml"},
		{name: "url with userinfo and fragment", value: "https://user:" + redactTestSecret + "@example.com/base.yaml#frag", want: "https://example.com/base.yaml#frag"},
		{name: "forced getter with userinfo", value: "git::https://user:" + redactTestSecret + "@example.com/repo.git", want: "<redacted>"},
		{name: "unparsable url with userinfo", value: "https://user:" + redactTestSecret + "@exa mple.com/%zz", want: "<redacted>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactIncludeArgument(tt.value)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, redactTestSecret)
		})
	}
}

// TestProcessInclude_ErrorsDoNotEchoCredentials runs every error path that formats an include
// argument or target with a credential-bearing URL and checks the credential never reaches the error.
func TestProcessInclude_ErrorsDoNotEchoCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/invalid.yaml":
			_, _ = fmt.Fprint(w, "value: [\n")
		case "/empty.yaml":
			_, _ = fmt.Fprint(w, "# only a comment\n---\n")
		default:
			_, _ = fmt.Fprint(w, "a:\n  b: 1\n")
		}
	}))
	defer server.Close()
	withUserinfo := strings.Replace(server.URL, "http://", "http://user:"+redactTestSecret+"@", 1)
	closed := "http://user:" + redactTestSecret + "@127.0.0.1:1"

	tests := []struct {
		name    string
		include string
		// rawOK reports that !include.raw does not parse the body, so it cannot fail on it.
		rawOK bool
	}{
		{name: "query evaluation failure", include: withUserinfo + "/config.yaml .a[ | eval"},
		{name: "query evaluation failure without eval", include: withUserinfo + "/config.yaml .a["},
		{name: "invalid remote yaml", include: withUserinfo + "/invalid.yaml | eval", rawOK: true},
		{name: "invalid remote yaml without eval", include: withUserinfo + "/invalid.yaml", rawOK: true},
		{name: "fetch failure", include: closed + "/config.yaml"},
		{name: "fetch failure with eval", include: closed + "/config.yaml | eval"},
		{name: "raw fetch failure", include: closed + "/config.txt"},
		{name: "oci include without a file path", include: "oci://user:" + redactTestSecret + "@127.0.0.1:1/org/image:tag"},
		{name: "oci pull failure", include: "oci://user:" + redactTestSecret + "@127.0.0.1:1/org/image:tag//file.yaml"},
	}
	for _, tag := range []string{"!include", "!include.raw"} {
		for _, tt := range tests {
			t.Run(tag+"/"+tt.name, func(t *testing.T) {
				dir := t.TempDir()
				config := &schema.AtmosConfiguration{BasePath: dir}
				_, err := UnmarshalYAMLFromFile[map[string]any](config, "included: "+tag+" "+tt.include+"\n", filepath.Join(dir, "stack.yaml"))
				if tag == "!include.raw" && tt.rawOK {
					require.NoError(t, err)
					return
				}
				require.Error(t, err)
				assert.NotContains(t, err.Error(), redactTestSecret, "the include credential must not reach the error text")
			})
		}
	}
}
