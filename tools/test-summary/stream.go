package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
)

const (
	checkMark = "✔"
	xMark     = "✘"
	skipMark  = "⏭"
)

// StreamProcessor handles real-time test output with buffering.
type StreamProcessor struct {
	mu         sync.Mutex
	buffers    map[string][]string
	jsonWriter io.Writer
}

// StreamMode runs tests with real-time filtered output.
func StreamMode(testPackages []string, outputFile string, coverProfile string, testArgs string) error {
	// Build the go test command
	args := []string{"test", "-json"}

	// Add coverage if requested
	if coverProfile != "" {
		args = append(args, fmt.Sprintf("-coverprofile=%s", coverProfile))
		args = append(args, "-coverpkg=./...")
	}

	// Add verbose flag
	args = append(args, "-v")

	// Add timeout and other test arguments
	if testArgs != "" {
		// Parse testArgs string into individual arguments
		extraArgs := strings.Fields(testArgs)
		args = append(args, extraArgs...)
	}

	// Add packages to test
	args = append(args, testPackages...)

	// Create and run the command
	return runTestsWithStreaming(args, outputFile)
}

func runTestsWithStreaming(testArgs []string, outputFile string) error {
	// Create the command
	cmd := exec.Command("go", testArgs...)
	cmd.Stderr = os.Stderr // Pass through stderr

	// Get stdout pipe
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start go test: %w", err)
	}

	// Create JSON output file
	jsonFile, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer jsonFile.Close()

	// Create processor
	processor := &StreamProcessor{
		buffers:    make(map[string][]string),
		jsonWriter: jsonFile,
	}

	// Process the stream
	err = processor.processStream(stdout)

	// Wait for command to complete
	testErr := cmd.Wait()

	// Return processing error if any, otherwise test error
	if err != nil {
		return err
	}

	// Don't treat test failures as errors - that's expected
	if testErr != nil {
		if exitErr, ok := testErr.(*exec.ExitError); ok {
			// Exit code 1 means tests failed, which is normal
			if exitErr.ExitCode() == 1 {
				return nil
			}
		}
		return testErr
	}

	return nil
}

func (p *StreamProcessor) processStream(input io.Reader) error {
	scanner := bufio.NewScanner(input)

	for scanner.Scan() {
		line := scanner.Bytes()

		// Write to JSON file
		p.jsonWriter.Write(line)
		p.jsonWriter.Write([]byte("\n"))

		// Parse and process event
		var event TestEvent
		if err := json.Unmarshal(line, &event); err != nil {
			// Skip non-JSON lines
			continue
		}

		p.processEvent(&event)
	}

	return scanner.Err()
}

func (p *StreamProcessor) processEvent(event *TestEvent) {
	// Skip package-level events
	if event.Test == "" {
		return
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	switch event.Action {
	case "run":
		// Initialize buffer for this test
		p.buffers[event.Test] = []string{}

	case "output":
		// Buffer the output
		if p.buffers[event.Test] != nil {
			p.buffers[event.Test] = append(p.buffers[event.Test], event.Output)
		}

	case "pass":
		// Show success with actual test name
		fmt.Fprintf(os.Stderr, " %s %s (%.2fs)\n",
			checkMark, event.Test, event.Elapsed)
		// Clear buffer
		delete(p.buffers, event.Test)

	case "fail":
		// Show failure with actual test name
		fmt.Fprintf(os.Stderr, " %s %s (%.2fs)\n",
			xMark, event.Test, event.Elapsed)

		// Show buffered error output
		if output, exists := p.buffers[event.Test]; exists {
			for _, line := range output {
				// Filter to show only meaningful error lines
				if shouldShowErrorLine(line) {
					fmt.Fprint(os.Stderr, "    "+line)
				}
			}
		}
		delete(p.buffers, event.Test)

	case "skip":
		// Show skip with actual test name
		fmt.Fprintf(os.Stderr, " %s %s\n", skipMark, event.Test)
		delete(p.buffers, event.Test)
	}
}

// shouldShowErrorLine determines if a line contains useful error information.
func shouldShowErrorLine(line string) bool {
	trimmed := strings.TrimSpace(line)

	// Skip the RUN/PASS/FAIL status lines
	if strings.HasPrefix(trimmed, "=== RUN") ||
		strings.HasPrefix(trimmed, "=== PAUSE") ||
		strings.HasPrefix(trimmed, "=== CONT") {
		return false
	}

	// Skip the --- PASS/FAIL lines (we show our own summary)
	if strings.HasPrefix(trimmed, "--- PASS") ||
		strings.HasPrefix(trimmed, "--- FAIL") ||
		strings.HasPrefix(trimmed, "--- SKIP") {
		return false
	}

	// Show actual error messages
	if strings.Contains(line, "_test.go:") || // File:line references
		strings.Contains(line, "Error:") ||
		strings.Contains(line, "Error Trace:") ||
		strings.Contains(line, "Test:") ||
		strings.Contains(line, "Messages:") ||
		strings.Contains(line, "expected:") ||
		strings.Contains(line, "actual:") ||
		strings.Contains(line, "got:") ||
		strings.Contains(line, "want:") {
		return true
	}

	// Show assertion failures
	if strings.Contains(line, "assertion failed") ||
		strings.Contains(line, "should be") ||
		strings.Contains(line, "should have") ||
		strings.Contains(line, "expected") {
		return true
	}

	// Skip empty lines in error output
	if trimmed == "" {
		return false
	}

	// When in doubt, show it if it's indented (part of test output)
	return strings.HasPrefix(line, "    ") || strings.HasPrefix(line, "\t")
}