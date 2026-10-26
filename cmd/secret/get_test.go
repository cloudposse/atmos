package secret

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateGetFlags(t *testing.T) {
	tests := []struct {
		name          string
		raw           bool
		formatChanged bool
		format        string
		wantErr       bool
	}{
		{name: "no raw, default format", raw: false, formatChanged: false, format: "text"},
		{name: "raw alone", raw: true, formatChanged: false, format: "text"},
		{name: "raw + explicit text", raw: true, formatChanged: true, format: "text"},
		{name: "raw + explicit json conflicts", raw: true, formatChanged: true, format: "json", wantErr: true},
		{name: "raw + explicit env conflicts", raw: true, formatChanged: true, format: "env", wantErr: true},
		{name: "json without raw", raw: false, formatChanged: true, format: "json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateGetFlags(tt.raw, tt.formatChanged, tt.format)
			if tt.wantErr {
				require.ErrorIs(t, err, ErrRawFormatConflict)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestRenderSecretValue(t *testing.T) {
	tests := []struct {
		name        string
		secret      string
		value       any
		format      string
		raw         bool
		wantContent string
		wantNewline bool
	}{
		{name: "raw scalar, no newline", secret: "API", value: "s3cr3t", raw: true, wantContent: "s3cr3t", wantNewline: false},
		{name: "raw ignores format", secret: "API", value: "s3cr3t", format: "json", raw: true, wantContent: "s3cr3t", wantNewline: false},
		{name: "text default, newline", secret: "API", value: "s3cr3t", format: "text", wantContent: "s3cr3t", wantNewline: true},
		{name: "env format", secret: "API", value: "s3cr3t", format: "env", wantContent: "API=s3cr3t", wantNewline: true},
		{name: "json format", secret: "API", value: map[string]any{"a": 1}, format: "json", wantContent: `{"a":1}`, wantNewline: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content, newline, err := renderSecretValue(tt.secret, tt.value, tt.format, tt.raw)
			require.NoError(t, err)
			if tt.format == "json" && tt.wantNewline {
				assert.JSONEq(t, tt.wantContent, content)
			} else {
				assert.Equal(t, tt.wantContent, content)
			}
			assert.Equal(t, tt.wantNewline, newline)
			// Raw output must never carry a trailing newline (the whole point of the flag).
			if tt.raw {
				assert.NotContains(t, content, "\n")
			}
		})
	}
}
