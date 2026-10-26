package say

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestParseMacVoices(t *testing.T) {
	out := "Alex                en_US    # Most people recognize me by my voice.\n" +
		"Bad News            en_US    # The light you see at the end of the tunnel.\n" +
		"Samantha            en_US    # Hello, my name is Samantha.\n"

	voices := parseMacVoices([]byte(out))
	require.Len(t, voices, 3)
	assert.Equal(t, "Alex", voices[0])
	assert.Equal(t, "Bad News", voices[1], "multi-word voice names must be preserved")
	assert.Equal(t, "Samantha", voices[2])
}

func TestParseEspeakVoices(t *testing.T) {
	out := "Pty Language       Age/Gender VoiceName          File                 Other Languages\n" +
		" 5  af                 --/M   Afrikaans          gmw/af\n" +
		" 5  en-us              --/M   English (America)  gmw/en-US\n"

	voices := parseEspeakVoices([]byte(out))
	require.Len(t, voices, 2)
	assert.Equal(t, "af", voices[0], "espeak language code column is used for -v")
	assert.Equal(t, "en-us", voices[1])
}

func TestParseLines(t *testing.T) {
	out := "Microsoft David Desktop\n\n  Microsoft Zira Desktop  \n"

	lines := parseLines([]byte(out))
	require.Len(t, lines, 2)
	assert.Equal(t, "Microsoft David Desktop", lines[0])
	assert.Equal(t, "Microsoft Zira Desktop", lines[1])
}

func TestMatchVoice(t *testing.T) {
	tests := []struct {
		name      string
		installed []string
		requested []string
		want      string
	}{
		{name: "first available wins in requested order", installed: []string{"Alex", "Samantha"}, requested: []string{"Zira", "Samantha"}, want: "Samantha"},
		{name: "substring matches qualified name and returns installed value", installed: []string{"Microsoft Zira Desktop"}, requested: []string{"Samantha", "Zira"}, want: "Microsoft Zira Desktop"},
		{name: "case insensitive", installed: []string{"Samantha"}, requested: []string{"samantha"}, want: "Samantha"},
		{name: "no match returns empty", installed: []string{"Alex"}, requested: []string{"Daniel"}, want: ""},
		{name: "empty requested returns empty", installed: []string{"Alex"}, requested: nil, want: ""},
		{name: "blank tokens skipped", installed: []string{"Alex"}, requested: []string{"  ", "Alex"}, want: "Alex"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, matchVoice(tt.installed, tt.requested))
		})
	}
}

func TestListVoices(t *testing.T) {
	t.Run("mac queries say -v ?", func(t *testing.T) {
		runner := &mockRunner{outputBytes: []byte("Samantha            en_US    # hi\n")}
		voices, err := ListVoices(&SayInfo{Path: "/usr/bin/say", Backend: BackendMacSay}, runner)
		require.NoError(t, err)
		assert.Equal(t, []string{"Samantha"}, voices)
		require.Len(t, runner.outputCalls, 1)
		assert.Equal(t, "/usr/bin/say", runner.outputCalls[0].Name)
		assert.Equal(t, []string{"-v", "?"}, runner.outputCalls[0].Args)
	})

	t.Run("espeak queries --voices", func(t *testing.T) {
		runner := &mockRunner{outputBytes: []byte("Pty Language Age/Gender VoiceName File\n 5 en-us --/M English gmw/en\n")}
		voices, err := ListVoices(&SayInfo{Path: "/usr/bin/espeak", Backend: BackendEspeak}, runner)
		require.NoError(t, err)
		assert.Equal(t, []string{"en-us"}, voices)
		require.Len(t, runner.outputCalls, 1)
		assert.Equal(t, []string{"--voices"}, runner.outputCalls[0].Args)
	})

	t.Run("powershell enumerates installed voices", func(t *testing.T) {
		runner := &mockRunner{outputBytes: []byte("Microsoft David Desktop\nMicrosoft Zira Desktop\n")}
		voices, err := ListVoices(&SayInfo{Path: "powershell", Backend: BackendPowerShell}, runner)
		require.NoError(t, err)
		assert.Equal(t, []string{"Microsoft David Desktop", "Microsoft Zira Desktop"}, voices)
		require.Len(t, runner.outputCalls, 1)
		assert.Equal(t, []string{"-NoProfile", "-Command", psListVoicesScript}, runner.outputCalls[0].Args)
	})

	t.Run("spd-say is unsupported", func(t *testing.T) {
		runner := &mockRunner{}
		voices, err := ListVoices(&SayInfo{Path: "/usr/bin/spd-say", Backend: BackendSpdSay}, runner)
		assert.ErrorIs(t, err, errUtils.ErrVoiceListUnsupported)
		assert.Nil(t, voices)
		assert.Empty(t, runner.outputCalls, "spd-say must not shell out to enumerate voices")
	})

	t.Run("output error propagates", func(t *testing.T) {
		runner := &mockRunner{outputErr: assert.AnError}
		_, err := ListVoices(&SayInfo{Path: "/usr/bin/say", Backend: BackendMacSay}, runner)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
