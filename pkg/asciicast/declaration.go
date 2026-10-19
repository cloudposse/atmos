package asciicast

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/castplan"
	"github.com/cloudposse/atmos/pkg/perf"
)

type declarationCheckpoint struct {
	marker castplan.Marker
	action *castplan.Action
}

// ValidateDeclaration verifies saved recording evidence without executing or rendering anything.
// The returned text contains raw output for caller-supplied policies such as color checks.
func ValidateDeclaration(path string, plan *castplan.Plan) (string, error) {
	defer perf.Track(nil, "asciicast.ValidateDeclaration")()
	header, events, err := readDeclarationEvents(path)
	if err != nil {
		return "", err
	}
	if header.Version != 2 && header.Version != 3 {
		return "", declarationError("unsupported cast version %d", header.Version)
	}
	return validateDeclarationEvents(events, plan)
}

func readDeclarationEvents(path string) (Header, []Event, error) {
	lines, err := readRecordingLines(path)
	if err != nil {
		return Header{}, nil, err
	}
	var header Header
	if err := json.Unmarshal(lines[0], &header); err != nil {
		return Header{}, nil, declarationError("invalid cast header: %s", err)
	}
	var events []Event
	for _, line := range lines[1:] {
		// Retain the existing reader's support for comment lines.
		if len(line) > 0 && line[0] == '#' {
			continue
		}
		_, event, err := parseDeclarationEvent(line)
		if err != nil {
			return Header{}, nil, err
		}
		events = append(events, event)
	}
	return header, events, nil
}

// parseDeclarationEvent keeps raw fields so normalization preserves timestamp precision.
func parseDeclarationEvent(line []byte) ([]json.RawMessage, Event, error) {
	var fields []json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil {
		return nil, Event{}, declarationError("invalid event: %s", err)
	}
	if len(fields) != 3 {
		return nil, Event{}, declarationError("malformed event: expected three fields")
	}
	timestamp, err := declarationTimestamp(fields[0])
	if err != nil {
		return nil, Event{}, err
	}
	var stream, data *string
	if err := json.Unmarshal(fields[1], &stream); err != nil || stream == nil {
		return nil, Event{}, declarationError("event stream must be a string")
	}
	if err := json.Unmarshal(fields[2], &data); err != nil || data == nil {
		return nil, Event{}, declarationError("event data must be a string")
	}
	return fields, Event{Time: timestamp, Stream: *stream, Data: *data}, nil
}

func declarationTimestamp(raw json.RawMessage) (float64, error) {
	var timestamp *float64
	if err := json.Unmarshal(raw, &timestamp); err != nil || timestamp == nil {
		return 0, declarationError("invalid event timestamp")
	}
	if *timestamp < 0 || math.IsInf(*timestamp, 0) || math.IsNaN(*timestamp) {
		return 0, declarationError("event timestamp must be finite and nonnegative")
	}
	return *timestamp, nil
}

func isRecordingOutput(stream string) bool {
	return stream == "o" || stream == "e"
}

func validateDeclarationEvents(events []Event, plan *castplan.Plan) (string, error) {
	hash, err := plan.Fingerprint()
	if err != nil {
		return "", err
	}
	expected := []declarationCheckpoint{{marker: castplan.Marker{Kind: "plan", Fingerprint: hash}}}
	expected = append(expected, declarationCheckpoints(plan.Actions)...)
	expected = append(expected, declarationCheckpoint{marker: castplan.Marker{Kind: "complete", Fingerprint: hash}})
	var raw, span strings.Builder
	next := 0
	for _, event := range events {
		if isRecordingOutput(event.Stream) {
			if next == len(expected) {
				return "", declarationError("output follows completion marker")
			}
			raw.WriteString(event.Data)
			span.WriteString(event.Data)
		}
		if event.Stream != "m" || !strings.HasPrefix(event.Data, castplan.MarkerPrefix) {
			continue
		}
		if next >= len(expected) {
			return "", declarationError("unexpected metadata after completion")
		}
		if err := validateDeclarationCheckpoint(&expected[next], event.Data, span.String()); err != nil {
			return "", err
		}
		span.Reset()
		next++
	}
	if next != len(expected) {
		return "", declarationError("missing recording metadata or incomplete recording; regenerate the cast")
	}
	return raw.String(), nil
}

func validateDeclarationCheckpoint(checkpoint *declarationCheckpoint, data, output string) error {
	var marker castplan.Marker
	if err := json.Unmarshal([]byte(strings.TrimPrefix(data, castplan.MarkerPrefix)), &marker); err != nil {
		return declarationError("invalid metadata: %s", err)
	}
	if marker.Kind != checkpoint.marker.Kind || marker.ID != checkpoint.marker.ID || marker.Fingerprint != checkpoint.marker.Fingerprint {
		return declarationError("declaration or action ordering changed at %s; regenerate the cast", checkpoint.marker.ID)
	}
	if checkpoint.action != nil {
		return checkpoint.action.Check(ansi.Strip(output), marker.ExitCode)
	}
	if marker.ExitCode != 0 {
		return declarationError("unexpected exit status in %s marker", marker.Kind)
	}
	return nil
}

func declarationCheckpoints(actions []castplan.Action) []declarationCheckpoint {
	var result []declarationCheckpoint
	for i := range actions {
		action := &actions[i]
		id := fmt.Sprint(i)
		if action.Kind == "hidden" {
			continue
		}
		result = append(result, declarationCheckpoint{marker: castplan.Marker{Kind: "begin", ID: id}})
		if action.Kind == "session" {
			result = append(result, sessionCheckpoints(action, id)...)
		}
		end := declarationCheckpoint{marker: castplan.Marker{Kind: "end", ID: id}}
		if action.Kind == "run" {
			end.action = action
		}
		result = append(result, end)
	}
	return result
}

func sessionCheckpoints(session *castplan.Action, id string) []declarationCheckpoint {
	var result []declarationCheckpoint
	hidden := false
	for i := range session.Children {
		action := &session.Children[i]
		switch action.Kind {
		case "hide":
			hidden = true
		case "show":
			hidden = false
			result = append(result, declarationCheckpoint{marker: castplan.Marker{Kind: "show", ID: fmt.Sprintf("%s.%d", id, i)}})
		case "wait":
			checkpoint := declarationCheckpoint{marker: castplan.Marker{Kind: "wait", ID: fmt.Sprintf("%s.%d", id, i)}}
			if !hidden {
				matcher := castplan.Matcher{Text: fieldString(action.Fields, "text")}
				if expression := fieldString(action.Fields, "regex"); expression != "" {
					matcher = castplan.Matcher{Text: expression, Regex: true}
				}
				checkpoint.action = &castplan.Action{Kind: "terminal.wait", Expect: []castplan.Matcher{matcher}}
			}
			result = append(result, checkpoint)
		}
	}
	return result
}

func fieldString(fields map[string]any, key string) string {
	value, _ := fields[key].(string)
	return value
}

func declarationError(format string, args ...any) error {
	return fmt.Errorf("%w: %s", errUtils.ErrAutomation, fmt.Sprintf(format, args...))
}

// ValidateBeforeCommit checks a flushed temporary recording and commits only on success.
func (r *Recorder) ValidateBeforeCommit(validate func(string) error) error {
	defer perf.Track(nil, "asciicast.Recorder.ValidateBeforeCommit")()
	r.mu.Lock()
	err := r.writer.Flush()
	path := r.tempPath
	r.mu.Unlock()
	if err != nil {
		return err
	}
	if err := validate(path); err != nil {
		return err
	}
	return r.Close()
}
