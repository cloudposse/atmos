// Package castplan defines declarative recordings independently of their language binding.
package castplan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// MarkerPrefix distinguishes recording metadata from legacy screenshot paths.
const MarkerPrefix = "atmos:cast:v1:"

// Plan is an ordered recording declaration. Configuration uses existing cast step fields.
type Plan struct {
	// Replacements normalize machine-specific output; source paths are not part of the portable fingerprint.
	Replacements  map[string]string `json:"-"`
	Configuration map[string]any    `json:"configuration"`
	Setup         []Action          `json:"setup,omitempty"`
	Actions       []Action          `json:"actions"`
	Cleanup       []Action          `json:"cleanup,omitempty"`
}

// Action describes a command, registered step, presentation action, or nested group.
type Action struct {
	Kind     string         `json:"kind"`
	Fields   map[string]any `json:"fields,omitempty"`
	Children []Action       `json:"children,omitempty"`
	Expect   []Matcher      `json:"expect,omitempty"`
	Reject   []Matcher      `json:"reject,omitempty"`
	ExitCode int            `json:"exit_code,omitempty"`
}

// Matcher is either a literal substring or a regular expression.
type Matcher struct {
	Text  string `json:"text"`
	Regex bool   `json:"regex,omitempty"`
}

// Marker identifies a declaration boundary without exposing command text or environment values.
type Marker struct {
	Kind        string `json:"kind"`
	ID          string `json:"id,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	ExitCode    int    `json:"exit_code,omitempty"`
}

// Fingerprint binds saved evidence to the complete declaration, including hidden actions.
func (p *Plan) Fingerprint() (string, error) {
	defer perf.Track(nil, "castplan.Plan.Fingerprint")()
	b, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("%w: cast declaration: %w", errUtils.ErrAutomation, err)
	}
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:]), nil
}

// Encode formats a metadata marker; all members are primitive JSON values.
func (m Marker) Encode() string {
	defer perf.Track(nil, "castplan.Marker.Encode")()

	b, _ := json.Marshal(m)
	return MarkerPrefix + string(b)
}

// Matches tests one assertion against normalized terminal text.
func (m Matcher) Matches(text string) bool {
	defer perf.Track(nil, "castplan.Matcher.Matches")()

	if !m.Regex {
		return strings.Contains(text, m.Text)
	}
	re, err := regexp.Compile(m.Text)
	return err == nil && re.MatchString(text)
}

// Check verifies output and the actual exit status for a command.
func (a *Action) Check(text string, exitCode int) error {
	defer perf.Track(nil, "castplan.Action.Check")()
	label := a.Fields["command"]
	if label == nil {
		label = a.Kind
	}
	if exitCode != a.ExitCode {
		return fmt.Errorf("%w: cast %v: exit status %d, expected %d", errUtils.ErrAutomation, label, exitCode, a.ExitCode)
	}
	for _, matcher := range a.Expect {
		if !matcher.Matches(text) {
			return fmt.Errorf("%w: cast %v: missing %q", errUtils.ErrAutomation, label, matcher.Text)
		}
	}
	for _, matcher := range a.Reject {
		if matcher.Matches(text) {
			return fmt.Errorf("%w: cast %v: rejected %q", errUtils.ErrAutomation, label, matcher.Text)
		}
	}
	return nil
}
