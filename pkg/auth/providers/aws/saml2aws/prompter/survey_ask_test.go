package prompter

import (
	"errors"
	"testing"

	survey "github.com/AlecAivazis/survey/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// answerWith replaces the survey backend so every prompt is answered with answer (or fails with err)
// and records the prompt that was asked. The answer is written through the response pointer the way
// survey does.
func answerWith(t *testing.T, answer string, err error) *survey.Prompt {
	t.Helper()

	asked := new(survey.Prompt)
	orig := askOne
	askOne = func(p survey.Prompt, response interface{}, _ ...survey.AskOpt) error {
		*asked = p
		if err != nil {
			return err
		}
		switch target := response.(type) {
		case *string:
			*target = answer
		case *survey.OptionAnswer:
			// A select reports the row that was chosen; the first row with this label is chosen here.
			target.Value = answer
			target.Index = -1
			if sel, ok := p.(*survey.Select); ok {
				for i, option := range sel.Options {
					if option == answer {
						target.Index = i
						break
					}
				}
			}
		default:
			require.Failf(t, "unexpected survey response", "%T", response)
		}
		return nil
	}
	t.Cleanup(func() { askOne = orig })
	return asked
}

var errPromptFailed = errors.New("prompt failed")

func TestCliPrompterSuccessfulAnswers(t *testing.T) {
	cli := NewCli()

	t.Run("RequestSecurityCode returns the entered token", func(t *testing.T) {
		asked := answerWith(t, "123456", nil)
		got, err := cli.RequestSecurityCode("000000")
		require.NoError(t, err)
		assert.Equal(t, "123456", got)
		input, ok := (*asked).(*survey.Input)
		require.True(t, ok)
		assert.Equal(t, "Security Token [000000]", input.Message)
	})

	t.Run("String returns the entered value and offers the default", func(t *testing.T) {
		asked := answerWith(t, "eu-west-1", nil)
		got, err := cli.String("Region", "us-east-1")
		require.NoError(t, err)
		assert.Equal(t, "eu-west-1", got)
		input, ok := (*asked).(*survey.Input)
		require.True(t, ok)
		assert.Equal(t, "Region", input.Message)
		assert.Equal(t, "us-east-1", input.Default)
	})

	t.Run("StringRequired returns the entered value", func(t *testing.T) {
		answerWith(t, "alice", nil)
		got, err := cli.StringRequired("Username")
		require.NoError(t, err)
		assert.Equal(t, "alice", got)
	})

	t.Run("Password returns the entered secret", func(t *testing.T) {
		asked := answerWith(t, "hunter2", nil)
		got, err := cli.Password("Password")
		require.NoError(t, err)
		assert.Equal(t, "hunter2", got)
		_, ok := (*asked).(*survey.Password)
		assert.True(t, ok)
	})
}

func TestCliPrompterChoose(t *testing.T) {
	cli := NewCli()
	options := []string{"a", "b", "c"}

	tests := []struct {
		name      string
		answer    string
		askErr    error
		wantIndex int
		wantErr   error
		badInput  bool
	}{
		{name: "first option", answer: "a", wantIndex: 0},
		{name: "last option", answer: "c", wantIndex: 2},
		{name: "answer outside the options is bad input", answer: "zzz", badInput: true},
		{name: "prompt failure is propagated", askErr: errPromptFailed, wantErr: errPromptFailed},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			answerWith(t, tc.answer, tc.askErr)
			got, err := cli.Choose("Pick", options)
			switch {
			case tc.wantErr != nil:
				require.ErrorIs(t, err, tc.wantErr)
			case tc.badInput:
				require.EqualError(t, err, "bad input")
			default:
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantIndex, got)
		})
	}
}

// Regression: selecting the second of two identically labeled rows returned the first index.
func TestCliPrompterChooseDuplicateLabels(t *testing.T) {
	options := []string{"push", "code", "push"}
	orig := askOne
	t.Cleanup(func() { askOne = orig })

	for _, wantIndex := range []int{0, 2} {
		askOne = func(_ survey.Prompt, response interface{}, _ ...survey.AskOpt) error {
			target, ok := response.(*survey.OptionAnswer)
			require.True(t, ok, "a select must read survey's own row index")
			*target = survey.OptionAnswer{Value: options[wantIndex], Index: wantIndex}
			return nil
		}
		got, err := NewCli().Choose("Pick", options)
		require.NoError(t, err)
		assert.Equal(t, wantIndex, got)
	}

	t.Run("an index that does not match the answer is bad input", func(t *testing.T) {
		askOne = func(_ survey.Prompt, response interface{}, _ ...survey.AskOpt) error {
			*(response.(*survey.OptionAnswer)) = survey.OptionAnswer{Value: "push", Index: 1}
			return nil
		}
		_, err := NewCli().Choose("Pick", options)
		require.EqualError(t, err, "bad input")
	})
}

// Regression: a panic inside survey (Ctrl+D with no matching options) crashed the process.
func TestCliPrompterSelectPanicBecomesError(t *testing.T) {
	orig := askOne
	t.Cleanup(func() { askOne = orig })
	askOne = func(survey.Prompt, interface{}, ...survey.AskOpt) error {
		var empty []string
		_ = empty[0]
		return nil
	}

	_, err := NewCli().Choose("Pick", []string{"a"})
	require.ErrorIs(t, err, errSelectAborted)
	got, err := NewCli().ChooseWithDefault("Pick", "a", []string{"a"})
	require.ErrorIs(t, err, errSelectAborted)
	assert.Empty(t, got)

	t.Run("ordinary prompt failures are not reported as aborted", func(t *testing.T) {
		askOne = func(survey.Prompt, interface{}, ...survey.AskOpt) error { return errPromptFailed }
		_, err := NewCli().Choose("Pick", []string{"a"})
		require.ErrorIs(t, err, errPromptFailed)
		assert.NotErrorIs(t, err, errSelectAborted)
	})
}

// Regression: the YubiKey prompt accepted a blank line and sent an empty passcode.
func TestCliPrompterPasswordRequiresInput(t *testing.T) {
	orig := askOne
	t.Cleanup(func() { askOne = orig })
	var validated error
	askOne = func(_ survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		cfg := &survey.AskOptions{}
		for _, opt := range opts {
			require.NoError(t, opt(cfg))
		}
		require.NotEmpty(t, cfg.Validators)
		validated = cfg.Validators[0]("")
		return nil
	}
	_, err := NewCli().Password("Press the button")
	require.NoError(t, err)
	assert.Error(t, validated, "a blank answer must be rejected")
}

func TestCliPrompterChooseWithDefault(t *testing.T) {
	cli := NewCli()
	options := []string{"a", "b", "c"}

	t.Run("returns the selected option and passes the default", func(t *testing.T) {
		asked := answerWith(t, "c", nil)
		got, err := cli.ChooseWithDefault("Pick", "b", options)
		require.NoError(t, err)
		assert.Equal(t, "c", got)
		sel, ok := (*asked).(*survey.Select)
		require.True(t, ok)
		assert.Equal(t, "b", sel.Default)
		assert.Equal(t, options, sel.Options)
	})

	t.Run("answer outside the options is bad input", func(t *testing.T) {
		answerWith(t, "zzz", nil)
		got, err := cli.ChooseWithDefault("Pick", "b", options)
		require.EqualError(t, err, "bad input")
		assert.Empty(t, got)
	})

	t.Run("prompt failure is propagated", func(t *testing.T) {
		answerWith(t, "", errPromptFailed)
		got, err := cli.ChooseWithDefault("Pick", "b", options)
		require.ErrorIs(t, err, errPromptFailed)
		assert.Empty(t, got)
	})
}
