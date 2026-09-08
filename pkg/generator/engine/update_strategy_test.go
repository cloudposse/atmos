package engine

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestParseUpdateStrategy(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    UpdateStrategy
		wantErr bool
	}{
		{name: "empty defaults to tracked", input: "", want: UpdateStrategyTracked},
		{name: "tracked", input: "tracked", want: UpdateStrategyTracked},
		{name: "rendered", input: "rendered", want: UpdateStrategyRendered},
		{name: "invalid value", input: "bogus", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseUpdateStrategy(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				assert.ErrorIs(t, err, errUtils.ErrUnknownUpdateStrategy)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
