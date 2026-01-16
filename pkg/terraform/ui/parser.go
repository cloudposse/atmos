package ui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Parser reads and parses Terraform JSON streaming output.
type Parser struct {
	scanner *bufio.Scanner
}

// NewParser creates a new parser from an io.Reader.
func NewParser(r io.Reader) *Parser {
	defer perf.Track(nil, "terraform.ui.NewParser")()

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
	defer perf.Track(nil, "terraform.ui.Parser.Next")()

	// Use iterative approach to skip empty lines, avoiding potential stack overflow.
	for {
		if !p.scanner.Scan() {
			if err := p.scanner.Err(); err != nil {
				return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
			}
			return nil, io.EOF
		}

		line := p.scanner.Bytes()
		// Skip empty lines and whitespace-only lines.
		if len(line) == 0 || len(strings.TrimSpace(string(line))) == 0 {
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

// unmarshalMessage is a helper that unmarshals JSON into a message pointer.
func unmarshalMessage[T any](line []byte) (*T, error) {
	var msg T
	if err := json.Unmarshal(line, &msg); err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrParseTerraformOutput, err)
	}
	return &msg, nil
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
		return unmarshalMessage[VersionMessage](line)
	case MessageTypePlannedChange:
		return unmarshalMessage[PlannedChangeMessage](line)
	case MessageTypeChangeSummary:
		return unmarshalMessage[ChangeSummaryMessage](line)
	case MessageTypeApplyStart:
		return unmarshalMessage[ApplyStartMessage](line)
	case MessageTypeApplyProgress:
		return unmarshalMessage[ApplyProgressMessage](line)
	case MessageTypeApplyComplete:
		return unmarshalMessage[ApplyCompleteMessage](line)
	case MessageTypeApplyErrored:
		return unmarshalMessage[ApplyErroredMessage](line)
	case MessageTypeRefreshStart:
		return unmarshalMessage[RefreshStartMessage](line)
	case MessageTypeRefreshComplete:
		return unmarshalMessage[RefreshCompleteMessage](line)
	case MessageTypeDiagnostic:
		return unmarshalMessage[DiagnosticMessage](line)
	case MessageTypeOutputs:
		return unmarshalMessage[OutputsMessage](line)
	case MessageTypeInitOutput:
		return unmarshalMessage[InitOutputMessage](line)
	case MessageTypeLog:
		return unmarshalMessage[LogMessage](line)
	default:
		// Unknown message type - return base message.
		return &base, nil
	}
}
