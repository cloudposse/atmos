package manifest

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func unsupportedTagError(tag string) error {
	return fmt.Errorf("%w: '%s' found in file 'stack.yaml'. Supported tags are: !env", errUtils.ErrUnsupportedYamlTag, tag)
}

func TestUnsupportedTagHint(t *testing.T) {
	intrinsics := []string{
		"!Ref", "!Sub", "!GetAtt", "!Join", "!Select", "!Split", "!If", "!Equals", "!And", "!Or", "!Not",
		"!FindInMap", "!Base64", "!Cidr", "!GetAZs", "!ImportValue", "!Transform", "!Condition",
	}
	for _, tag := range intrinsics {
		t.Run(tag, func(t *testing.T) {
			hint := UnsupportedTagHint(unsupportedTagError(tag))

			require.NotEmpty(t, hint)
			assert.Contains(t, hint, "`"+tag+"`")
			assert.Contains(t, hint, "path:", "points at the path: alternative")
			assert.Contains(t, hint, "long form")
			assert.NotContains(t, hint, ".yaml.tmpl", "never suggests the unrelated templating fix")
		})
	}

	t.Run("long forms", func(t *testing.T) {
		for tag, want := range map[string]string{"!Ref": "Ref", "!Condition": "Condition", "!Sub": "Fn::Sub", "!GetAtt": "Fn::GetAtt"} {
			long, ok := ShortFormIntrinsicLongForm(tag)
			require.True(t, ok)
			assert.Equal(t, want, long)
			assert.Contains(t, UnsupportedTagHint(unsupportedTagError(tag)), "`"+want+":`")
		}
	})

	negatives := []struct {
		name string
		err  error
	}{
		{name: "other unsupported tag", err: unsupportedTagError("!envv")},
		{name: "case differs", err: unsupportedTagError("!ref")},
		{name: "intrinsic name without being an unsupported-tag error", err: errors.New("'!Ref' is fine")},
		{name: "nil", err: nil},
	}
	for _, tt := range negatives {
		t.Run(tt.name, func(t *testing.T) {
			assert.Empty(t, UnsupportedTagHint(tt.err))
		})
	}
}
