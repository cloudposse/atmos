package starlark

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAutomationLoadAttributeSyntax(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`output = module.load("lib.star").value`,
		`output = module . load ("lib.star").value`,
		"output = (module. # comment mentioning .load\n load(\"lib.star\").value)",
		"output = module.\\\nload(\"lib.star\").value",
		"label = \"é\"; output = module.load(\"lib.star\").value",
		"# CRLF\r\noutput = module.load(\"lib.star\").value\r\n",
		"# CR\routput = module.load(\"lib.star\").value\r",
		"loader = module.load\noutput = loader(\"lib.star\").value",
		`alias = module
output = alias.load("lib.star").value`,
		`custom = struct(**{"load": lambda: 42, "Load": lambda: 7})
output = custom.load() + custom.Load() - 7`,
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			result, err := runSource(t, source, WithReadFile(func(string) ([]byte, error) {
				return []byte("value = 42"), nil
			}))
			require.NoError(t, err)
			assert.Equal(t, "42", result.Value)
		})
	}
}

func TestAutomationLoadAttributeLeavesTextUntouched(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`output = "module.load('lib.star')"`,
		`output = 'module.load("lib.star")'`,
		`output = r"escaped\".load"`,
		`output = b"module.load('lib.star')"`,
		`output = """module.load("lib.star")
.load
"""`,
		`output = '''module.load('lib.star')
.load
'''`,
		`# module.load("lib.star")
output = "done"`,
		`obj.load_extra; obj.loader; obj.loadé; obj.Load`,
		`load("lib.star", "value")`,
		`def load(): pass`,
		`obj.if(True)`,
		`output = "unterminated .load`,
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			adapted, positions := loadAttributes(source)
			assert.Equal(t, source, adapted)
			assert.Empty(t, positions)
		})
	}
}

func TestAutomationLoadAttributePositions(t *testing.T) {
	t.Parallel()
	adapted, positions := loadAttributes("# .load\r\n\"é\"; module.load('a')\r\nmodule.load('b')")
	assert.Equal(t, "# .load\r\n\"é\"; module.Load('a')\r\nmodule.Load('b')", adapted)
	assert.Equal(t, map[[2]int32]bool{{2, 13}: true, {3, 8}: true}, positions)
	_, err := runSource(t, "value = module.load\nvalue(1)")
	require.ErrorContains(t, err, "module.load")
	assert.Contains(t, errorDetails(err), "test.star:2:")
	assert.NotContains(t, errorDetails(err), "module.Load")
}

func TestAutomationLoadAttributeRejectsOtherReservedUses(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"load = 1", "def load(): pass", "module.if(True)"} {
		_, err := runSource(t, source)
		require.Error(t, err)
	}
}
