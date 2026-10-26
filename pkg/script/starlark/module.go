package starlark

import (
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
)

const activeModuleKey = "atmos.starlark.activeModule"

// moduleContextKey carries the active initializer into parallel child threads.
type moduleContextKey struct{}

// moduleEntry is shared by both import APIs. Results are immutable after done closes.
// Dependencies are protected by moduleMu and counted because parallel children can
// wait on the same dependency on behalf of their initializing parent.
type moduleEntry struct {
	done      chan struct{}
	waits     map[*moduleEntry]int
	globals   starlark.StringDict
	namespace starlark.Value
	err       error
}

func (s *session) loadModule(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, invalidArg("module.load: path must not be empty")
	}
	entry, err := s.loadEntry(thread, path)
	if err != nil {
		return nil, err
	}
	return entry.namespace, nil
}

// load is the ordinary Starlark load statement's callback.
func (s *session) load(thread *starlark.Thread, name string) (starlark.StringDict, error) {
	entry, err := s.loadEntry(thread, name)
	if err != nil {
		return nil, err
	}
	return entry.globals, nil
}

// modulePath skips native frames (including module.load itself) and uses the
// nearest Starlark caller, even when it is a function in a different source file.
func (s *session) modulePath(thread *starlark.Thread, name string) string {
	if filepath.IsAbs(name) {
		return filepath.Clean(name)
	}
	dir := s.spec.WorkingDirectory
	for depth := 0; depth < thread.CallStackDepth(); depth++ {
		pos := thread.CallFrame(depth).Pos
		if pos.Line == 0 {
			continue
		}
		if filepath.IsAbs(pos.Filename()) {
			dir = filepath.Dir(pos.Filename())
		}
		break
	}
	return filepath.Clean(filepath.Join(dir, name))
}

func (s *session) loadEntry(thread *starlark.Thread, name string) (*moduleEntry, error) {
	ctx := threadContext(thread)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := s.modulePath(thread, name)
	parent, _ := thread.Local(activeModuleKey).(*moduleEntry)
	entry, owner, err := s.acquireModule(path, parent)
	if err != nil {
		return nil, err
	}
	if parent != nil {
		defer s.releaseModuleDependency(parent, entry)
	}
	if owner {
		s.evaluateModule(thread, path, entry)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-entry.done:
		return entry, entry.err
	}
}

// acquireModule records an edge before either evaluating or waiting. Looking for
// a reverse path detects same-thread recursion and cross-thread wait cycles alike.
func (s *session) acquireModule(path string, parent *moduleEntry) (*moduleEntry, bool, error) {
	s.moduleMu.Lock()
	defer s.moduleMu.Unlock()
	entry, exists := s.modules[path]
	if exists {
		select {
		case <-entry.done:
			if parent != nil {
				parent.waits[entry]++
			}
			return entry, false, nil
		default:
		}
		if parent != nil && moduleDependsOn(entry, parent, make(map[*moduleEntry]bool)) {
			return nil, false, fail(errUtils.ErrStarlark, "cyclic load of %s", path)
		}
	} else {
		entry = &moduleEntry{done: make(chan struct{}), waits: make(map[*moduleEntry]int)}
		s.modules[path] = entry
	}
	if parent != nil {
		parent.waits[entry]++
	}
	return entry, !exists, nil
}

func moduleDependsOn(entry, target *moduleEntry, seen map[*moduleEntry]bool) bool {
	if entry == target {
		return true
	}
	if seen[entry] {
		return false
	}
	seen[entry] = true
	for dependency := range entry.waits {
		if moduleDependsOn(dependency, target, seen) {
			return true
		}
	}
	return false
}

func (s *session) releaseModuleDependency(parent, entry *moduleEntry) {
	s.moduleMu.Lock()
	defer s.moduleMu.Unlock()
	if parent.waits[entry] <= 1 {
		delete(parent.waits, entry)
	} else {
		parent.waits[entry]--
	}
}

func (s *session) evaluateModule(thread *starlark.Thread, path string, entry *moduleEntry) {
	previous := thread.Local(activeModuleKey)
	thread.SetLocal(activeModuleKey, entry)
	defer thread.SetLocal(activeModuleKey, previous)

	globals, err := s.execModule(thread, path)
	if ctxErr := threadContext(thread).Err(); ctxErr != nil {
		err = ctxErr
	}
	s.moduleMu.Lock()
	defer s.moduleMu.Unlock()
	entry.err = err
	if err == nil {
		entry.globals = globals
		public := make(starlark.StringDict)
		for name, value := range globals {
			if !strings.HasPrefix(name, "_") {
				public[name] = value
			}
		}
		// execAutomation has already frozen all globals, including private state.
		entry.namespace = module(path, public)
	} else {
		// Existing waiters see this failure; a later call may retry initialization.
		delete(s.modules, path)
	}
	close(entry.done)
}

func (s *session) execModule(thread *starlark.Thread, path string) (starlark.StringDict, error) {
	source, err := s.engine.readFile(path)
	if err != nil {
		return nil, failWith(errUtils.ErrStarlark, err, "cannot read module: %s: %s", path, err)
	}
	if err := threadContext(thread).Err(); err != nil {
		return nil, err
	}
	return execAutomation(thread, path, string(source), s.globals)
}
