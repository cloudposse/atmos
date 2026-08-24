package say

import (
	"fmt"
	"strings"
)

// Speech rate tokens.
const (
	rateSlow   = "slow"
	rateNormal = "normal"
	rateFast   = "fast"
)

// normalizeRate maps any input to a known rate token, defaulting to normal.
func normalizeRate(rate string) string {
	switch strings.ToLower(strings.TrimSpace(rate)) {
	case rateSlow:
		return rateSlow
	case rateFast:
		return rateFast
	default:
		return rateNormal
	}
}

// buildArgs assembles the command-line arguments for the given backend. The
// resolved voice may be empty (use backend default); a normal rate omits the
// rate flag so each backend keeps its own default cadence.
func buildArgs(backend Backend, voice, rate, text string) []string {
	switch backend {
	case BackendMacSay:
		return macArgs(voice, rate, text)
	case BackendEspeak:
		return espeakArgs(voice, rate, text)
	case BackendSpdSay:
		return spdSayArgs(rate, text)
	case BackendPowerShell:
		return []string{"-NoProfile", "-Command", powerShellScript(voice, rate, text)}
	default:
		return []string{text}
	}
}

func macArgs(voice, rate, text string) []string {
	var args []string
	if voice != "" {
		args = append(args, "-v", voice)
	}
	if wpm := macRate(rate); wpm != "" {
		args = append(args, "-r", wpm)
	}
	return append(args, text)
}

func espeakArgs(voice, rate, text string) []string {
	var args []string
	if voice != "" {
		args = append(args, "-v", voice)
	}
	if wpm := espeakRate(rate); wpm != "" {
		args = append(args, "-s", wpm)
	}
	return append(args, text)
}

// spdSayArgs ignores voice: spd-say selects voices by module/type, not by name.
func spdSayArgs(rate, text string) []string {
	var args []string
	if r := spdSayRate(rate); r != "" {
		args = append(args, "-r", r)
	}
	return append(args, text)
}

// macRate maps a rate token to a words-per-minute value ("" keeps the default).
func macRate(rate string) string {
	switch normalizeRate(rate) {
	case rateSlow:
		return "150"
	case rateFast:
		return "220"
	default:
		return ""
	}
}

// espeakRate maps a rate token to a words-per-minute value ("" keeps the default).
func espeakRate(rate string) string {
	switch normalizeRate(rate) {
	case rateSlow:
		return "130"
	case rateFast:
		return "220"
	default:
		return ""
	}
}

// spdSayRate maps a rate token to spd-say's -100..100 scale ("" keeps 0).
func spdSayRate(rate string) string {
	switch normalizeRate(rate) {
	case rateSlow:
		return "-30"
	case rateFast:
		return "30"
	default:
		return ""
	}
}

// powerShellRate maps a rate token to System.Speech's -10..10 scale ("" keeps 0).
func powerShellRate(rate string) string {
	switch normalizeRate(rate) {
	case rateSlow:
		return "-3"
	case rateFast:
		return "3"
	default:
		return ""
	}
}

// powerShellScript builds the System.Speech one-liner. Single quotes in the
// embedded voice and text are doubled so they are safe inside PowerShell
// single-quoted string literals.
func powerShellScript(voice, rate, text string) string {
	var b strings.Builder
	b.WriteString("Add-Type -AssemblyName System.Speech; ")
	b.WriteString("$s = New-Object System.Speech.Synthesis.SpeechSynthesizer; ")
	if voice != "" {
		fmt.Fprintf(&b, "$s.SelectVoice('%s'); ", psEscape(voice))
	}
	if r := powerShellRate(rate); r != "" {
		fmt.Fprintf(&b, "$s.Rate = %s; ", r)
	}
	fmt.Fprintf(&b, "$s.Speak('%s')", psEscape(text))
	return b.String()
}

// psEscape doubles single quotes for embedding in a PowerShell single-quoted string.
func psEscape(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}
