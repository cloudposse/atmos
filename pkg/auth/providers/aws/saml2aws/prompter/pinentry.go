package prompter

// Atmos fork: the PinentryRunner mock is generated with mockgen.
//go:generate go run go.uber.org/mock/mockgen -source=$GOFILE -destination=mock_pinentry_test.go -package=$GOPACKAGE

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// errPinentryIncomplete is returned when pinentry ends its output without a completed GETPIN answer.
var errPinentryIncomplete = errors.New("pinentry did not return a complete code")

const (
	defaultPinentryDialog string = "Security token [%s]"
)

// PinentryRunner is the interface for pinentry to run itself
type PinentryRunner interface {
	Run(string) (string, error)
}

// RealPinentryRunner is the concrete implementation of PinentryRunner
type RealPinentryRunner struct {
	PinentryBin string
}

// PinentryPrompter is a concrete implementation of the Prompter interface.
// It uses the default Cli under the hood, except for RequestSecurityCode, where
// it uses any _pinentry_ binary to capture the security code.
// Its purpose is mainly to capture the TOTP code outside of the TTY, and thus
// making it possible to use TOTP with the credential process.
// https://github.com/Versent/saml2aws#using-saml2aws-as-credential-process
type PinentryPrompter struct {
	Runner          PinentryRunner
	DefaultPrompter Prompter
}

// NewPinentryPrompter is a factory for PinentryPrompter
func NewPinentryPrompter(bin string) *PinentryPrompter {
	return &PinentryPrompter{Runner: NewRealPinentryRunner(bin), DefaultPrompter: NewCli()}
}

// NewRealPinentryRunner is a factory for RealPinentryRunner
func NewRealPinentryRunner(bin string) *RealPinentryRunner {
	return &RealPinentryRunner{PinentryBin: bin}
}

// RequestSecurityCode for PinentryPrompter is creating a query for pinentry
// and sends it to the pinentry bin.
func (p *PinentryPrompter) RequestSecurityCode(pattern string) (string, error) {
	commandTemplate := "SETPROMPT %s\nGETPIN\n"
	prompt := fmt.Sprintf(defaultPinentryDialog, pattern)
	command := fmt.Sprintf(commandTemplate, prompt)
	output, err := p.Runner.Run(command)
	if err != nil {
		return "", err
	}
	return output, nil
}

// ChooseWithDefault is running the default CLI ChooseWithDefault
func (p *PinentryPrompter) ChooseWithDefault(prompt string, def string, choices []string) (string, error) {
	return p.DefaultPrompter.ChooseWithDefault(prompt, def, choices)
}

// Choose is running the default CLI Choose
func (p *PinentryPrompter) Choose(pr string, options []string) (int, error) {
	return p.DefaultPrompter.Choose(pr, options)
}

// StringRequired is runniner the default Cli StringRequired
func (p *PinentryPrompter) StringRequired(pr string) (string, error) {
	return p.DefaultPrompter.StringRequired(pr)
}

// String is runniner the default Cli String
func (p *PinentryPrompter) String(pr string, defaultValue string) (string, error) {
	return p.DefaultPrompter.String(pr, defaultValue)
}

// Password is runniner the default Cli Password
func (p *PinentryPrompter) Password(pr string) (string, error) {
	return p.DefaultPrompter.Password(pr)
}

// Display is runniner the default Cli Display
func (p *PinentryPrompter) Display(pr string) {
	p.DefaultPrompter.Display(pr)
}

// Run wraps a pinentry run. It sends the query to pinentry via stdin and
// reads its stdout to determine the user PIN.
// Pinentry uses an Assuan protocol
func (r *RealPinentryRunner) Run(command string) (output string, err error) {
	// Atmos fork: upstream ran cmd.Run() in a goroutine while parsing stdout here. Both
	// goroutines wrote the named `err` result (a data race), and cmd.Run() waits for the
	// process and closes the stdout pipe, which may happen before all output was read
	// (os/exec forbids calling Wait before reads from StdoutPipe complete). Start the
	// process, read its output, then Wait, sequentially.
	cmd := exec.Command(r.PinentryBin, "--ttyname", "/dev/tty") //nolint:gosec // Binary is the configured pinentry program; arguments are fixed.
	cmd.Stdin = strings.NewReader(command)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	output, parseErr := ParseResults(out)
	if parseErr != nil && !errors.Is(parseErr, errPinentryIncomplete) {
		// Atmos fork: ParseResults stopped reading, so a child that is still writing could block on a full
		// pipe and Wait would never return. Terminate it, close the pipe and reap it, keeping the parse error.
		_ = cmd.Process.Kill()
		_ = out.Close()
		_ = cmd.Wait()
		return "", parseErr
	}
	waitErr := cmd.Wait()
	if waitErr != nil {
		// A failing process is the more informative cause than an answer that was cut short.
		return "", waitErr
	}
	if parseErr != nil {
		return "", parseErr
	}
	return output, nil
}

// ParseResults parses the standard output of the pinentry command and determine the
// user input, or whether the program yielded any error
func ParseResults(pinEntryOutput io.Reader) (output string, err error) {
	scanner := bufio.NewScanner(pinEntryOutput)
	completed := false
	for scanner.Scan() {
		line := scanner.Text()
		// fmt.Println(line)
		if strings.HasPrefix(line, "D ") {
			output = line[2:]
			completed = false
		}
		// Atmos fork: the answer to GETPIN is complete only when the data line is followed by OK.
		if output != "" && (line == "OK" || strings.HasPrefix(line, "OK ")) {
			completed = true
		}
		if strings.HasPrefix(line, "ERR ") {
			return "", fmt.Errorf("error while running pinentry: %s", line[4:])
		}
	}

	// Atmos fork: upstream ignored scanner.Err(), so a read failure after "D <code>" returned the code as a success.
	if scanErr := scanner.Err(); scanErr != nil {
		return "", fmt.Errorf("error while reading pinentry output: %w", scanErr)
	}

	// Atmos fork: upstream returned whatever "D" line it had seen, so a clean exit without the final OK
	// returned a cut-off code, and a clean exit with no data at all returned an empty code as a success.
	if output == "" || !completed {
		return "", errPinentryIncomplete
	}

	return output, err
}
