package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Operation names recorded in the audit log.
const (
	OpList    = "list"
	OpSign    = "sign"
	OpConnect = "connect"
)

// Result values recorded in the audit log.
const (
	ResultOK      = "ok"
	ResultRefused = "refused"
	ResultError   = "error"
)

const auditFileMode os.FileMode = 0o600

// AuditEntry is one line of the audit log. It never holds payload bytes or signatures.
type AuditEntry struct {
	Time           time.Time `json:"time"`
	Identity       string    `json:"identity"`
	Operation      string    `json:"operation"`
	Classification Kind      `json:"classification,omitempty"`
	Namespace      string    `json:"namespace,omitempty"`
	User           string    `json:"user,omitempty"`
	Service        string    `json:"service,omitempty"`
	Fingerprint    string    `json:"fingerprint,omitempty"`
	Result         string    `json:"result"`
	// Reason says why a request was refused or failed, in words that carry no payload.
	Reason string `json:"reason,omitempty"`
	// ScreenLocked is true when the key store refused because the screen is locked.
	ScreenLocked bool `json:"screen_locked,omitempty"`
}

// AuditLog appends JSON lines to a file. A nil *AuditLog discards everything.
type AuditLog struct {
	mu   sync.Mutex
	file *os.File
	now  func() time.Time
}

// OpenAuditLog opens path for appending, creating it with mode 0600.
func OpenAuditLog(path string) (*AuditLog, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create audit log directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, auditFileMode)
	if err != nil {
		return nil, fmt.Errorf("open audit log: %w", err)
	}
	return &AuditLog{file: f, now: func() time.Time { return time.Now().UTC() }}, nil
}

// Record writes one entry. A zero Time is filled in. Write failures are returned so the daemon can
// log them, but they never block a request.
func (l *AuditLog) Record(e AuditEntry) error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if e.Time.IsZero() {
		e.Time = l.now()
	}
	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("encode audit entry: %w", err)
	}
	if _, err := l.file.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write audit entry: %w", err)
	}
	return nil
}

// Close closes the log file.
func (l *AuditLog) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.file.Close()
}
