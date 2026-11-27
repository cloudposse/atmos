package adoption

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/cloudposse/atmos/pkg/perf"
	stackadoption "github.com/cloudposse/atmos/pkg/stack/adoption"
)

// Adapters returns the source formats that can produce Terraform component instances.
func Adapters() stackadoption.Adapters {
	defer perf.Track(nil, "adoption.Adapters")()

	return stackadoption.Adapters{rootAdapter{}, unitAdapter{}, atlantisAdapter{}}
}

type request struct {
	stackadoption.Registration
	atlantisSource, atlantisWorkspace, atlantisVersion string
}

type rootAdapter struct{}

func (rootAdapter) Kind() string {
	defer perf.Track(nil, "adoption.rootAdapter.Kind")()
	return terraform
}

func (rootAdapter) Options() []string {
	defer perf.Track(nil, "adoption.rootAdapter.Options")()
	return []string{"command", "name", "var-file"}
}

func (rootAdapter) Detect(s stackadoption.Source) (stackadoption.Match, error) {
	defer perf.Track(nil, "adoption.rootAdapter.Detect")()

	if s.IsDir && slices.ContainsFunc(s.Files, func(name string) bool { return configSuffix(name, ".tf") || configSuffix(name, ".tofu") }) {
		return stackadoption.Match{Path: s.Path}, nil
	}
	return stackadoption.Match{}, nil
}

func (rootAdapter) Adopt(ctx context.Context, r *stackadoption.Registration, path string) error {
	defer perf.Track(nil, "adoption.rootAdapter.Adopt")()

	req := request{Registration: *r}
	req.Dir = path
	return req.saveTerraform(ctx)
}

type unitAdapter struct{}

func (unitAdapter) Kind() string {
	defer perf.Track(nil, "adoption.unitAdapter.Kind")()
	return tg
}

func (unitAdapter) Options() []string {
	defer perf.Track(nil, "adoption.unitAdapter.Options")()
	return []string{"command"}
}

func (unitAdapter) Detect(s stackadoption.Source) (stackadoption.Match, error) {
	defer perf.Track(nil, "adoption.unitAdapter.Detect")()

	name := filepath.Base(s.Path)
	if !s.IsDir && name != unitFile && name != stackFile {
		return stackadoption.Match{}, nil
	}
	unit, err := TerragruntUnit(s.Path)
	return stackadoption.Match{Path: unit, Wrapper: unit != ""}, err
}

func (unitAdapter) Adopt(ctx context.Context, r *stackadoption.Registration, path string) error {
	defer perf.Track(nil, "adoption.unitAdapter.Adopt")()

	req := request{Registration: *r}
	return req.saveUnit(ctx, path)
}

type atlantisAdapter struct{}

func (atlantisAdapter) Kind() string {
	defer perf.Track(nil, "adoption.atlantisAdapter.Kind")()
	return "atlantis"
}

func (atlantisAdapter) Options() []string {
	defer perf.Track(nil, "adoption.atlantisAdapter.Options")()
	return []string{"command", "name", "project"}
}

func (atlantisAdapter) Detect(s stackadoption.Source) (stackadoption.Match, error) {
	defer perf.Track(nil, "adoption.atlantisAdapter.Detect")()

	name := filepath.Base(s.Path)
	if !s.IsDir && (name == "atlantis.yaml" || name == "atlantis.yml") {
		return stackadoption.Match{Path: s.Path}, nil
	}
	return stackadoption.Match{}, nil
}

func (atlantisAdapter) Adopt(ctx context.Context, r *stackadoption.Registration, path string) error {
	defer perf.Track(nil, "adoption.atlantisAdapter.Adopt")()

	req := request{Registration: *r}
	req.Dir = path
	return req.saveAtlantis(ctx)
}
