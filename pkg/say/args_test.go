package say

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeRate(t *testing.T) {
	assert.Equal(t, rateSlow, normalizeRate("slow"))
	assert.Equal(t, rateSlow, normalizeRate("  SLOW "))
	assert.Equal(t, rateFast, normalizeRate("Fast"))
	assert.Equal(t, rateNormal, normalizeRate("normal"))
	assert.Equal(t, rateNormal, normalizeRate(""))
	assert.Equal(t, rateNormal, normalizeRate("warp-speed"), "unknown rate falls back to normal")
}

func TestBuildArgs(t *testing.T) {
	tests := []struct {
		name    string
		backend Backend
		voice   string
		rate    string
		text    string
		want    []string
	}{
		{name: "mac voice and fast", backend: BackendMacSay, voice: "Samantha", rate: "fast", text: "done", want: []string{"-v", "Samantha", "-r", "220", "done"}},
		{name: "mac no voice normal", backend: BackendMacSay, voice: "", rate: "normal", text: "done", want: []string{"done"}},
		{name: "mac slow", backend: BackendMacSay, voice: "", rate: "slow", text: "done", want: []string{"-r", "150", "done"}},
		{name: "espeak voice slow", backend: BackendEspeak, voice: "en-us", rate: "slow", text: "hi", want: []string{"-v", "en-us", "-s", "130", "hi"}},
		{name: "spd-say ignores voice", backend: BackendSpdSay, voice: "Samantha", rate: "fast", text: "hi", want: []string{"-r", "30", "hi"}},
		{name: "spd-say normal", backend: BackendSpdSay, voice: "", rate: "normal", text: "hi", want: []string{"hi"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, buildArgs(tt.backend, tt.voice, tt.rate, tt.text))
		})
	}
}

func TestBuildArgsPowerShell(t *testing.T) {
	args := buildArgs(BackendPowerShell, "Microsoft Zira Desktop", "fast", "It's done")
	assert.Equal(t, []string{"-NoProfile", "-Command"}, args[:2])

	script := args[2]
	assert.Contains(t, script, "Add-Type -AssemblyName System.Speech")
	assert.Contains(t, script, "$s.SelectVoice('Microsoft Zira Desktop')")
	assert.Contains(t, script, "$s.Rate = 3")
	// Single quote in the text must be doubled for the PowerShell string literal.
	assert.Contains(t, script, "$s.Speak('It''s done')")
}

func TestPowerShellScriptOmitsDefaults(t *testing.T) {
	script := powerShellScript("", "normal", "hello")
	assert.NotContains(t, script, "SelectVoice")
	assert.NotContains(t, script, "$s.Rate")
	assert.True(t, strings.HasSuffix(script, "$s.Speak('hello')"))
}

func TestRateMappings(t *testing.T) {
	assert.Equal(t, "150", macRate("slow"))
	assert.Equal(t, "", macRate("normal"))
	assert.Equal(t, "220", macRate("fast"))

	assert.Equal(t, "130", espeakRate("slow"))
	assert.Equal(t, "", espeakRate("normal"))

	assert.Equal(t, "-30", spdSayRate("slow"))
	assert.Equal(t, "30", spdSayRate("fast"))
	assert.Equal(t, "", spdSayRate("normal"))

	assert.Equal(t, "-3", powerShellRate("slow"))
	assert.Equal(t, "3", powerShellRate("fast"))
	assert.Equal(t, "", powerShellRate("normal"))
}
