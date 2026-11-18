package prompter

import (
	"fmt"
	"regexp"
)

// ActivePrompter is by default the survey cli prompter
var ActivePrompter Prompter = NewCli()

// Prompter handles prompting user for input
type Prompter interface {
	// Atmos fork: the prompting methods return an error (for example when the user cancels with Ctrl+C
	// or no terminal is available) so callers can stop instead of continuing with a zero value.
	RequestSecurityCode(string) (string, error)
	ChooseWithDefault(string, string, []string) (string, error)
	Choose(string, []string) (int, error)
	StringRequired(string) (string, error)
	String(string, string) (string, error)
	Password(string) (string, error)
	Display(string)
}

// SetPrompter configure an aternate prompter to the default one
func SetPrompter(prmpt Prompter) {
	ActivePrompter = prmpt
}

// ValidateAndSetPrompter validates the user configuration and will create
// a concrete prompter based on this configuration
func ValidateAndSetPrompter(prmptCfg string) error {
	if prmptCfg == "" || prmptCfg == "survey" || prmptCfg == "default" {
		// Atmos fork: reset to the survey prompter so an earlier pinentry selection does not stay active.
		SetPrompter(NewCli())
		return nil
	}

	// all pinentry programs will start with `pinentry`
	re := regexp.MustCompile(`^pinentry(-.*)?$`)
	if re.MatchString(prmptCfg) {
		SetPrompter(NewPinentryPrompter(prmptCfg))
		return nil
	}

	return fmt.Errorf("prompter %s is not valid", prmptCfg)
}

// RequestSecurityCode request a security code to be entered by the user
func RequestSecurityCode(pattern string) (string, error) {
	return ActivePrompter.RequestSecurityCode(pattern)
}

// ChooseWithDefault given the choice return the option selected with a default
func ChooseWithDefault(pr string, defaultValue string, options []string) (string, error) {
	// ensure the default is not empty and avoid bad input error
	if defaultValue == "" {
		if len(options) > 0 {
			defaultValue = options[0]
		}
	}

	return ActivePrompter.ChooseWithDefault(pr, defaultValue, options)
}

// Choose given the choice return the option selected
func Choose(pr string, options []string) (int, error) {
	return ActivePrompter.Choose(pr, options)
}

// StringRequired prompt for string which is required
func StringRequired(pr string) (string, error) {
	return ActivePrompter.StringRequired(pr)
}

// String prompt for string which is required
func String(pr string, defaultValue string) (string, error) {
	return ActivePrompter.String(pr, defaultValue)
}

// Password prompt for password which is required
func Password(pr string) (string, error) {
	return ActivePrompter.Password(pr)
}

// Display prompt, no user input required
func Display(pr string) {
	ActivePrompter.Display(pr)
}
