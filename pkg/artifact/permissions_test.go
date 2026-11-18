package artifact

import (
	"io/fs"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestParsePermissions(t *testing.T) {
	tests := []struct {
		in      string
		want    fs.FileMode
		wantErr bool
	}{
		{in: "0600", want: 0o600},
		{in: "600", want: 0o600},
		{in: "0644", want: 0o644},
		{in: "0777", want: 0o777},
		{in: "0000", want: 0},
		{in: "", wantErr: true},
		{in: "preserve", wantErr: true},
		{in: "0o600", wantErr: true},
		{in: "0900", wantErr: true},
		{in: "4755", wantErr: true},
		{in: "1777", wantErr: true},
		{in: "06000", wantErr: true},
		{in: "-0600", wantErr: true},
		{in: " 0600", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParsePermissions(tt.in)
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
	assert.Equal(t, fs.FileMode(0o600), DefaultArchivePermissions)
}
