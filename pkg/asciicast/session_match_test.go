package asciicast

import (
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/castplan"
)

func TestScopedWaitPreservesFollowingPrompts(t *testing.T) {
	for name, input := range map[string]string{
		"plain":     "Name:\nEmail:\n",
		"color":     "\x1b[32mName:\x1b[0m\n\x1b[31mEmail:\x1b[0m\n",
		"unicode":   "名前 Name:\n📧 Email:\n",
		"hyperlink": "\x1b]8;;https://example.com\aName:\x1b]8;;\a\nEmail:\n",
	} {
		t.Run(name, func(t *testing.T) {
			for _, useRegex := range []bool{false, true} {
				state := &sessionState{scopedWaits: true}
				state.output.WriteString(input)
				literal := "Name:"
				var expression *regexp.Regexp
				if useRegex {
					literal = ""
					expression = regexp.MustCompile(`N[a-z]+:`)
				}
				require.True(t, outputMatches(state, literal, expression))
				require.Equal(t, strings.SplitN(ansi.Strip(input), "Name:", 2)[1], ansi.Strip(state.output.String()[state.waitOffset:]))
				opts := &SessionOptions{ScopedWaits: true, Actions: []SessionAction{{Type: "wait"}}, Observe: func(_ int, _ *SessionAction) error { return nil }}
				require.NoError(t, observeSessionAction(state, opts, 0))
				require.True(t, outputMatches(state, "Email:", nil))
				require.False(t, outputMatches(state, "Email:", nil))
			}
		})
	}
}

func TestRawTerminalOffset(t *testing.T) {
	for _, input := range []string{"plain\r\n\t", "\x1b[32m色\x1b[0m🎉é", "\x1b]0;title\aName:\n", "a\x1b[\n32mb"} {
		plain := ansi.Strip(input)
		for end := range plain {
			boundary := rawTerminalOffset(input, end)
			require.Equal(t, plain[:end], ansi.Strip(input[:boundary]))
		}
		require.Equal(t, plain, ansi.Strip(input[:rawTerminalOffset(input, len(plain))]))
	}
	require.Equal(t, 0, rawTerminalOffset("", 0))
	require.Equal(t, 3, rawTerminalOffset("abc", 100))
}

func TestValidationRetainsBufferedWaitOutput(t *testing.T) {
	plan := sessionPlan(
		castplan.Action{Kind: "wait", Fields: map[string]any{"regex": `N[a-z]+:`}},
		castplan.Action{Kind: "wait", Fields: map[string]any{"text": "Email:"}},
	)
	markers := []castplan.Marker{{Kind: "wait", ID: "0.0"}, {Kind: "wait", ID: "0.1"}}
	events := sessionEvents(t, plan, map[string]string{"0.0": "\x1b[32mName:\x1b[0m\nEmail:\n"}, markers...)
	_, err := validateDeclarationEvents(events, plan)
	require.NoError(t, err)

	plan.Actions[0].Children[1].Fields["text"] = "Name:"
	events = sessionEvents(t, plan, map[string]string{"0.0": "Name:\nEmail:\n"}, markers...)
	_, err = validateDeclarationEvents(events, plan)
	require.ErrorContains(t, err, `missing "Name:"`)
}
