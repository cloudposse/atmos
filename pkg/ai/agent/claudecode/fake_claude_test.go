package claudecode

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Fake-binary gate env vars.
const (
	// The fakeScenarioEnv variable selects the scripted behavior of the fake `claude`.
	fakeScenarioEnv = "_ATMOS_CLAUDE_FAKE"
	// The fakeLogEnv variable names a file where the fake records argv, stdin lines, and MCP config checks.
	fakeLogEnv           = "_ATMOS_CLAUDE_FAKE_LOG"
	fakeDescendantDirEnv = "_ATMOS_CLAUDE_DESCENDANT_DIR"
)

// Fake scenarios.
const (
	scenarioPlain            = "plain"
	scenarioTools            = "tools"
	scenarioAsk              = "ask"
	scenarioAskTwo           = "ask_two"
	scenarioEmpty            = "empty"
	scenarioMaxTurns         = "max_turns"
	scenarioIsError          = "is_error"
	scenarioGarbage          = "garbage"
	scenarioHuge             = "huge"
	scenarioDenials          = "denials"
	scenarioHang             = "hang"
	scenarioResultThenHang   = "result_then_hang"
	scenarioCrash            = "crash"
	scenarioNoResult         = "no_result"
	scenarioDescendant       = "descendant"
	scenarioDescendantResult = "descendant_result"
	scenarioDescendantChild  = "descendant_child"
)

// hugeSize is the size of the oversized lines emitted by the "huge" scenario (above bufio's 64KB default).
const hugeSize = 200 * 1024

// fakeLogEntry is one line of the fake's record file.
type fakeLogEntry struct {
	Kind   string   `json:"kind"`             // "args", "stdin", "stdin_response", or "mcp".
	Args   []string `json:"args,omitempty"`   // For "args".
	Line   string   `json:"line,omitempty"`   // For "stdin" and "stdin_response".
	Path   string   `json:"path,omitempty"`   // For "mcp".
	Exists bool     `json:"exists,omitempty"` // For "mcp".
}

// fakeClaude is the state of one fake run.
type fakeClaude struct {
	log *os.File
	in  *bufio.Reader
}

// runFakeClaude plays the scripted scenario and returns the process exit code.
func runFakeClaude(scenario string) int {
	f := &fakeClaude{in: bufio.NewReaderSize(os.Stdin, 1<<20)}
	if path := os.Getenv(fakeLogEnv); path != "" {
		file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err == nil {
			f.log = file
			defer file.Close()
		}
	}

	args := os.Args[1:]
	f.record(&fakeLogEntry{Kind: "args", Args: args})
	for i, a := range args {
		if a == "--mcp-config" && i+1 < len(args) {
			_, err := os.Stat(args[i+1])
			f.record(&fakeLogEntry{Kind: "mcp", Path: args[i+1], Exists: err == nil})
		}
	}

	f.readPrompt(args)
	return f.play(scenario)
}

// readPrompt consumes the prompt: one JSON line with the stream-json protocol, otherwise all of stdin.
func (f *fakeClaude) readPrompt(args []string) {
	for _, a := range args {
		if a == "--input-format" {
			line, _ := f.in.ReadString('\n')
			f.record(&fakeLogEntry{Kind: "stdin", Line: strings.TrimSpace(line)})
			return
		}
	}
	all, _ := io.ReadAll(f.in)
	f.record(&fakeLogEntry{Kind: "stdin", Line: string(all)})
}

func (f *fakeClaude) record(e *fakeLogEntry) {
	if f.log == nil {
		return
	}
	data, _ := json.Marshal(e)
	_, _ = f.log.Write(append(data, '\n'))
}

// emit writes one NDJSON line to stdout.
func emit(v any) {
	data, _ := json.Marshal(v)
	_, _ = os.Stdout.Write(append(data, '\n'))
}

// emitRaw writes a raw line to stdout.
func emitRaw(s string) {
	_, _ = os.Stdout.WriteString(s + "\n")
}

type obj = map[string]any

func successResult(text string) obj {
	return obj{"type": "result", "subtype": "success", "is_error": false, "result": text, "num_turns": 2, "session_id": "s1", "permission_denials": []any{}}
}

func toolUse(id, name string, input obj) obj {
	return obj{"type": "assistant", "message": obj{"content": []any{obj{"type": "tool_use", "id": id, "name": name, "input": input}}}}
}

func toolResult(id string, isError bool) obj {
	return obj{"type": "user", "message": obj{"content": []any{obj{"type": "tool_result", "tool_use_id": id, "content": "ok", "is_error": isError}}}}
}

func fakeControlRequest(id, tool, toolUseID string, input obj) obj {
	return obj{"type": "control_request", "request_id": id, "request": obj{"subtype": "can_use_tool", "tool_name": tool, "tool_use_id": toolUseID, "input": input}}
}

// readControlResponse reads and decodes the control_response Atmos writes for a request.
func (f *fakeClaude) readControlResponse() (behavior string, interrupt bool, message, input string) {
	line, _ := f.in.ReadString('\n')
	f.record(&fakeLogEntry{Kind: "stdin_response", Line: strings.TrimSpace(line)})
	var resp struct {
		Response struct {
			RequestID string `json:"request_id"`
			Response  struct {
				Behavior     string         `json:"behavior"`
				Interrupt    bool           `json:"interrupt"`
				Message      string         `json:"message"`
				UpdatedInput map[string]any `json:"updatedInput"`
			} `json:"response"`
		} `json:"response"`
	}
	_ = json.Unmarshal([]byte(line), &resp)
	updated, _ := json.Marshal(resp.Response.Response.UpdatedInput)
	return resp.Response.Response.Behavior, resp.Response.Response.Interrupt, resp.Response.Response.Message, string(updated)
}

func interruptedResult() obj {
	return obj{
		"type": "result", "subtype": "error_during_execution", "is_error": true, "result": nil, "num_turns": 1,
		"permission_denials": []any{obj{"tool_name": "Bash", "tool_use_id": "toolu_1", "tool_input": obj{"command": "touch x"}}},
	}
}

// play runs the scenario.
func (f *fakeClaude) play(scenario string) int {
	emit(obj{"type": "system", "subtype": "init"})
	emit(obj{"type": "rate_limit_event"})

	switch scenario {
	case scenarioPlain:
		emit(obj{"type": "assistant", "message": obj{"content": []any{obj{"type": "text", "text": "thinking"}}}})
		emit(successResult("hello from claude"))
	case scenarioTools:
		emit(toolUse("toolu_a", "Bash", obj{"command": "atmos list stacks\nsecond line", "description": "d"}))
		emit(toolResult("toolu_a", false))
		emit(toolUse("toolu_b", "Read", obj{"file_path": "/tmp/a.txt"}))
		emit(toolResult("toolu_b", true))
		emit(successResult("tools done"))
	case scenarioAsk:
		return f.playAsk()
	case scenarioAskTwo:
		return f.playAskTwo()
	case scenarioEmpty:
		emit(successResult(""))
	case scenarioMaxTurns:
		emit(obj{
			"type": "result", "subtype": "error_max_turns", "is_error": true, "result": nil, "num_turns": 6,
			"errors": []any{"Reached maximum number of turns (5)"}, "permission_denials": []any{},
		})
		return 1
	case scenarioIsError:
		emit(obj{"type": "result", "subtype": "error", "is_error": true, "result": "Authentication expired"})
		return 1
	case scenarioGarbage:
		emitRaw("not json at all")
		emitRaw(`{"type":`)
		emitRaw(`[1,2,3]`)
		emitRaw(`{"no":"type"}`)
		emitRaw("")
		emitRaw("   ")
		emit(successResult("survived garbage"))
	case scenarioHuge:
		emit(obj{"type": "assistant", "message": obj{"content": []any{obj{"type": "text", "text": strings.Repeat("x", hugeSize)}}}})
		emit(successResult(strings.Repeat("y", hugeSize)))
	case scenarioDenials:
		emit(obj{
			"type": "result", "subtype": "success", "is_error": false, "result": "This needs your approval.", "num_turns": 2,
			"permission_denials": []any{obj{"tool_name": "Bash", "tool_use_id": "toolu_1", "tool_input": obj{"command": "touch x"}}},
		})
	case scenarioHang:
		time.Sleep(time.Minute)
	case scenarioResultThenHang:
		emit(successResult("done but lingering"))
		time.Sleep(time.Minute)
	case scenarioCrash:
		_, _ = os.Stderr.WriteString("boom: claude crashed")
		return 2
	case scenarioNoResult:
		return 0
	case scenarioDescendant, scenarioDescendantResult:
		return f.playDescendant(scenario)
	case scenarioDescendantChild:
		return playDescendantChild()
	}
	return 0
}

const descendantPoll = 10 * time.Millisecond

func (f *fakeClaude) playDescendant(scenario string) int {
	child := exec.Command(os.Args[0])
	child.Env = append(os.Environ(), fakeScenarioEnv+"="+scenarioDescendantChild, fakeLogEnv+"=")
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	if err := child.Start(); err != nil {
		return 1
	}
	dir := os.Getenv(fakeDescendantDirEnv)
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(descendantPoll) {
		if _, err := os.Stat(filepath.Join(dir, "ready")); err == nil {
			emit(toolUse("child-ready", "Bash", obj{"command": "child ready"}))
			if scenario == scenarioDescendantResult {
				emit(successResult("result with lingering child"))
			}
			time.Sleep(time.Minute)
			return 0
		}
	}
	return 1
}

func playDescendantChild() int {
	dir := os.Getenv(fakeDescendantDirEnv)
	if err := os.WriteFile(filepath.Join(dir, "ready"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return 1
	}
	for deadline := time.Now().Add(time.Minute); time.Now().Before(deadline); time.Sleep(descendantPoll) {
		if _, err := os.Stat(filepath.Join(dir, "trigger")); err == nil {
			if err := os.WriteFile(filepath.Join(dir, "side-effect"), []byte("child survived"), 0o600); err != nil {
				return 1
			}
			return 0
		}
	}
	return 0
}

// playAsk requests permission for one Bash call and echoes the answer into the final result text.
func (f *fakeClaude) playAsk() int {
	input := obj{"command": "touch x", "description": "d"}
	emit(toolUse("toolu_1", "Bash", input))
	emit(fakeControlRequest("req-1", "Bash", "toolu_1", input))
	behavior, interrupt, message, updated := f.readControlResponse()
	if interrupt {
		emit(interruptedResult())
		return 1
	}
	emit(toolResult("toolu_1", behavior != "allow"))
	result := successResult(fmt.Sprintf("behavior=%s interrupt=%v message=%q input=%s", behavior, interrupt, message, updated))
	if behavior != "allow" {
		// Like the real CLI, list the refused call in permission_denials even though the run succeeded.
		result["permission_denials"] = []any{obj{"tool_name": "Bash", "tool_use_id": "toolu_1", "tool_input": input}}
	}
	emit(result)
	return 0
}

// playAskTwo sends two permission requests back to back, as Claude does for parallel tool calls.
func (f *fakeClaude) playAskTwo() int {
	emit(fakeControlRequest("req-1", "Bash", "toolu_1", obj{"command": "touch one"}))
	emit(fakeControlRequest("req-2", "Bash", "toolu_2", obj{"command": "touch two"}))
	b1, i1, m1, _ := f.readControlResponse()
	b2, i2, m2, _ := f.readControlResponse()
	if i1 {
		emit(interruptedResult())
		return 1
	}
	emit(successResult(fmt.Sprintf("r1=%s/%v/%q;r2=%s/%v/%q", b1, i1, m1, b2, i2, m2)))
	return 0
}
