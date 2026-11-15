package artifact

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: "", want: ""},
		{name: "no userinfo", raw: "oci://ghcr.io/acme/catalog:stable", want: "oci://ghcr.io/acme/catalog:stable"},
		{name: "digest reference keeps its at sign", raw: "oci://ghcr.io/acme/catalog@sha256:abc", want: "oci://ghcr.io/acme/catalog@sha256:abc"},
		{name: "user and password", raw: "oci://user:hunter2@ghcr.io/acme/catalog:stable", want: "oci://ghcr.io/acme/catalog:stable"},
		{name: "token as user", raw: "https://hunter2@issuer.example", want: "https://issuer.example"},
		{name: "userinfo and digest", raw: "oci://user:hunter2@ghcr.io/acme/catalog@sha256:abc", want: "oci://ghcr.io/acme/catalog@sha256:abc"},
		{name: "malformed escape with userinfo", raw: "oci://user:hunter2@ghcr.io/acme/%zz:stable", want: RedactedValue},
		{name: "scheme-less userinfo", raw: "user:hunter2@ghcr.io", want: RedactedValue},
		{name: "scheme-less userinfo and port", raw: "user:hunter2@ghcr.io:5000", want: RedactedValue},
		{name: "malformed without an at sign is not credential bearing", raw: "oci://ghcr.io/%zz", want: "oci://ghcr.io/%zz"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RedactURL(tt.raw)
			assert.Equal(t, tt.want, got)
			assert.NotContains(t, got, "hunter2")
		})
	}
}
