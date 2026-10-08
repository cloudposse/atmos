package shared

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRunOptions_NormalizesTags(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  []string
	}{
		{name: "env style comma separated string", value: "prod,tier-1", want: []string{"prod", "tier-1"}},
		{name: "comma separated with spaces is trimmed", value: "a, b", want: []string{"a", "b"}},
		{name: "repeated flag style slice", value: []string{" a ", "b"}, want: []string{"a", "b"}},
		{name: "slice element holding a comma list", value: []string{"x,y", "z"}, want: []string{"x", "y", "z"}},
		{name: "empty segments are dropped", value: "a,,b,", want: []string{"a", "b"}},
		{name: "empty value yields no tags", value: "", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			v.Set("tags", tt.value)

			opts, err := ParseRunOptions(v)
			require.NoError(t, err)
			assert.Equal(t, tt.want, opts.Tags)
		})
	}
}

// TestParseRunOptions_TagsFromBoundEnv reproduces the ATMOS_TAGS=prod,tier-1 case end to end through
// a Viper env binding: Viper's string-to-slice cast splits on whitespace, not commas.
func TestParseRunOptions_TagsFromBoundEnv(t *testing.T) {
	t.Setenv("ATMOS_TAGS", "prod,tier-1")

	v := viper.New()
	require.NoError(t, v.BindEnv("tags", "ATMOS_TAGS"))

	opts, err := ParseRunOptions(v)
	require.NoError(t, err)
	assert.Equal(t, []string{"prod", "tier-1"}, opts.Tags)
}
