package ui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/terminal"
)

func TestFormatterInfoAutolinks(t *testing.T) {
	for _, profile := range []terminal.ColorProfile{terminal.ColorNone, terminal.ColorTrue} {
		for _, tc := range []struct{ name, input, want string }{
			{"bare email", "hello a@b.com", "hello a@b.com"},
			{"angle email", "hello <a@b.com>", "hello a@b.com"},
			{"email after code", "`a@b.com` then a@b.com", "a@b.com then a@b.com"},
			{"email after named link", "[support](mailto:a@b.com) then a@b.com", "support mailto:a@b.com then a@b.com"},
			{"repeated email", "a@b.com then a@b.com", "a@b.com then a@b.com"},
			{"named email link", "hello [support](mailto:a@b.com)", "hello support mailto:a@b.com"},
			{"named web link", "see [docs](https://atmos.tools)", "see docs https://atmos.tools"},
			{"bare web link", "see https://atmos.tools", "see https://atmos.tools"},
			{"www web link", "see www.atmos.tools now", "see http://www.atmos.tools now"},
			{"angle web link", "see <https://atmos.tools> now", "see https://atmos.tools now"},
			{"tool reference", "run terraform@1.5.0", "run terraform@1.5.0"},
		} {
			t.Run(tc.name+"/"+strconv.Itoa(int(profile)), func(t *testing.T) {
				term := &mockTerminal{profile: profile}
				f := NewFormatter(createTestIOContext(), term)
				got := ansi.Strip(f.Info(tc.input))
				assert.Equal(t, tc.want, strings.TrimSpace(strings.TrimPrefix(got, "▶")))
			})
		}
	}
}
