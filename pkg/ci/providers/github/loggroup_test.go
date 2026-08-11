package github

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProvider_StartEndGroup(t *testing.T) {
	var buf bytes.Buffer
	p := NewProvider()

	p.StartGroup(&buf, "terraform init (bounded)")
	p.EndGroup(&buf)

	assert.Equal(t, "::group::terraform init (bounded)\n::endgroup::\n", buf.String())
}

func TestProvider_StartGroup_Escaping(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain", in: "build", want: "::group::build\n"},
		{name: "percent", in: "50% off", want: "::group::50%25 off\n"},
		{name: "newline", in: "line1\nline2", want: "::group::line1%0Aline2\n"},
		{name: "carriage return", in: "a\rb", want: "::group::a%0Db\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			NewProvider().StartGroup(&buf, tt.in)
			assert.Equal(t, tt.want, buf.String())
		})
	}
}
