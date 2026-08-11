// Package say provides a cross-platform text-to-speech abstraction.
//
// It mirrors the pkg/browser package: a small interface (Speaker) backed by
// per-OS detection of a TTS engine (macOS `say`, Linux `spd-say`/`espeak`,
// Windows PowerShell System.Speech), with a CommandRunner seam for testing.
//
// When no engine is available — or the process is running in CI or a test —
// New returns a Speaker that invokes a caller-supplied fallback instead of
// emitting audio, so callers degrade gracefully on headless machines.
package say

import (
	"errors"
	"os"
	"os/exec"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/telemetry"
)

// Speaker speaks text aloud (text-to-speech).
type Speaker interface {
	// Speak speaks the given text, or invokes the configured fallback when
	// speech is unavailable.
	Speak(text string) error
}

// CommandRunner executes external commands. Abstracted for testing.
type CommandRunner interface {
	// Run executes the command and waits for it to complete.
	Run(name string, args ...string) error
	// Output executes the command, waits, and returns its standard output.
	Output(name string, args ...string) ([]byte, error)
}

// execRunner is the default CommandRunner backed by os/exec.
type execRunner struct{}

func (r *execRunner) Run(name string, args ...string) error {
	defer perf.Track(nil, "say.execRunner.Run")()

	return exec.Command(name, args...).Run()
}

func (r *execRunner) Output(name string, args ...string) ([]byte, error) {
	defer perf.Track(nil, "say.execRunner.Output")()

	return exec.Command(name, args...).Output()
}

// FallbackFunc renders text when speech is unavailable.
type FallbackFunc func(text string) error

// Option configures a Speaker.
type Option func(*config)

type config struct {
	voices   []string
	rate     string
	runner   CommandRunner
	fallback FallbackFunc
}

// WithCommandRunner sets a custom command runner (for testing).
func WithCommandRunner(runner CommandRunner) Option {
	defer perf.Track(nil, "say.WithCommandRunner")()

	return func(c *config) { c.runner = runner }
}

// WithVoices sets the ordered list of candidate voices. The first voice
// actually installed on the host is used (CSS font-family style); if none
// match, the backend's default voice is used.
func WithVoices(voices []string) Option {
	defer perf.Track(nil, "say.WithVoices")()

	return func(c *config) { c.voices = voices }
}

// WithRate sets the speech rate: slow, normal, or fast.
func WithRate(rate string) Option {
	defer perf.Track(nil, "say.WithRate")()

	return func(c *config) { c.rate = rate }
}

// WithFallback sets the function used to render text when speech is unavailable.
func WithFallback(fn FallbackFunc) Option {
	defer perf.Track(nil, "say.WithFallback")()

	return func(c *config) { c.fallback = fn }
}

// detectSayFn is the function used to detect a backend. Overridable in tests.
var detectSayFn = DetectSay

// shouldSpeakFn reports whether audio makes sense here. Overridable in tests.
var shouldSpeakFn = shouldSpeak

// New returns a Speaker configured with the given options.
// When a TTS backend is available and the environment supports audio, it
// returns a speaker that emits speech; otherwise it returns a speaker that
// invokes the fallback.
func New(opts ...Option) Speaker {
	defer perf.Track(nil, "say.New")()

	cfg := &config{
		runner:   &execRunner{},
		fallback: defaultFallback,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	if !shouldSpeakFn() {
		return &fallbackSpeaker{fallback: cfg.fallback}
	}

	info, err := detectSayFn()
	if err != nil {
		log.Debug("No text-to-speech backend available; using fallback", "error", err)
		return &fallbackSpeaker{fallback: cfg.fallback}
	}

	return &commandSpeaker{
		info:     info,
		voices:   cfg.voices,
		rate:     cfg.rate,
		runner:   cfg.runner,
		fallback: cfg.fallback,
	}
}

// shouldSpeak reports whether emitting audio makes sense in this environment.
// Audio does not require a TTY, but it never makes sense in CI or under test.
func shouldSpeak() bool {
	defer perf.Track(nil, "say.shouldSpeak")()

	if telemetry.IsCI() {
		return false
	}
	if os.Getenv("GO_TEST") == "1" { //nolint:forbidigo // GO_TEST is test infrastructure, not application config.
		return false
	}
	return true
}

// defaultFallback writes the text to stderr as plain output. Callers that want
// richer rendering (e.g. a Markdown blockquote) inject their own via WithFallback.
func defaultFallback(text string) error {
	_, err := os.Stderr.WriteString(text + "\n")
	return err
}

// fallbackSpeaker renders text without emitting audio.
type fallbackSpeaker struct {
	fallback FallbackFunc
}

func (s *fallbackSpeaker) Speak(text string) error {
	defer perf.Track(nil, "say.fallbackSpeaker.Speak")()

	return s.fallback(text)
}

// commandSpeaker emits speech via a detected backend, falling back on failure.
type commandSpeaker struct {
	info     *SayInfo
	voices   []string
	rate     string
	runner   CommandRunner
	fallback FallbackFunc
}

func (s *commandSpeaker) Speak(text string) error {
	defer perf.Track(nil, "say.commandSpeaker.Speak")()

	voice := s.resolveVoice()
	args := buildArgs(s.info.Backend, voice, s.rate, text)
	if err := s.runner.Run(s.info.Path, args...); err != nil {
		log.Debug("text-to-speech command failed; using fallback", "error", err)
		return s.fallback(text)
	}
	return nil
}

// resolveVoice picks the first requested voice installed on the host, returning
// the backend's exact installed name. Returns "" (backend default) when no
// voice is requested or none match. When enumeration is unsupported it falls
// back to passing the first requested name through verbatim; for any other
// enumeration failure it uses the backend default rather than risk forcing an
// invalid voice.
func (s *commandSpeaker) resolveVoice() string {
	if len(s.voices) == 0 {
		return ""
	}

	installed, err := ListVoices(s.info, s.runner)
	if err != nil {
		if errors.Is(err, errUtils.ErrVoiceListUnsupported) {
			log.Debug("Voice enumeration unsupported; passing first requested voice through", "error", err)
			return s.voices[0]
		}
		log.Debug("Voice enumeration failed; using backend default voice", "error", err)
		return ""
	}

	return matchVoice(installed, s.voices)
}
