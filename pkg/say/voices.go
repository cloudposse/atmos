package say

import (
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// psListVoicesScript enumerates installed Windows voices, one name per line.
const psListVoicesScript = "Add-Type -AssemblyName System.Speech; " +
	"(New-Object System.Speech.Synthesis.SpeechSynthesizer).GetInstalledVoices() | " +
	"ForEach-Object { $_.VoiceInfo.Name }"

// ListVoices enumerates the voices installed for the detected backend.
// Returns ErrVoiceListUnsupported for backends that cannot enumerate voices
// (e.g. spd-say, whose voice model is module-based rather than named).
func ListVoices(info *SayInfo, runner CommandRunner) ([]string, error) {
	defer perf.Track(nil, "say.ListVoices")()

	switch info.Backend {
	case BackendMacSay:
		out, err := runner.Output(info.Path, "-v", "?")
		if err != nil {
			return nil, err
		}
		return parseMacVoices(out), nil
	case BackendEspeak:
		out, err := runner.Output(info.Path, "--voices")
		if err != nil {
			return nil, err
		}
		return parseEspeakVoices(out), nil
	case BackendPowerShell:
		out, err := runner.Output(info.Path, "-NoProfile", "-Command", psListVoicesScript)
		if err != nil {
			return nil, err
		}
		return parseLines(out), nil
	case BackendSpdSay:
		return nil, errUtils.ErrVoiceListUnsupported
	default:
		return nil, errUtils.ErrVoiceListUnsupported
	}
}

// matchVoice returns the first installed voice that satisfies any requested
// voice, in requested order. Matching is case-insensitive and accepts a
// substring so a short token (e.g. "Zira") matches a fully-qualified installed
// name (e.g. "Microsoft Zira Desktop"). Returns the installed name so callers
// pass the backend the exact value it expects. Returns "" when nothing matches.
func matchVoice(installed, requested []string) string {
	for _, want := range requested {
		wantLower := strings.ToLower(strings.TrimSpace(want))
		if wantLower == "" {
			continue
		}
		for _, have := range installed {
			haveLower := strings.ToLower(have)
			if haveLower == wantLower || strings.Contains(haveLower, wantLower) {
				return have
			}
		}
	}
	return ""
}

// parseMacVoices parses `say -v "?"` output. Each line is
// "<name...>  <locale>  # <sample>"; the name may contain spaces, so the name
// is everything before the trailing locale column.
func parseMacVoices(out []byte) []string {
	var voices []string
	for _, line := range strings.Split(string(out), "\n") {
		if i := strings.Index(line, "#"); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := strings.Join(fields[:len(fields)-1], " ")
		if name == "" {
			name = fields[0]
		}
		voices = append(voices, name)
	}
	return voices
}

// parseEspeakVoices parses `espeak --voices` output. The header row starts with
// "Pty"; the second column is the language identifier used with -v (e.g. en-us).
func parseEspeakVoices(out []byte) []string {
	var voices []string
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		if strings.EqualFold(fields[0], "Pty") {
			continue
		}
		voices = append(voices, fields[1])
	}
	return voices
}

// parseLines returns the non-empty, trimmed lines of out.
func parseLines(out []byte) []string {
	var lines []string
	for _, l := range strings.Split(string(out), "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}
