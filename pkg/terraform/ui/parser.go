package ui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Parser reads and parses Terraform JSON streaming output.
type Parser struct {
	scanner *bufio.Scanner
}

// NewParser creates a new parser from an io.Reader.
func NewParser(r io.Reader) *Parser {
	scanner := bufio.NewScanner(r)
	// Increase buffer size for large JSON lines.
	const maxScanTokenSize = 1024 * 1024 // 1MB
	buf := make([]byte, maxScanTokenSize)
	scanner.Buffer(buf, maxScanTokenSize)
	return &Parser{
		scanner: scanner,
	}
}

// ParseResult represents a parsed message or error.
type ParseResult struct {
	Message any
	Raw     []byte
	Err     error
}

// Next reads and parses the next JSON message.
// Returns io.EOF when there are no more messages.
func (p *Parser) Next() (*ParseResult, error) {
	// Use iterative approach to skip empty lines, avoiding potential stack overflow.
	for {
		if !p.scanner.Scan() {
			if err := p.scanner.Err(); err != nil {
				return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
			}
			return nil, io.EOF
		}

		line := p.scanner.Bytes()
		if len(line) == 0 {
			// Skip empty lines and continue to the next iteration.
			continue
		}

		msg, err := p.parseMessage(line)
		return &ParseResult{
			Message: msg,
			Raw:     append([]byte{}, line...), // Copy to avoid scanner reuse.
			Err:     err,
		}, nil
	}
}

// parseMessage parses a JSON line into the appropriate message type.
func (p *Parser) parseMessage(line []byte) (any, error) {
	// First, parse to determine message type.
	var base BaseMessage
	if err := json.Unmarshal(line, &base); err != nil {
		// Not JSON - return as raw output.
		return nil, fmt.Errorf("%w: invalid JSON: %w", errUtils.ErrParseTerraformOutput, err)
	}

	// Parse into specific type based on message type.
	switch base.Type {
	case MessageTypeVersion:
		var msg VersionMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
		}
		return &msg, nil

	case MessageTypePlannedChange:
		var msg PlannedChangeMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
		}
		return &msg, nil

	case MessageTypeChangeSummary:
		var msg ChangeSummaryMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
		}
		return &msg, nil

	case MessageTypeApplyStart:
		var msg ApplyStartMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
		}
		return &msg, nil

	case MessageTypeApplyProgress:
		var msg ApplyProgressMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
		}
		return &msg, nil

	case MessageTypeApplyComplete:
		var msg ApplyCompleteMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
		}
		return &msg, nil

	case MessageTypeApplyErrored:
		var msg ApplyErroredMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
		}
		return &msg, nil

	case MessageTypeRefreshStart:
		var msg RefreshStartMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
		}
		return &msg, nil

	case MessageTypeRefreshComplete:
		var msg RefreshCompleteMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
		}
		return &msg, nil

	case MessageTypeDiagnostic:
		var msg DiagnosticMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
		}
		return &msg, nil

	case MessageTypeOutputs:
		var msg OutputsMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
		}
		return &msg, nil

	case MessageTypeInitOutput:
		var msg InitOutputMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
		}
		return &msg, nil

	case MessageTypeLog:
		var msg LogMessage
		if err := json.Unmarshal(line, &msg); err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
		}
		return &msg, nil

	default:
		// Unknown message type - return base message.
		return &base, nil
	}
}
