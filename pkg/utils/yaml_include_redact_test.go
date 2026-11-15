package utils

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/artifact/redacttest"
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
		{name: "forced getter with userinfo", value: "git::https://user:" + redactTestSecret + "@example.com/repo.git", want: "git::https://example.com/repo.git"},
		{name: "scheme-less userinfo", value: "user:" + redactTestSecret + "@example.com/base.yaml", want: "<redacted>"},
		{name: "scheme-less userinfo with a query", value: "user:" + redactTestSecret + "@example.com/base.yaml .a.b | eval", want: "<redacted>"},
		{name: "scheme-less token as user", value: "forced::user:" + redactTestSecret + "@example.com/repo.git", want: "<redacted>"},
		{name: "scheme-relative userinfo", value: "//user:" + redactTestSecret + "@example.com/base.yaml", want: "//example.com/base.yaml"},
		{name: "local path holding a colon", value: "./stacks/a:b.yaml", want: "./stacks/a:b.yaml"},
		{name: "windows path holding an at sign", value: `C:\stacks\user@host\base.yaml`, want: `C:\stacks\user@host\base.yaml`},
		{name: "local path holding an at sign", value: "./stacks/user@host/base.yaml", want: "./stacks/user@host/base.yaml"},
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
		// also is a second secret that must not reach the error.
		also string
	}{
		{name: "query evaluation failure", include: withUserinfo + "/config.yaml .a[ | eval"},
		{name: "query evaluation failure without eval", include: withUserinfo + "/config.yaml .a["},
		{name: "invalid remote yaml", include: withUserinfo + "/invalid.yaml | eval", rawOK: true},
		{name: "invalid remote yaml without eval", include: withUserinfo + "/invalid.yaml", rawOK: true},
		{name: "fetch failure", include: closed + "/config.yaml"},
		{name: "fetch failure with eval", include: closed + "/config.yaml | eval"},
		{name: "raw fetch failure", include: closed + "/config.txt"},
		{name: "oci include without a file path", include: "oci://user:" + redactTestSecret + "@127.0.0.1:1/org/image:tag"},
		{name: "oci pull failure with a double slash in the password", include: "oci://user:" + redactTestSecret + "//hunter3@127.0.0.1:1/org/image:tag//file.yaml", also: "hunter3"},
		{name: "scheme-less userinfo that is not found", include: "user:" + redactTestSecret + "@127.0.0.1:1/config.yaml"},
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
				if tt.also != "" {
					assert.NotContains(t, err.Error(), tt.also, "no part of the include credential may reach the error text")
				}
			})
		}
	}
}

func TestRedactIncludeError(t *testing.T) {
	cause := errors.New("pull failed")
	tests := []struct {
		name    string
		target  string
		message string
		want    string
	}{
		{name: "no credential", target: "https://example.com/a.yaml", message: "boom at example.com", want: "boom at example.com"},
		{name: "userinfo is cut out", target: "https://user:hunter2@example.com/a", message: "could not parse reference: user:hunter2@example.com/a", want: "could not parse reference: example.com/a"},
		{name: "password quoted on its own", target: "https://user:hunter2@example.com/a", message: "auth failed for hunter2", want: "auth failed for <redacted>"},
		{name: "encoded password is also removed decoded", target: "https://user:p%40ss@example.com/a", message: "bad password p@ss", want: "bad password <redacted>"},
		{name: "forced getter prefix", target: "git::https://user:hunter2@example.com/r.git", message: "clone https://user:hunter2@example.com/r.git failed", want: "clone https://example.com/r.git failed"},
		{name: "user name alone is not a password", target: "ssh://git@example.com/r.git", message: "git failed", want: "git failed"},
		{name: "double colon is part of the password", target: "https://user:pa::ss@example.com/a", message: "bad https://user:pa::ss@example.com/a", want: "bad https://example.com/a"},
		{name: "double colon password quoted on its own", target: "https://user:pa::ss@example.com/a", message: "bad pa::ss", want: "bad <redacted>"},
		{name: "last at sign ends the userinfo", target: "https://user:pa@ss@second@example.com/a", message: "bad https://user:pa@ss@second@example.com/a", want: "bad https://example.com/a"},
		{name: "password with an at sign quoted on its own", target: "https://user:pa@ss@second@example.com/a", message: "bad pa@ss@second", want: "bad <redacted>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := redactIncludeError(fmt.Errorf("%s: %w", tt.message, cause), tt.target)
			require.Error(t, got)
			assert.Equal(t, tt.want+": pull failed", got.Error())
			assert.ErrorIs(t, got, cause, "the cause stays in the chain")
		})
	}
	assert.NoError(t, redactIncludeError(nil, "https://user:hunter2@example.com"))
}

// TestRedactIncludeHelpers_SharedCredentialCases feeds the table shared with artifact.RedactURL to
// both include redaction helpers and asserts the one security property: no user or password of the
// input survives, whether the input is echoed as an argument or quoted by a fetch error.
func TestRedactIncludeHelpers_SharedCredentialCases(t *testing.T) {
	cause := errors.New("pull failed")
	for _, tc := range redacttest.Cases() {
		t.Run(tc.Name, func(t *testing.T) {
			if strings.Contains(tc.Raw, "://") {
				got := RedactIncludeArgument(tc.Raw)
				for _, secret := range tc.Secrets {
					assert.NotContains(t, got, secret, "argument %q", tc.Raw)
				}
			}
			messages := []string{"fetch " + tc.Raw + " failed"}
			for _, quoted := range tc.Quoted {
				messages = append(messages, "auth failed for "+quoted)
			}
			for _, message := range messages {
				got := redactIncludeError(fmt.Errorf("%s: %w", message, cause), tc.Raw)
				require.Error(t, got)
				for _, secret := range tc.Secrets {
					assert.NotContains(t, got.Error(), secret, "error %q", message)
				}
				assert.ErrorIs(t, got, cause)
			}
		})
	}
}
