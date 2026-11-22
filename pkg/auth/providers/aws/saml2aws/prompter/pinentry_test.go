package prompter

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/providers/aws/saml2aws/mocks"
)

// pinentryCommand is the command the pinentry prompter sends for the "000000" pattern.
const pinentryCommand = "SETPROMPT Security token [000000]\nGETPIN\n"

// newPinentryRunner returns a PinentryRunner mock that expects exactly one run of the "000000" command and
// answers it by parsing the given raw pinentry output, as the real runner does.
func newPinentryRunner(t *testing.T, pinentryOutput string) *MockPinentryRunner {
	t.Helper()

	runner := NewMockPinentryRunner(gomock.NewController(t))
	runner.EXPECT().Run(pinentryCommand).DoAndReturn(func(string) (string, error) {
		return ParseResults(strings.NewReader(pinentryOutput))
	}).Times(1)
	return runner
}

func TestValidateAndSetPrompterShouldFailWithWrongInput(t *testing.T) {
	// backing up the current prompters for the other tests
	oldPrompter := ActivePrompter
	defer func() { ActivePrompter = oldPrompter }()

	errPrompts := []string{"abcde", "invalid", "prompt", "    ", "pinentryfake"}
	for _, errPrompt := range errPrompts {
		err := ValidateAndSetPrompter(errPrompt)
		assert.Error(t, err)
	}
}

func TestValidateAndSetPrompterShouldWorkWithInputForCli(t *testing.T) {
	// backing up the current prompters for the other tests
	oldPrompter := ActivePrompter
	defer func() { ActivePrompter = oldPrompter }()

	errPrompts := []string{"", "default", "survey"}
	for _, errPrompt := range errPrompts {
		err := ValidateAndSetPrompter(errPrompt)
		assert.NoError(t, err)
		assert.IsType(t, ActivePrompter, NewCli())
	}
}

// Regression: selecting a default alias after pinentry resets the active prompter, and an invalid
// value leaves the active prompter unchanged.
func TestValidateAndSetPrompterDefaultAliasesResetPinentry(t *testing.T) {
	oldPrompter := ActivePrompter
	defer func() { ActivePrompter = oldPrompter }()

	for _, alias := range []string{"", "default", "survey"} {
		require.NoError(t, ValidateAndSetPrompter("pinentry"))
		assert.IsType(t, &PinentryPrompter{}, ActivePrompter)

		require.NoError(t, ValidateAndSetPrompter(alias))
		assert.IsType(t, &CliPrompter{}, ActivePrompter)
	}

	require.NoError(t, ValidateAndSetPrompter("pinentry"))
	active := ActivePrompter
	require.Error(t, ValidateAndSetPrompter("invalid"))
	assert.IsType(t, &PinentryPrompter{}, ActivePrompter)
	// The very same instance stays configured, not a replacement of the same type.
	assert.Same(t, active, ActivePrompter)
}

// scannerErrReader returns data and then a read error.
type scannerErrReader struct{ done bool }

func (r *scannerErrReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, errors.New("read failed")
	}
	r.done = true
	return copy(p, "D 123456\n"), nil
}

// Regression: a reader failure after "D <code>" must not return the code as a successful result.
func TestParseResultsReturnsScannerError(t *testing.T) {
	output, err := ParseResults(&scannerErrReader{})
	require.Error(t, err)
	assert.Empty(t, output)
}

func TestValidateAndSetPrompterShouldWorkWithInputForPinentry(t *testing.T) {
	// backing up the current prompters for the other tests
	oldPrompter := ActivePrompter
	defer func() { ActivePrompter = oldPrompter }()

	errPrompts := []string{"pinentry", "pinentry-tty", "pinentry-mac", "pinentry-gnome3"}
	for _, errPrompt := range errPrompts {
		err := ValidateAndSetPrompter(errPrompt)
		assert.NoError(t, err)
		assert.IsType(t, ActivePrompter, &PinentryPrompter{})
	}
}

func TestChecksPinentryPrompterDefault(t *testing.T) {
	// Every prompt except RequestSecurityCode is delegated to the default prompter exactly once, with the
	// caller's arguments; the controller verifies the calls when the test ends.
	defaultPrompter := mocks.NewMockPrompter(gomock.NewController(t))
	defaultPrompter.EXPECT().Choose("random", []string{"1", "2"}).Return(0, nil).Times(1)
	defaultPrompter.EXPECT().ChooseWithDefault("random", "random", []string{"1", "2"}).Return("", nil).Times(1)
	defaultPrompter.EXPECT().String("random", "random").Return("", nil).Times(1)
	defaultPrompter.EXPECT().StringRequired("random").Return("", nil).Times(1)
	defaultPrompter.EXPECT().Password("random").Return("", nil).Times(1)
	defaultPrompter.EXPECT().Display("random").Times(1)
	p := &PinentryPrompter{DefaultPrompter: defaultPrompter}

	_, _ = p.Choose("random", []string{"1", "2"})
	_, _ = p.ChooseWithDefault("random", "random", []string{"1", "2"})
	_, _ = p.String("random", "random")
	_, _ = p.StringRequired("random")
	_, _ = p.Password("random")
	p.Display("random")
}

func TestChecksPinentryPrompterCallsPinentryForRequestSecurityCode(t *testing.T) {
	// The runner is called exactly once, with the pinentry command for the pattern.
	runner := NewMockPinentryRunner(gomock.NewController(t))
	runner.EXPECT().Run(pinentryCommand).Return("random_code", nil).Times(1)
	p := &PinentryPrompter{Runner: runner}
	pinentryOutput, err := p.RequestSecurityCode("000000")

	assert.NoError(t, err)
	assert.Equal(t, pinentryOutput, "random_code")
}

func TestChecksPinentryPrompterReturnsRightCodeGivenFakePinentryOutput(t *testing.T) {
	runner := newPinentryRunner(t, "OK This line should get ignored\nOK This line should too\nD 654321\nOK Final\n")
	p := &PinentryPrompter{Runner: runner}
	pinentryOutput, err := p.RequestSecurityCode("000000")

	assert.NoError(t, err)
	assert.Equal(t, pinentryOutput, "654321")
}

func TestChecksPinentryPrompterReturnsNoCodeGivenErroneousFakePinentryOutput(t *testing.T) {
	runner := newPinentryRunner(t, "OK This line should get ignored\nOK This line should too\nERR This is an error\nD 654321\nOK Final\n")
	p := &PinentryPrompter{Runner: runner}
	pinentryOutput, err := p.RequestSecurityCode("000000")

	assert.Error(t, err)
	assert.Equal(t, pinentryOutput, "")
}

// Regression: output without a completed GETPIN answer was returned as a code, or as an empty success.
func TestParseResultsRequiresCompletedAnswer(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   string
		wantIs error
	}{
		{name: "data and the final OK", input: "OK greeting\nOK\nD 424242\nOK\n", want: "424242"},
		{name: "data without the final OK", input: "OK greeting\nOK\nD 424242\n", wantIs: errPinentryIncomplete},
		{name: "greeting and SETPROMPT acknowledgment only", input: "OK Pleased to meet you\nOK\n", wantIs: errPinentryIncomplete},
		{name: "empty output", input: "", wantIs: errPinentryIncomplete},
		{name: "a later data line must be completed too", input: "D 111111\nOK\nD 222222\n", wantIs: errPinentryIncomplete},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseResults(strings.NewReader(tt.input))
			if tt.wantIs != nil {
				require.ErrorIs(t, err, tt.wantIs)
				assert.Empty(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// Regression: a parse failure left the child blocked on a full pipe, so Run never returned.
func TestRealPinentryRunnerStopsChildOnParseError(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	t.Setenv(fakePinentryEnv, "err-flood")

	type result struct {
		out string
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := NewRealPinentryRunner(exe).Run(expectedPinentryInput)
		done <- result{out, err}
	}()

	select {
	case res := <-done:
		require.Error(t, res.err)
		assert.Contains(t, res.err.Error(), "error while running pinentry: 1 failed")
		assert.Empty(t, res.out)
	case <-time.After(30 * time.Second):
		t.Fatal("Run did not return after a parse error")
	}
}

func TestParseOutputShouldThrowError(t *testing.T) {
	input := strings.NewReader("OK Ignore this line\nERR This is an error\nD This result should be ignored\nOK this is irrelevant\n")
	output, err := ParseResults(input)

	assert.Error(t, err)
	assert.Equal(t, output, "")
}

func TestParseOutputShouldReturnCorrectOutput(t *testing.T) {
	input := strings.NewReader("OK Ignore this line\nD THISISTHERETURN\nOK this is irrelevant\n")
	output, err := ParseResults(input)

	assert.NoError(t, err)
	assert.Equal(t, output, "THISISTHERETURN")
}
