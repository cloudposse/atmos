package agent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

// ErrNoState means the daemon state file does not exist, so no daemon is recorded as running.
var ErrNoState = errors.New("no daemon state recorded")

// State is what a running daemon records about itself.
type State struct {
	PID     int       `json:"pid"`
	Version string    `json:"version"`
	Started time.Time `json:"started"`
}

// WriteState writes the state file atomically with mode 0600.
func WriteState(path string, s State) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("encode daemon state: %w", err)
	}
	return writeFileAtomic(path, append(data, '\n'), 0o600)
}

// ReadState reads the state file. A missing file is ErrNoState.
func ReadState(path string) (State, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return State{}, fmt.Errorf("%w: %s", ErrNoState, path)
		}
		return State{}, fmt.Errorf("read daemon state: %w", err)
	}
	var s State
	if err := json.Unmarshal(data, &s); err != nil {
		return State{}, fmt.Errorf("decode daemon state %s: %w", path, err)
	}
	return s, nil
}

// RemoveState deletes the state file, but only when it still records pid, so a daemon that is
// exiting never removes the record of the daemon that replaced it.
func RemoveState(path string, pid int) error {
	s, err := ReadState(path)
	if errors.Is(err, ErrNoState) {
		return nil
	}
	if err != nil {
		return err
	}
	if s.PID != pid {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove daemon state: %w", err)
	}
	return nil
}
