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
	// Cap large JSON lines (e.g. sizable resource states/diffs in big plans) well above
	// the default 64KB scanner limit, which trips bufio.ErrTooLong and aborts streaming.
	// The initial buffer starts small; bufio.Scanner grows it toward maxScanTokenSize
	// only as needed, so this doesn't pre-allocate 10MB per parser.
	const maxScanTokenSize = 10 * 1024 * 1024 // 10MB
	const initialScanBufSize = 64 * 1024      // 64KB
	buf := make([]byte, initialScanBufSize)
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

// messageParsers maps each known Terraform JSON message type to a function that
// unmarshals a line into the corresponding concrete message type. Using a lookup table
// instead of a type switch keeps parseMessage's complexity low as new message types are added.
var messageParsers = map[MessageType]func([]byte) (any, error){
	MessageTypeVersion:       func(line []byte) (any, error) { return unmarshalMessage[VersionMessage](line) },
	MessageTypePlannedChange: func(line []byte) (any, error) { return unmarshalMessage[PlannedChangeMessage](line) },
	MessageTypeChangeSummary: func(line []byte) (any, error) { return unmarshalMessage[ChangeSummaryMessage](line) },
	MessageTypeApplyStart:    func(line []byte) (any, error) { return unmarshalMessage[ApplyStartMessage](line) },
	MessageTypeApplyProgress: func(line []byte) (any, error) { return unmarshalMessage[ApplyProgressMessage](line) },
	MessageTypeApplyComplete: func(line []byte) (any, error) { return unmarshalMessage[ApplyCompleteMessage](line) },
	MessageTypeApplyErrored:  func(line []byte) (any, error) { return unmarshalMessage[ApplyErroredMessage](line) },
	MessageTypeRefreshStart:  func(line []byte) (any, error) { return unmarshalMessage[RefreshStartMessage](line) },
	MessageTypeRefreshComplete: func(line []byte) (any, error) {
		return unmarshalMessage[RefreshCompleteMessage](line)
	},
	MessageTypeDiagnostic: func(line []byte) (any, error) { return unmarshalMessage[DiagnosticMessage](line) },
	MessageTypeOutputs:    func(line []byte) (any, error) { return unmarshalMessage[OutputsMessage](line) },
	MessageTypeInitOutput: func(line []byte) (any, error) { return unmarshalMessage[InitOutputMessage](line) },
	MessageTypeLog:        func(line []byte) (any, error) { return unmarshalMessage[LogMessage](line) },
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
	if parseFn, ok := messageParsers[base.Type]; ok {
		return parseFn(line)
	}

	// Unknown message type - return base message.
	return &base, nil
}
