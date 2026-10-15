package exec

import (
	"context"
	"fmt"
	"reflect"
	"text/template"

	"github.com/hairyhenderson/gomplate/v3/data"
	"github.com/hairyhenderson/gomplate/v3/funcs" //nolint:staticcheck // Gomplate v3 exposes namespace types through this embedding API.

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/policy/guard"
)

const policyTemplateEngine = "go-template"

// policyTemplateFuncs wraps a private map, never the cached Sprig map.
// Namespace constructors are admission points too, including namespace values saved in template variables.
func policyTemplateFuncs(original template.FuncMap, file string, d *data.Data) (template.FuncMap, error) {
	functionRules, err := guard.Enabled("function")
	if err != nil {
		return nil, err
	}
	sourceRules, err := guard.Enabled("source")
	if err != nil {
		return nil, err
	}
	if !functionRules && !sourceRules {
		return original, nil
	}
	result := make(template.FuncMap, len(original))
	for name, fn := range original {
		fn = policyNamespace(name, fn, file)
		if sourceRules {
			fn = policySourceFunction(name, fn, file, d)
		}
		result[name] = wrapTemplateFunction(name, fn, file)
	}

	return result, nil
}

func wrapTemplateFunction(name string, fn any, file string) any {
	value := reflect.ValueOf(fn)
	typ := value.Type()
	errorType := reflect.TypeFor[error]()
	in := make([]reflect.Type, typ.NumIn())
	for i := range in {
		in[i] = typ.In(i)
	}
	out := make([]reflect.Type, typ.NumOut())
	for i := range out {
		out[i] = typ.Out(i)
	}
	hasError := len(out) == 2 && out[1] == errorType
	if !hasError {
		out = append(out, errorType)
	}
	wrapper := reflect.FuncOf(in, out, typ.IsVariadic())
	return reflect.MakeFunc(wrapper, func(args []reflect.Value) []reflect.Value {
		if err := admitTemplateCall(name, file, args); err != nil {
			return templateCallError(out, err)
		}
		var result []reflect.Value
		if typ.IsVariadic() {
			result = value.CallSlice(args)
		} else {
			result = value.Call(args)
		}
		if !hasError {
			result = append(result, reflect.Zero(errorType))
		}
		return result
	}).Interface()
}

func policyNamespace(name string, fn any, file string) any {
	switch name {
	case "atmos":
		native := *fn.(func() any)().(*AtmosFuncs)
		native.sourceFile = file
		return func() any { return &native }
	case "data":
		native := fn.(func() any)().(*funcs.DataFuncs)
		return func() any { return &policyDataFuncs{native: native, file: file} }
	case "file":
		native := fn.(func() interface{})().(*funcs.FileFuncs)
		return func() any { return &policyFileFuncs{native: native, file: file} }
	case "net":
		native := fn.(func() interface{})().(*funcs.NetFuncs)
		return func() any { return &policyNetFuncs{native: native, file: file} }
	case "env":
		if reflect.TypeOf(fn).NumIn() == 0 {
			native := fn.(func() interface{})().(*funcs.EnvFuncs)
			return func() any { return &policyEnvFuncs{native: native, file: file} }
		}
	}
	return fn
}

func policySourceFunction(name string, fn any, file string, d *data.Data) any {
	switch name {
	case "json":
		return guardedTemplateJSON
	case "datasource", "ds":
		return func(alias string, args ...string) (any, error) {
			return policyDatasource(d, file, alias, false, args...)
		}
	case "include":
		return func(alias string, args ...string) (any, error) {
			return policyDatasource(d, file, alias, true, args...)
		}
	case "datasourceReachable":
		return func(alias string, args ...string) (bool, error) {
			_, err := policyDatasource(d, file, alias, true, args...)
			return err == nil, err
		}
	// These namespaces hide their destinations inside the provider SDK.
	case "aws", "gcp":
		return func() (any, error) {
			return nil, fmt.Errorf("%w: provider template namespace has no guarded source collector", errUtils.ErrPolicyInputUnavailable)
		}
	}
	return fn
}

func templateCallError(out []reflect.Type, err error) []reflect.Value {
	values := make([]reflect.Value, len(out))
	for i := range values {
		values[i] = reflect.Zero(out[i])
	}
	values[len(values)-1] = reflect.ValueOf(err)
	return values
}

func admitTemplateCall(name, file string, args []reflect.Value) error {
	if err := guard.Function(context.Background(), name, policyTemplateEngine, guard.FileOrigin(file)); err != nil {
		return err
	}
	if name == "getHostByName" && len(args) > 0 {
		return guard.Source(context.Background(), "template.dns", "dns://"+fmt.Sprint(args[0].Interface()), file)
	}
	return nil
}
