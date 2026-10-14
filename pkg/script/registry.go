package script

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RegistrationOption declares how an interpreter is selected for standalone files.
type RegistrationOption func(*registration)

type registration struct {
	extensions []string
	shebang    bool
}

// WithExtensions associates case-sensitive file extensions (including the dot) with an interpreter.
func WithExtensions(extensions ...string) RegistrationOption {
	defer perf.Track(nil, "script.WithExtensions")()

	owned := append([]string(nil), extensions...)
	return func(r *registration) { r.extensions = append(r.extensions, owned...) }
}

// WithAtmosShebang selects the interpreter for an extensionless Atmos shebang
// and for stdin scripts without an explicit --interpreter.
// Only one interpreter may own this fallback; a registered extension takes precedence.
func WithAtmosShebang() RegistrationOption {
	defer perf.Track(nil, "script.WithAtmosShebang")()

	return func(r *registration) { r.shebang = true }
}

type engineRegistry struct {
	sync.RWMutex
	engines    map[string]Engine
	extensions map[string]string
	shebang    string
}

func newRegistry() *engineRegistry {
	return &engineRegistry{engines: make(map[string]Engine), extensions: make(map[string]string)}
}

var registry = newRegistry()

// Register installs an interpreter, panicking on invalid or conflicting metadata.
// Replacing an engine under the same name preserves its existing extension and
// shebang associations, so hosts can inject services after CLI initialization.
func Register(name string, engine Engine, options ...RegistrationOption) {
	defer perf.Track(nil, "script.Register")()
	if err := registry.register(name, engine, options...); err != nil {
		panic(err)
	}
}

func (r *engineRegistry) register(name string, engine Engine, options ...RegistrationOption) error {
	name = strings.TrimSpace(name)
	if name == "" || engine == nil {
		return fmt.Errorf("%w: name and engine are required", errUtils.ErrScriptRegistration)
	}
	config := registration{}
	for _, option := range options {
		option(&config)
	}
	r.Lock()
	defer r.Unlock()
	if err := r.validate(name, config); err != nil {
		return err
	}
	r.engines[name] = engine
	for _, extension := range config.extensions {
		r.extensions[extension] = name
	}
	if config.shebang {
		r.shebang = name
	}
	return nil
}

// Get looks up an embedded interpreter; executable paths remain external.
func Get(name string) (Engine, bool) {
	defer perf.Track(nil, "script.Get")()
	registry.RLock()
	defer registry.RUnlock()
	engine, ok := registry.engines[strings.TrimSpace(name)]
	return engine, ok
}

// InterpreterForFile resolves a registered extension without reading the file.
func InterpreterForFile(path string) (string, bool) {
	defer perf.Track(nil, "script.InterpreterForFile")()

	registry.RLock()
	defer registry.RUnlock()
	name, ok := registry.extensions[filepath.Ext(path)]
	return name, ok
}

func shebangInterpreter() string {
	registry.RLock()
	defer registry.RUnlock()
	return registry.shebang
}

// validate checks all associations before register mutates any of them. The caller holds the lock.
func (r *engineRegistry) validate(name string, config registration) error {
	for _, extension := range config.extensions {
		if !validExtension(extension) {
			return fmt.Errorf("%w: invalid extension %q", errUtils.ErrScriptRegistration, extension)
		}
		if owner, exists := r.extensions[extension]; exists && owner != name {
			return fmt.Errorf("%w: extension %q already belongs to %q", errUtils.ErrScriptRegistration, extension, owner)
		}
	}
	if config.shebang && r.shebang != "" && r.shebang != name {
		return fmt.Errorf("%w: Atmos shebang already belongs to %q", errUtils.ErrScriptRegistration, r.shebang)
	}
	return nil
}

func validExtension(extension string) bool {
	return len(extension) >= 2 && extension[0] == '.' && !strings.ContainsAny(extension, "/\\ \t\r\n") && filepath.Ext("file"+extension) == extension
}
