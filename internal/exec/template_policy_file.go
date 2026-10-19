package exec

import (
	"context"
	"os"

	"github.com/hairyhenderson/gomplate/v3/conv"
	"github.com/hairyhenderson/gomplate/v3/funcs" //nolint:staticcheck // Gomplate v3 exposes namespace types through this embedding API.

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy/guard"
)

// policyFileFuncs deliberately does not embed FileFuncs: new library methods must not bypass admission.
type policyFileFuncs struct {
	native *funcs.FileFuncs
	file   string
}

// check admits the function and the path it names, and returns that path. Gomplate converts the path
// argument with conv.ToString, so a []byte or other value names the file it converts to; the same
// string is checked and then handed to the native method, so admission and the read cannot disagree.
func (f *policyFileFuncs) check(method string, path any) (string, error) {
	if err := guard.Function(context.Background(), "file."+method, policyTemplateEngine, guard.FileOrigin(f.file)); err != nil {
		return "", err
	}
	name := conv.ToString(path)
	if err := guard.Source(context.Background(), "template.file", name, f.file); err != nil {
		return "", err
	}
	return name, nil
}

func (f *policyFileFuncs) Read(path any) (string, error) {
	defer perf.Track(nil, "exec.Read")()

	name, err := f.check("Read", path)
	if err != nil {
		return "", err
	}
	return f.native.Read(name)
}

func (f *policyFileFuncs) Stat(path any) (os.FileInfo, error) {
	defer perf.Track(nil, "exec.Stat")()

	name, err := f.check("Stat", path)
	if err != nil {
		return nil, err
	}
	return f.native.Stat(name)
}

func (f *policyFileFuncs) Exists(path any) (bool, error) {
	defer perf.Track(nil, "exec.Exists")()

	name, err := f.check("Exists", path)
	if err != nil {
		return false, err
	}
	return f.native.Exists(name), nil
}

func (f *policyFileFuncs) IsDir(path any) (bool, error) {
	defer perf.Track(nil, "exec.IsDir")()

	name, err := f.check("IsDir", path)
	if err != nil {
		return false, err
	}
	return f.native.IsDir(name), nil
}

func (f *policyFileFuncs) ReadDir(path any) ([]string, error) {
	defer perf.Track(nil, "exec.ReadDir")()

	name, err := f.check("ReadDir", path)
	if err != nil {
		return nil, err
	}
	return f.native.ReadDir(name)
}

func (f *policyFileFuncs) Walk(path any) ([]string, error) {
	defer perf.Track(nil, "exec.Walk")()

	name, err := f.check("Walk", path)
	if err != nil {
		return nil, err
	}
	return f.native.Walk(name)
}

func (f *policyFileFuncs) Write(path, content any) (string, error) {
	defer perf.Track(nil, "exec.Write")()

	name, err := f.check("Write", path)
	if err != nil {
		return "", err
	}
	return f.native.Write(name, content)
}
