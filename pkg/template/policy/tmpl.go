package policy

import (
	"context"

	"github.com/hairyhenderson/gomplate/v5/tmpl"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy/guard"
)

// nativeTmplFuncs describes gomplate's `tmpl` namespace, whose methods define
// and execute templates at run time.
type nativeTmplFuncs interface {
	Path() (string, error)
	PathDir() (string, error)
	Inline(args ...any) (string, error)
	Exec(name string, tmplcontext ...any) (string, error)
}

var _ nativeTmplFuncs = (*tmpl.Template)(nil)

// policyTmplFuncs deliberately does not embed the native namespace: new library methods must not bypass admission.
// Each method is admitted as "tmpl.<Method>", as the other guarded namespaces are. Templates these methods run
// use the render's function map, which is already guarded, so only the methods themselves need admission here.
type policyTmplFuncs struct {
	native nativeTmplFuncs
	file   string
}

func (f *policyTmplFuncs) admit(method string) error {
	return guard.Function(context.Background(), "tmpl."+method, Engine, guard.FileOrigin(f.file))
}

func (f *policyTmplFuncs) Path() (string, error) {
	defer perf.Track(nil, "policy.TmplPath")()

	if err := f.admit("Path"); err != nil {
		return "", err
	}
	return f.native.Path()
}

func (f *policyTmplFuncs) PathDir() (string, error) {
	defer perf.Track(nil, "policy.TmplPathDir")()

	if err := f.admit("PathDir"); err != nil {
		return "", err
	}
	return f.native.PathDir()
}

func (f *policyTmplFuncs) Inline(args ...any) (string, error) {
	defer perf.Track(nil, "policy.TmplInline")()

	if err := f.admit("Inline"); err != nil {
		return "", err
	}
	return f.native.Inline(args...)
}

func (f *policyTmplFuncs) Exec(name string, tmplcontext ...any) (string, error) {
	defer perf.Track(nil, "policy.TmplExec")()

	if err := f.admit("Exec"); err != nil {
		return "", err
	}
	return f.native.Exec(name, tmplcontext...)
}
