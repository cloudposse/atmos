package policy

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"

	errUtils "github.com/cloudposse/atmos/errors"
)

// ContextCheck is a pure, engine-independent operation over the shared context.
// Path is an RFC 6901 JSON pointer. Ranges include start and exclude end.
type ContextCheck struct {
	Kind       string   `json:"kind" yaml:"kind"`
	Path       string   `json:"path,omitempty" yaml:"path"`
	Constraint string   `json:"constraint,omitempty" yaml:"constraint"`
	Format     string   `json:"format,omitempty" yaml:"format"`
	Start      string   `json:"start,omitempty" yaml:"start"`
	End        string   `json:"end,omitempty" yaml:"end"`
	Timezone   string   `json:"timezone,omitempty" yaml:"timezone"`
	Weekdays   []string `json:"weekdays,omitempty" yaml:"weekdays"`
	StartTime  string   `json:"start_time,omitempty" yaml:"start_time"`
	EndTime    string   `json:"end_time,omitempty" yaml:"end_time"`
}

const maxContextChecks = 100

var checkName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func validateChecks(checks map[string]ContextCheck) error {
	if len(checks) > maxContextChecks {
		return fmt.Errorf("%w: at most 100 shared checks", errUtils.ErrPolicyInvalid)
	}
	for name := range checks {
		check := checks[name]
		if !checkName.MatchString(name) {
			return fmt.Errorf("%w: invalid check name", errUtils.ErrPolicyInvalid)
		}
		if err := check.validate(); err != nil {
			return fmt.Errorf("%w: check %s: %w", errUtils.ErrPolicyInvalid, name, err)
		}
	}
	return nil
}

func (c *ContextCheck) validate() error {
	if !validContextPath(c.Path) {
		return fmt.Errorf("%w: checks require an absolute JSON pointer outside checks", errUtils.ErrPolicyInvalid)
	}
	switch c.Kind {
	case "semver":
		return c.validateVersion()
	case "date":
		return c.validateDateCheck()
	case "schedule":
		if c.Constraint != "" || c.Format != "" {
			return errUtils.ErrPolicyInvalid
		}
		if err := c.validateDates(); err != nil {
			return err
		}
		return c.validateSchedule()
	default:
		return fmt.Errorf("%w: check kind must be semver, date, or schedule", errUtils.ErrPolicyInvalid)
	}
}

func (c *ContextCheck) validateVersion() error {
	if c.Path == "" || c.Constraint == "" || c.Format != "" || c.hasCalendarFields() {
		return errUtils.ErrPolicyInvalid
	}
	_, err := semver.NewConstraint(c.Constraint)
	return err
}

func (c *ContextCheck) validateDateCheck() error {
	if c.Path == "" || c.Constraint != "" || len(c.Weekdays) != 0 || c.StartTime != "" || c.EndTime != "" {
		return errUtils.ErrPolicyInvalid
	}
	return c.validateDates()
}

func (c *ContextCheck) hasCalendarFields() bool {
	return c.Start != "" || c.End != "" || c.Timezone != "" || len(c.Weekdays) > 0 || c.StartTime != "" || c.EndTime != ""
}

func (c *ContextCheck) location() (*time.Location, error) {
	if c.Timezone == "Local" {
		return nil, fmt.Errorf("%w: use an explicit IANA timezone or UTC", errUtils.ErrPolicyInvalid)
	}
	if c.Timezone == "" {
		return time.UTC, nil
	}
	return time.LoadLocation(c.Timezone)
}

func (c *ContextCheck) layout() (string, error) {
	switch c.Format {
	case "", "date-time":
		return time.RFC3339Nano, nil
	case "date":
		return time.DateOnly, nil
	case "time":
		return "15:04:05", nil
	default:
		return "", fmt.Errorf("%w: format must be date, date-time, or time", errUtils.ErrPolicyInvalid)
	}
}

func (c *ContextCheck) validateDates() error {
	location, err := c.location()
	if err != nil {
		return err
	}
	layout, err := c.layout()
	if err != nil {
		return err
	}
	var start, end time.Time
	if c.Start != "" {
		start, err = time.ParseInLocation(layout, c.Start, location)
		if err != nil {
			return err
		}
	}
	if c.End != "" {
		end, err = time.ParseInLocation(layout, c.End, location)
		if err != nil {
			return err
		}
	}
	if !start.IsZero() && !end.IsZero() && !start.Before(end) {
		return errUtils.ErrPolicyInvalid
	}
	return nil
}

func (c *ContextCheck) validateSchedule() error {
	for _, day := range c.Weekdays {
		if !slices.Contains([]string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}, day) {
			return errUtils.ErrPolicyInvalid
		}
	}
	if (c.StartTime == "") != (c.EndTime == "") {
		return errUtils.ErrPolicyInvalid
	}
	if c.StartTime != "" {
		start, err := time.Parse("15:04", c.StartTime)
		if err != nil {
			return err
		}
		end, err := time.Parse("15:04", c.EndTime)
		if err != nil {
			return err
		}
		if !start.Before(end) {
			return fmt.Errorf("%w: time window must end after start; split overnight windows", errUtils.ErrPolicyInvalid)
		}
	}
	return nil
}

func evaluateChecks(checks map[string]ContextCheck, activation map[string]any) (map[string]bool, error) {
	if err := validateChecks(checks); err != nil {
		return nil, err
	}
	result := make(map[string]bool, len(checks))
	for name := range checks {
		check := checks[name]
		path := check.Path
		if path == "" {
			path = "/time/now"
		}
		value, ok := contextPointer(activation, path)
		if !ok || value == nil {
			return nil, fmt.Errorf("%w: check %s requires unavailable data", errUtils.ErrPolicyInputUnavailable, name)
		}
		text, ok := value.(string)
		result[name] = ok && check.matches(text)
	}
	return result, nil
}

func (c *ContextCheck) matches(value string) bool {
	if c.Kind == "semver" {
		version, err := semver.StrictNewVersion(strings.TrimPrefix(value, "v"))
		if err != nil {
			return false
		}
		constraint, _ := semver.NewConstraint(c.Constraint)
		return constraint.Check(version)
	}
	location, _ := c.location()
	layout, _ := c.layout()
	instant, err := time.ParseInLocation(layout, value, location)
	if err != nil {
		return false
	}
	if c.Start != "" {
		start, _ := time.ParseInLocation(layout, c.Start, location)
		if instant.Before(start) {
			return false
		}
	}
	if c.End != "" {
		end, _ := time.ParseInLocation(layout, c.End, location)
		if !instant.Before(end) {
			return false
		}
	}
	return c.Kind != "schedule" || c.matchesSchedule(instant.In(location))
}

func (c *ContextCheck) matchesSchedule(local time.Time) bool {
	if len(c.Weekdays) > 0 && !slices.Contains(c.Weekdays, local.Weekday().String()) {
		return false
	}
	clock := local.Format("15:04")
	return c.StartTime == "" || (clock >= c.StartTime && clock < c.EndTime)
}

func contextPointer(value any, pointer string) (any, bool) {
	for _, part := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch current := value.(type) {
		case map[string]any:
			var exists bool
			value, exists = current[part]
			if !exists {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(current) || strconv.Itoa(index) != part {
				return nil, false
			}
			value = current[index]
		default:
			return nil, false
		}
	}
	return value, true
}

func extendActivation(activation map[string]any, evaluation Evaluation) (map[string]any, error) {
	// Direct adapter calls never collect live facts. The host engine owns collection.
	if evaluation.Request.Context != nil {
		raw, err := json.Marshal(evaluation.Request.Context)
		if err != nil {
			return nil, err
		}
		fields, err := decodeObject(raw)
		if err != nil {
			return nil, err
		}
		for key, value := range fields {
			activation[key] = value
		}
	} else {
		for _, key := range []string{"env", "atmos", "runtime", "time"} {
			activation[key] = nil
		}
	}
	activation["ci"] = normalizedCI(activation["ci"])
	checks := evaluation.Checks
	if checks == nil {
		checks = map[string]bool{}
	}
	activation["checks"] = checks
	// Normalize check maps through the same JSON boundary used by every adapter.
	raw, err := json.Marshal(activation)
	if err != nil {
		return nil, err
	}
	return decodeObject(raw)
}

func validContextPath(path string) bool {
	if path == "" {
		return true
	}
	if !strings.HasPrefix(path, "/") || path == "/checks" || strings.HasPrefix(path, "/checks/") {
		return false
	}
	for index := 0; index < len(path); index++ {
		if path[index] != '~' {
			continue
		}
		index++
		if index == len(path) || (path[index] != '0' && path[index] != '1') {
			return false
		}
	}
	return true
}
