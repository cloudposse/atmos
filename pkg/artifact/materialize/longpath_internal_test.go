package materialize

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtendedLengthPath(t *testing.T) {
	long := `C:\` + strings.Repeat(`a\`, 130) + "dir"
	longUNC := `\\server\share\` + strings.Repeat(`a\`, 130) + "dir"
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"short drive path is unchanged", `C:\tmp\out`, `C:\tmp\out`},
		{"short UNC path is unchanged", `\\server\share\out`, `\\server\share\out`},
		{"long drive path gets the prefix", long, `\\?\` + long},
		{"long UNC path gets the UNC prefix", longUNC, `\\?\UNC\` + longUNC[2:]},
		{"an extended-length path is unchanged", `\\?\` + long, `\\?\` + long},
		{"the boundary length is converted", `C:\` + strings.Repeat("b", maxPathChars-3), `\\?\C:\` + strings.Repeat("b", maxPathChars-3)},
		{"one below the boundary is unchanged", `C:\` + strings.Repeat("b", maxPathChars-4), `C:\` + strings.Repeat("b", maxPathChars-4)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extendedLengthPath(tt.in))
		})
	}
}
