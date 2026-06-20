package say

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

// mockCall records a single command invocation.
type mockCall struct {
	Name string
	Args []string
}

// mockRunner records Run/Output invocations and returns canned results.
type mockRunner struct {
	mu          sync.Mutex
	runCalls    []mockCall
	runErr      error
	outputCalls []mockCall
	outputBytes []byte
	outputErr   error
}

func (m *mockRunner) Run(name string, args ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runCalls = append(m.runCalls, mockCall{Name: name, Args: append([]string(nil), args...)})
	return m.runErr
}

func (m *mockRunner) Output(name string, args ...string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outputCalls = append(m.outputCalls, mockCall{Name: name, Args: append([]string(nil), args...)})
	return m.outputBytes, m.outputErr
}

// withMockDetectSay replaces detectSayFn for the duration of the test.
func withMockDetectSay(t *testing.T, fn func() (*SayInfo, error)) {
	t.Helper()
	orig := detectSayFn
	detectSayFn = fn
	t.Cleanup(func() { detectSayFn = orig })
}

// withMockShouldSpeak replaces shouldSpeakFn for the duration of the test.
func withMockShouldSpeak(t *testing.T, v bool) {
	t.Helper()
	orig := shouldSpeakFn
	shouldSpeakFn = func() bool { return v }
	t.Cleanup(func() { shouldSpeakFn = orig })
}

func TestNewSelectsCommandSpeakerAndSpeaks(t *testing.T) {
	withMockShouldSpeak(t, true)
	withMockDetectSay(t, func() (*SayInfo, error) {
		return &SayInfo{Path: "/usr/bin/say", Backend: BackendMacSay}, nil
	})

	runner := &mockRunner{outputBytes: []byte("Alex en_US # hi\nSamantha en_US # hi\n")}
	sp := New(WithCommandRunner(runner), WithVoices([]string{"Zira", "Samantha"}), WithRate("fast"))
	require.IsType(t, &commandSpeaker{}, sp)

	require.NoError(t, sp.Speak("deploy complete"))

	require.Len(t, runner.runCalls, 1)
	assert.Equal(t, "/usr/bin/say", runner.runCalls[0].Name)
	// "Zira" is not installed; resolution picks "Samantha"; rate fast -> -r 220.
	assert.Equal(t, []string{"-v", "Samantha", "-r", "220", "deploy complete"}, runner.runCalls[0].Args)
}

func TestNewFallsBackWhenAudioUnsupported(t *testing.T) {
	withMockShouldSpeak(t, false)

	var captured string
	sp := New(WithFallback(func(text string) error {
		captured = text
		return nil
	}))
	require.IsType(t, &fallbackSpeaker{}, sp)

	require.NoError(t, sp.Speak("quiet please"))
	assert.Equal(t, "quiet please", captured)
}

func TestNewFallsBackWhenNoBackend(t *testing.T) {
	withMockShouldSpeak(t, true)
	withMockDetectSay(t, func() (*SayInfo, error) {
		return nil, errUtils.ErrSayNotFound
	})

	sp := New()
	assert.IsType(t, &fallbackSpeaker{}, sp)
}

func TestCommandSpeakerFallsBackOnRunError(t *testing.T) {
	runner := &mockRunner{runErr: assert.AnError}
	var captured string
	sp := &commandSpeaker{
		info:     &SayInfo{Path: "/usr/bin/say", Backend: BackendMacSay},
		runner:   runner,
		fallback: func(text string) error { captured = text; return nil },
	}

	require.NoError(t, sp.Speak("uh oh"))
	assert.Equal(t, "uh oh", captured, "a failed TTS command must invoke the fallback")
	require.Len(t, runner.runCalls, 1)
}

func TestCommandSpeakerResolveVoicePassthroughWhenUnsupported(t *testing.T) {
	// spd-say cannot enumerate voices; resolveVoice passes the first request through.
	runner := &mockRunner{}
	sp := &commandSpeaker{
		info:     &SayInfo{Path: "/usr/bin/spd-say", Backend: BackendSpdSay},
		voices:   []string{"Samantha"},
		runner:   runner,
		fallback: func(string) error { return nil },
	}

	assert.Equal(t, "Samantha", sp.resolveVoice())
	assert.Empty(t, runner.outputCalls, "unsupported enumeration must not shell out")
}

func TestCommandSpeakerResolveVoiceEmptyWhenNoneRequested(t *testing.T) {
	sp := &commandSpeaker{
		info:   &SayInfo{Path: "/usr/bin/say", Backend: BackendMacSay},
		runner: &mockRunner{},
	}
	assert.Empty(t, sp.resolveVoice())
}

func TestFallbackSpeaker(t *testing.T) {
	var captured string
	sp := &fallbackSpeaker{fallback: func(text string) error { captured = text; return nil }}
	require.NoError(t, sp.Speak("hello"))
	assert.Equal(t, "hello", captured)
}

func TestShouldSpeakFalseUnderTestEnv(t *testing.T) {
	t.Setenv("GO_TEST", "1")
	assert.False(t, shouldSpeak())
}

func TestShouldSpeakFalseInCI(t *testing.T) {
	t.Setenv("GO_TEST", "")
	t.Setenv("CI", "true")
	assert.False(t, shouldSpeak())
}
