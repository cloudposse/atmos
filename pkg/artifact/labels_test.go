package artifact

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestValidateLabel(t *testing.T) {
	tests := []struct {
		name       string
		key, value string
		wantErr    string
	}{
		{name: "plain", key: "team", value: "platform"},
		{name: "empty value is allowed", key: "team", value: ""},
		{name: "dots and dashes in key", key: "cost-center.id", value: "1234"},
		{name: "empty key", key: "", value: "x", wantErr: "must not be empty"},
		{name: "space-padded key", key: " team", value: "x", wantErr: "whitespace"},
		{name: "equals in key", key: "a=b", value: "x", wantErr: "--labels"},
		{name: "colon in key", key: "a:b", value: "x", wantErr: "--labels"},
		{name: "comma in key", key: "a,b", value: "x", wantErr: "--labels"},
		{name: "control char in key", key: "a\nb", value: "x", wantErr: "control"},
		{name: "oversized key", key: strings.Repeat("k", 129), value: "x", wantErr: "exceeds"},
		{name: "comma in value", key: "team", value: "a,b", wantErr: "','"},
		{name: "control char in value", key: "team", value: "a\tb", wantErr: "control"},
		{name: "oversized value", key: "team", value: strings.Repeat("v", 1025), wantErr: "exceeds"},
		{name: "colon in value is selectable", key: "ref", value: "refs/heads/main:abc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLabel(tt.key, tt.value)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestValidateLabels_JoinsInKeyOrderAndBoundsCount(t *testing.T) {
	err := ValidateLabels(map[string]string{"b,": "x", "a=": "y", "ok": "fine"})
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
	message := err.Error()
	assert.Less(t, strings.Index(message, `"a="`), strings.Index(message, `"b,"`), "errors follow sorted key order")
	assert.NotContains(t, message, `"ok"`)

	require.NoError(t, ValidateLabels(nil))

	many := make(map[string]string)
	for i := range MaxLabels + 1 {
		many[strings.Repeat("k", 1)+string(rune('a'+i%26))+strings.Repeat("z", i/26)] = "v"
	}
	require.Error(t, ValidateLabels(many))
}
