package exec

import (
	"context"
	"fmt"
	"os"

	"github.com/hairyhenderson/gomplate/v3/funcs" //nolint:staticcheck // Gomplate v3 exposes namespace types through this embedding API.

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy/guard"
)

// policyFileFuncs deliberately does not embed FileFuncs: new library methods must not bypass admission.
type policyFileFuncs struct {
	native *funcs.FileFuncs
	file   string
}

func (f *policyFileFuncs) check(method string, path any) error {
	if err := guard.Function(context.Background(), "file."+method, policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return err
	}
	return guard.Source(context.Background(), "template.file", fmt.Sprint(path), f.file)
}

func (f *policyFileFuncs) Read(path any) (string, error) {
	defer perf.Track(nil, "exec.Read")()

	if err := f.check("Read", path); err != nil {
		return "", err
	}
	return f.native.Read(path)
}

func (f *policyFileFuncs) Stat(path any) (os.FileInfo, error) {
	defer perf.Track(nil, "exec.Stat")()

	if err := f.check("Stat", path); err != nil {
		return nil, err
	}
	return f.native.Stat(path)
}

func (f *policyFileFuncs) Exists(path any) (bool, error) {
	defer perf.Track(nil, "exec.Exists")()

	if err := f.check("Exists", path); err != nil {
		return false, err
	}
	return f.native.Exists(path), nil
}

func (f *policyFileFuncs) IsDir(path any) (bool, error) {
	defer perf.Track(nil, "exec.IsDir")()

	if err := f.check("IsDir", path); err != nil {
		return false, err
	}
	return f.native.IsDir(path), nil
}

func (f *policyFileFuncs) ReadDir(path any) ([]string, error) {
	defer perf.Track(nil, "exec.ReadDir")()

	if err := f.check("ReadDir", path); err != nil {
		return nil, err
	}
	return f.native.ReadDir(path)
}

func (f *policyFileFuncs) Walk(path any) ([]string, error) {
	defer perf.Track(nil, "exec.Walk")()

	if err := f.check("Walk", path); err != nil {
		return nil, err
	}
	return f.native.Walk(path)
}

func (f *policyFileFuncs) Write(path, content any) (string, error) {
	defer perf.Track(nil, "exec.Write")()

	if err := f.check("Write", path); err != nil {
		return "", err
	}
	return f.native.Write(path, content)
}
