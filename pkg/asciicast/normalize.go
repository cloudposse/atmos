package asciicast

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

type normalizationEvent struct {
	fields []json.RawMessage
	stream string
	data   string
}

// NormalizeRecording replaces literal output values in a staged recording, including
// matches split across events. It retains event ordering, timing, and metadata.
func NormalizeRecording(path string, replacements map[string]string) error {
	defer perf.Track(nil, "asciicast.NormalizeRecording")()

	if len(replacements) == 0 {
		return nil
	}
	lines, err := readRecordingLines(path)
	if err != nil {
		return err
	}
	events, err := normalizationEvents(lines[1:])
	if err != nil {
		return err
	}
	keys, err := replacementKeys(replacements)
	if err != nil {
		return err
	}
	normalizeEventGroups(events, keys, replacements)
	return writeNormalizedRecording(path, lines[0], events)
}

func readRecordingLines(path string) ([][]byte, error) {
	source, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = source.Close() }()
	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 0, initialScanBuffer), maxEventTokenSize)
	var lines [][]byte
	for scanner.Scan() {
		lines = append(lines, append([]byte(nil), scanner.Bytes()...))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(lines) == 0 {
		return nil, ErrEmptyCastFile
	}
	return lines, nil
}

func normalizationEvents(lines [][]byte) ([]normalizationEvent, error) {
	var events []normalizationEvent
	for _, line := range lines {
		fields, event, err := parseDeclarationEvent(line)
		if err != nil {
			return nil, err
		}
		events = append(events, normalizationEvent{fields: fields, stream: event.Stream, data: event.Data})
	}
	return events, nil
}

func replacementKeys(replacements map[string]string) ([]string, error) {
	keys := make([]string, 0, len(replacements))
	for key := range replacements {
		if key == "" {
			return nil, declarationError("empty output replacement")
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) == len(keys[j]) {
			return keys[i] < keys[j]
		}
		return len(keys[i]) > len(keys[j])
	})
	return keys, nil
}

func normalizeEventGroups(events []normalizationEvent, keys []string, replacements map[string]string) {
	for start := 0; start < len(events); {
		end := start
		for end < len(events) && isRecordingOutput(events[end].stream) {
			end++
		}
		if end == start {
			start++
			continue
		}
		normalizeOutputEvents(events[start:end], keys, replacements)
		start = end
	}
}

func writeNormalizedRecording(path string, header []byte, events []normalizationEvent) error {
	var output strings.Builder
	output.Write(header)
	output.WriteByte('\n')
	for _, event := range events {
		data, err := json.Marshal(event.data)
		if err != nil {
			return err
		}
		event.fields[2] = data
		line, err := json.Marshal(event.fields)
		if err != nil {
			return err
		}
		output.Write(line)
		output.WriteByte('\n')
	}
	if err := os.WriteFile(path, []byte(output.String()), castFilePerm); err != nil {
		return fmt.Errorf("normalize staged recording: %w", err)
	}
	return nil
}

func normalizeOutputEvents(events []normalizationEvent, keys []string, replacements map[string]string) {
	var combined strings.Builder
	for _, event := range events {
		text := event.data
		combined.WriteString(text)
	}
	source := combined.String()
	position, end := 0, 0
	for index := range events {
		event := &events[index]
		text := event.data
		end += len(text)
		var output strings.Builder
		for position < end {
			matched := false
			for _, key := range keys {
				if strings.HasPrefix(source[position:], key) {
					output.WriteString(replacements[key])
					position += len(key)
					matched = true
					break
				}
			}
			if !matched {
				output.WriteByte(source[position])
				position++
			}
		}
		event.data = output.String()
	}
}
