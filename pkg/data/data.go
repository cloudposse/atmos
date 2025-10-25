package data

import (
	"encoding/json"
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/io"
)

var (
	globalIOContext io.Context
	ioMu            sync.RWMutex
)

// InitData initializes the global data writer with an I/O context.
// This should be called once at application startup (in root.go).
func InitData(ioCtx io.Context) {
	ioMu.Lock()
	defer ioMu.Unlock()
	globalIOContext = ioCtx
}

// getIOContext returns the global I/O context instance.
// Panics if not initialized (programming error, not runtime error).
func getIOContext() io.Context {
	ioMu.RLock()
	defer ioMu.RUnlock()

	if globalIOContext == nil {
		panic("data.InitData() must be called before using data package functions")
	}

	return globalIOContext
}

// Write writes content to the data channel (stdout).
func Write(content string) error {
	_, err := fmt.Fprint(getIOContext().Data(), content)
	return err
}

// Writef writes formatted content to the data channel (stdout).
func Writef(format string, a ...interface{}) error {
	_, err := fmt.Fprintf(getIOContext().Data(), format, a...)
	return err
}

// WriteJSON marshals v to JSON and writes to the data channel (stdout).
func WriteJSON(v interface{}) error {
	output, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return Write(string(output) + "\n")
}

// WriteYAML marshals v to YAML and writes to the data channel (stdout).
func WriteYAML(v interface{}) error {
	output, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return Write(string(output))
}
