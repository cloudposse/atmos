package claudecode

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/approval"
	log "github.com/cloudposse/atmos/pkg/logger"
)

const (
	// The waitDelay constant bounds how long Wait blocks on I/O after the process was killed.
	waitDelay = 2 * time.Second
)

// finishGrace is how long Claude gets to exit after it reported its final result and stdin was closed.
// It is a variable so tests can shorten it.
var finishGrace = 5 * time.Second

// deniedTool records the tool call that needed approval in a non-interactive environment.
type deniedTool struct {
	tool    string
	summary string
}

// session holds the state of one Claude Code run. Every method runs on the caller's goroutine,
// so progress callbacks and the approver never execute concurrently with the caller.
type session struct {
	approver approval.Approver
	tracker  *progressTracker
	// stdin is nil when the prompt was passed as plain text and no control protocol is in use.
	stdin io.Writer
	timer *pausableTimer

	result      []byte      // Raw final result line.
	interrupted bool        // The approver asked to stop the whole run.
	denied      *deniedTool // First tool that needed approval without a terminal.
	userDenied  map[string]struct{}
	fatal       error // Error returned by the approver.
}

// execClaude runs the claude CLI and returns the result text.
func (c *Client) execClaude(ctx context.Context, prompt, systemPrompt string) (string, error) {
	defer c.prepareMCPConfig()()
	args := c.buildArgs(systemPrompt)

	// runCtx is cancelled by the caller's ctx, by the (pausable) timeout, or when this function returns.
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	timer := newPausableTimer(c.timeout, cancel)
	defer timer.stop()

	cmd := exec.CommandContext(runCtx, c.binaryPath, args...) //nolint:gosec // Binary path is from user config or exec.LookPath.
	cmd.WaitDelay = waitDelay
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	sess := &session{approver: c.approver, tracker: newProgressTracker(c.progress), timer: timer}
	if c.approver != nil {
		stdin, err := cmd.StdinPipe()
		if err != nil {
			return "", execFailed(err, "")
		}
		sess.stdin = stdin
	} else {
		cmd.Stdin = strings.NewReader(prompt)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", execFailed(err, "")
	}
	if err := cmd.Start(); err != nil {
		return "", execFailed(err, "")
	}

	if c.approver != nil {
		sess.write(encodeUserMessage(prompt))
	}
	sess.consume(runCtx, stdout)

	// Closing stdin tells Claude no more input is coming. The grace timer kills a process that
	// reported its result but does not exit.
	if closer, ok := sess.stdin.(io.Closer); ok {
		_ = closer.Close()
	}
	grace := time.AfterFunc(finishGrace, cancel)
	_, _ = io.Copy(io.Discard, stdout)
	waitErr := cmd.Wait()
	grace.Stop()

	return c.finish(ctx, sess, waitErr, stderr.String())
}

// consume reads stream-json lines until the final result, the end of the stream, or a fatal error.
func (s *session) consume(ctx context.Context, r io.Reader) {
	reader := newLineReader(r)
	for {
		line, err := reader.next()
		if len(line) > 0 && s.handleLine(ctx, line) {
			return
		}
		if err != nil {
			if !isEOF(err) {
				log.Debug("Reading Claude Code output failed", "error", err)
			}
			return
		}
	}
}

// handleLine processes one stream line and reports whether the read loop should stop.
func (s *session) handleLine(ctx context.Context, line []byte) bool {
	msg, ok := parseStreamLine(line)
	if !ok {
		return false
	}

	switch msg.Type {
	case msgTypeAssistant, msgTypeUser:
		s.tracker.observe(msg)
	case msgTypeControlReq:
		s.handleControlRequest(ctx, msg)
		return s.fatal != nil
	case msgTypeControlCancel:
		log.Debug("Claude Code cancelled a control request", "request_id", msg.RequestID)
	case msgTypeResult:
		s.result = append([]byte(nil), line...)
		return true
	default:
		log.Debug("Ignoring Claude Code stream message", "type", msg.Type, "subtype", msg.Subtype)
	}
	return false
}

// handleControlRequest answers a permission request from Claude. Claude blocks until it gets the answer.
func (s *session) handleControlRequest(ctx context.Context, msg *streamMessage) {
	req := msg.Request
	if req == nil || req.Subtype != subtypeCanUseTool {
		subtype := ""
		if req != nil {
			subtype = req.Subtype
		}
		log.Debug("Rejecting unsupported Claude Code control request", "subtype", subtype)
		s.write(encodeControlError(msg.RequestID, "unsupported control request: "+subtype))
		return
	}

	// After an interrupt or a non-interactive denial the run is being torn down: refuse further
	// tool calls without bothering the approver again.
	if s.approver == nil || s.interrupted || s.denied != nil {
		s.write(encodeDeny(msg.RequestID, "", true))
		return
	}

	// Time spent waiting for a person must not count toward the run timeout.
	s.timer.pause()
	decision, err := s.approver.Approve(ctx, approval.Request{
		ToolName:  req.ToolName,
		ToolUseID: req.ToolUseID,
		Input:     req.Input,
	})
	s.timer.resume()

	if err != nil {
		// Deny with interrupt and stop reading. The caller then closes stdin, which lets Claude
		// receive the denial and exit; the grace timer kills it if it does not.
		s.write(encodeDeny(msg.RequestID, "Tool approval failed.", true))
		s.fatal = err
		return
	}
	s.applyDecision(msg.RequestID, req, decision)
}

// applyDecision writes the control response for an approver decision and records interrupts and denials.
func (s *session) applyDecision(requestID string, req *controlRequest, decision approval.Decision) {
	switch {
	case decision.Allow:
		input := req.Input
		if decision.UpdatedInput != nil {
			input = decision.UpdatedInput
		}
		s.write(encodeAllow(requestID, input))
	case decision.Interrupt:
		s.interrupted = true
		s.denied = nil
		s.write(encodeDeny(requestID, decision.Message, true))
	case decision.NonInteractive:
		s.denied = &deniedTool{tool: req.ToolName, summary: summarize(req.ToolName, req.Input)}
		// The model's prose is discarded in favor of an error, so stop the run instead of paying for more turns.
		s.write(encodeDeny(requestID, decision.Message, true))
	default:
		s.rememberUserDenial(req)
		s.write(encodeDeny(requestID, decision.Message, false))
	}
}

// rememberUserDenial records a tool call the approver refused. Claude lists such calls in the
// final permission_denials too, but they are the user's choice, not a missing approval.
func (s *session) rememberUserDenial(req *controlRequest) {
	if s.userDenied == nil {
		s.userDenied = make(map[string]struct{})
	}
	s.userDenied[denialKey(req.ToolUseID, req.ToolName)] = struct{}{}
}

// denialKey identifies a tool call by its id, or by tool name when Claude sent no id.
func denialKey(toolUseID, toolName string) string {
	if toolUseID != "" {
		return "id:" + toolUseID
	}
	return "name:" + toolName
}

// evaluateResult decodes the final result line, ignoring denials the user chose, and classifies it.
func (s *session) evaluateResult() (string, error) {
	var resp claudeResponse
	if err := json.Unmarshal(s.result, &resp); err != nil {
		return parseResponse(s.result)
	}
	remaining := resp.PermissionDenials[:0:0]
	for _, denial := range resp.PermissionDenials {
		_, byID := s.userDenied[denialKey(denial.ToolUseID, denial.ToolName)]
		_, byName := s.userDenied[denialKey("", denial.ToolName)]
		if byID || byName {
			continue
		}
		remaining = append(remaining, denial)
	}
	resp.PermissionDenials = remaining
	return evaluateResult(&resp)
}

// write sends an encoded line to Claude. Failures are logged because the read loop reports the real cause.
func (s *session) write(data []byte, encodeErr error) {
	if encodeErr != nil {
		log.Debug("Encoding Claude Code input failed", "error", encodeErr)
		return
	}
	if s.stdin == nil {
		return
	}
	if _, err := s.stdin.Write(data); err != nil {
		log.Debug("Writing to Claude Code stdin failed", "error", err)
	}
}

// finish classifies the outcome of a run once the process has exited.
func (c *Client) finish(ctx context.Context, sess *session, waitErr error, stderr string) (string, error) {
	switch {
	case sess.fatal != nil:
		return "", sess.fatal
	case sess.interrupted:
		return "", fmt.Errorf("%w: tool use was rejected, stopping the Claude Code run", errUtils.ErrUserAborted)
	case sess.denied != nil:
		return "", toolDeniedError(sess.denied.tool, sess.denied.summary)
	case sess.result != nil:
		return sess.evaluateResult()
	case sess.timer.didFire():
		return "", errUtils.Build(errUtils.ErrCLIProviderExecFailed).
			WithCause(context.DeadlineExceeded).
			WithContext("provider", ProviderName).
			WithContext("timeout", c.timeout.String()).
			WithExplanation("Claude Code did not finish before the timeout").
			WithHint("Raise `ai.timeout_seconds` in atmos.yaml").
			Err()
	case ctx.Err() != nil:
		return "", execFailed(ctx.Err(), stderr)
	case waitErr != nil:
		return "", execFailed(waitErr, stderr)
	default:
		return "", fmt.Errorf("%w: %s: no result message in output", errUtils.ErrCLIProviderParseResponse, ProviderName)
	}
}

// execFailed wraps err (and Claude's stderr when present) in ErrCLIProviderExecFailed.
func execFailed(err error, stderr string) error {
	if strings.TrimSpace(stderr) != "" {
		return fmt.Errorf("%w: %s: %s: %w", errUtils.ErrCLIProviderExecFailed, ProviderName, strings.TrimSpace(stderr), err)
	}
	return fmt.Errorf("%w: %s: %w", errUtils.ErrCLIProviderExecFailed, ProviderName, err)
}
