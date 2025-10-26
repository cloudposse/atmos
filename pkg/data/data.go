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

// InitWriter initializes the global data writer with an I/O context.
// This should be called once at application startup (in root.go).
func InitWriter(ioCtx io.Context) {
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
		panic("data.InitWriter() must be called before using data package functions")
	}

	return globalIOContext
}

// Write writes content to the data channel (stdout).
// Flow: data.Write() → io.Write(DataStream) → masking → stdout
func Write(content string) error {
	return getIOContext().Write(io.DataStream, content)
}

// Writef writes formatted content to the data channel (stdout).
// Flow: data.Writef() → io.Write(DataStream) → masking → stdout
func Writef(format string, a ...interface{}) error {
	return getIOContext().Write(io.DataStream, fmt.Sprintf(format, a...))
}

// WriteJSON marshals v to JSON and writes to the data channel (stdout).
// Flow: data.WriteJSON() → io.Write(DataStream) → masking → stdout
func WriteJSON(v interface{}) error {
	output, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return Write(string(output) + "\n")
}

// WriteYAML marshals v to YAML and writes to the data channel (stdout).
// Flow: data.WriteYAML() → io.Write(DataStream) → masking → stdout
func WriteYAML(v interface{}) error {
	output, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return Write(string(output))
}
