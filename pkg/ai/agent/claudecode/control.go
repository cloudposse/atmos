package claudecode

import (
	"encoding/json"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
)

const (
	// The subtypeCanUseTool constant is the control_request subtype Claude sends before running a tool
	// that requires permission.
	subtypeCanUseTool = "can_use_tool"

	// The behaviorAllow and behaviorDeny constants are the permission behaviors Claude understands.
	behaviorAllow = "allow"
	behaviorDeny  = "deny"

	// The defaultDenyMessage constant is shown to the model when a tool is denied without a reason.
	defaultDenyMessage = "The user denied permission to use this tool."
)

// userMessage is the stream-json line that carries the prompt to Claude.
type userMessage struct {
	Type    string      `json:"type"`
	Message userPayload `json:"message"`
}

type userPayload struct {
	Role    string      `json:"role"`
	Content []textBlock `json:"content"`
}

type textBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// controlResponse is the stream-json line that answers a control_request.
type controlResponse struct {
	Type     string          `json:"type"`
	Response controlEnvelope `json:"response"`
}

type controlEnvelope struct {
	Subtype   string           `json:"subtype"`
	RequestID string           `json:"request_id"`
	Response  *permissionReply `json:"response,omitempty"`
	Error     string           `json:"error,omitempty"`
}

// permissionReply is the inner payload of a can_use_tool answer.
type permissionReply struct {
	Behavior     string          `json:"behavior"`
	UpdatedInput *map[string]any `json:"updatedInput,omitempty"`
	Message      string          `json:"message,omitempty"`
	Interrupt    *bool           `json:"interrupt,omitempty"`
}

// encodeUserMessage builds the newline-terminated stream-json line carrying the prompt.
func encodeUserMessage(prompt string) ([]byte, error) {
	return marshalLine(userMessage{
		Type: msgTypeUser,
		Message: userPayload{
			Role:    "user",
			Content: []textBlock{{Type: "text", Text: prompt}},
		},
	})
}

// encodeAllow builds the response that lets a tool run with its original input.
func encodeAllow(requestID string, input map[string]any) ([]byte, error) {
	if input == nil {
		input = map[string]any{}
	}
	return marshalLine(controlResponse{
		Type: "control_response",
		Response: controlEnvelope{
			Subtype:   "success",
			RequestID: requestID,
			Response:  &permissionReply{Behavior: behaviorAllow, UpdatedInput: &input},
		},
	})
}

// encodeDeny builds the response that refuses a tool. With interrupt set, Claude stops the whole run.
func encodeDeny(requestID, message string, interrupt bool) ([]byte, error) {
	if message == "" {
		message = defaultDenyMessage
	}
	return marshalLine(controlResponse{
		Type: "control_response",
		Response: controlEnvelope{
			Subtype:   "success",
			RequestID: requestID,
			Response:  &permissionReply{Behavior: behaviorDeny, Message: message, Interrupt: &interrupt},
		},
	})
}

// encodeControlError builds the response for control requests Atmos does not support.
func encodeControlError(requestID, message string) ([]byte, error) {
	return marshalLine(controlResponse{
		Type: "control_response",
		Response: controlEnvelope{
			Subtype:   "error",
			RequestID: requestID,
			Error:     message,
		},
	})
}

// marshalLine encodes v as a single JSON line terminated by a newline.
func marshalLine(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", errUtils.ErrCLIProviderExecFailed, ProviderName, err)
	}
	return append(data, '\n'), nil
}
