package gomplate

import (
	"context"
	"maps"
	"reflect"
	"sync"
	"text/template"

	gomplate "github.com/hairyhenderson/gomplate/v4"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Gomplate v4 builds a working-directory filesystem for its file namespace on every
// gomplate.CreateFuncs call, which costs an os.Getwd system call per call. Atmos builds a
// function map for every template it renders (including each component's stack name
// template), so creating the full map per render made stack processing much slower than
// with Gomplate v3. The stateless namespaces are therefore created once per process and
// cloned per function map, and only the namespaces that capture process state are created
// per function map, lazily, the first time a template uses them.

// experimentalKey marks contexts whose function maps include Gomplate experimental functions.
type experimentalKey struct{}

// SetExperimental enables Gomplate experimental functions for CreateFuncs. It sets both this
// package's marker and Gomplate's own flag; contexts enabled with gomplate.SetExperimental alone
// are recognized too, as long as Gomplate's flag can be located (see upstreamExperimentalKey).
func SetExperimental(ctx context.Context) context.Context {
	defer perf.Track(nil, "gomplate.SetExperimental")()

	return context.WithValue(gomplate.SetExperimental(ctx), experimentalKey{}, true)
}

// upstreamExperimentalKey is the context key gomplate.SetExperimental stores its flag under. The
// key's type is unexported, so it is recovered from a context Gomplate enabled: reflection can read
// the type of the stored key, and a new value of that type is equal to the key. It is nil when the
// key cannot be located, in which case only this package's marker is recognized.
var upstreamExperimentalKey = func() any {
	value := reflect.ValueOf(gomplate.SetExperimental(context.Background()))
	if value.Kind() != reflect.Pointer || value.Elem().Kind() != reflect.Struct {
		return nil
	}
	field := value.Elem().FieldByName("key")
	if !field.IsValid() || field.Kind() != reflect.Interface || field.IsNil() {
		return nil
	}
	key := reflect.New(field.Elem().Type()).Elem()
	if !key.Type().Comparable() {
		return nil
	}
	return key.Interface()
}()

func isExperimental(ctx context.Context) bool {
	if enabled, _ := ctx.Value(experimentalKey{}).(bool); enabled {
		return true
	}
	if upstreamExperimentalKey == nil {
		return false
	}
	enabled, _ := ctx.Value(upstreamExperimentalKey).(bool)
	return enabled
}

// renderScopedFuncs are Gomplate functions whose namespaces capture process state when they
// are created: file binds the working directory, and the cloud namespaces read client options
// from the environment and cache their clients. Sharing them across renders would freeze that
// state, so every function map creates its own on first use, as Gomplate v3 did.
var renderScopedFuncs = []string{"file", "aws", "gcp", "ec2meta", "ec2dynamic", "ec2tag", "ec2tags", "ec2region"}

// sharedFuncMaps holds one function map per experimental setting (index 1 is experimental).
var sharedFuncMaps [2]struct {
	once  sync.Once
	funcs template.FuncMap
}

func sharedFuncMap(experimental bool) template.FuncMap {
	index := 0
	if experimental {
		index = 1
	}
	entry := &sharedFuncMaps[index]
	entry.once.Do(func() {
		ctx := context.Background()
		if experimental {
			ctx = gomplate.SetExperimental(ctx)
		}
		entry.funcs = gomplate.CreateFuncs(ctx)
	})
	return entry.funcs
}

// gomplateFuncs returns a new Gomplate function map for one render. Callers may modify it.
func gomplateFuncs(ctx context.Context) template.FuncMap {
	funcs := maps.Clone(sharedFuncMap(isExperimental(ctx)))
	scoped := sync.OnceValue(func() template.FuncMap { return gomplate.CreateFuncs(ctx) })
	namespace := func(name string) func() any {
		return func() any { return scoped()[name].(func() any)() }
	}
	funcs["file"] = namespace("file")
	funcs["aws"] = namespace("aws")
	funcs["gcp"] = namespace("gcp")
	funcs["ec2meta"] = func(key string, def ...string) (string, error) {
		return scoped()["ec2meta"].(func(string, ...string) (string, error))(key, def...)
	}
	funcs["ec2dynamic"] = func(key string, def ...string) (string, error) {
		return scoped()["ec2dynamic"].(func(string, ...string) (string, error))(key, def...)
	}
	funcs["ec2tag"] = func(tag string, def ...string) (string, error) {
		return scoped()["ec2tag"].(func(string, ...string) (string, error))(tag, def...)
	}
	funcs["ec2tags"] = func() (map[string]string, error) {
		return scoped()["ec2tags"].(func() (map[string]string, error))()
	}
	funcs["ec2region"] = func(def ...string) (string, error) {
		return scoped()["ec2region"].(func(...string) (string, error))(def...)
	}
	return funcs
}
