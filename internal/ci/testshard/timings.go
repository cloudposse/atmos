package testshard

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

// Timings is a portable summary of Go's JSON events. Tests contains top-level
// elapsed times only; package elapsed times must not be added to test durations.
type Timings struct {
	Tests    map[string]map[string]float64 `json:"tests"`
	Packages map[string]float64            `json:"packages"`
}

// ReadTimings reads go test -json output, ignoring ordinary non-JSON chatter.
// Scanner/IO errors are returned so a truncated artifact cannot appear complete.
func ReadTimings(input io.Reader) (Timings, error) {
	result := Timings{Tests: map[string]map[string]float64{}, Packages: map[string]float64{}}
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		var event struct {
			Action  string
			Package string
			Test    string
			Elapsed float64
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			if strings.HasPrefix(strings.TrimSpace(string(scanner.Bytes())), "{") {
				return result, fmt.Errorf("%w: parse test timing event: %w", ErrPlan, err)
			}
			continue
		}
		if event.Package == "" {
			continue
		}
		if !slices.Contains([]string{"pass", "fail", "skip"}, event.Action) {
			continue
		}
		if event.Test == "" {
			result.Packages[event.Package] = event.Elapsed
			continue
		}
		if strings.Contains(event.Test, "/") {
			continue
		}
		if result.Tests[event.Package] == nil {
			result.Tests[event.Package] = map[string]float64{}
		}
		result.Tests[event.Package][event.Test] = event.Elapsed
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("read test timings: %w", err)
	}
	return result, nil
}
