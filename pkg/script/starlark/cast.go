package starlark

import (
	"path/filepath"

	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/api"
	"github.com/cloudposse/atmos/pkg/asciicast"
	"github.com/cloudposse/atmos/pkg/castplan"
	"github.com/cloudposse/atmos/pkg/perf"
	castmodule "github.com/cloudposse/atmos/pkg/script/starlark/stdlib/cast"
)

type castHost struct{ session *session }

func (s *session) castModule() starlark.Value { return castmodule.New(&castHost{session: s}) }

func (h *castHost) services(t *starlark.Thread) (api.CastLibrary, *api.StepCall, error) {
	library, ok := threadSteps(t).(api.CastLibrary)
	if !ok {
		return nil, nil, invalidArg("the host has not provided cast services")
	}
	s := h.session
	call := &api.StepCall{
		Type: "cast", WorkingDirectory: s.spec.WorkingDirectory,
		ProcessEnv: s.tools.Environment(s.spec.ProcessEnv), Stdout: s.writer(t, stdoutStream), Stderr: s.writer(t, stderrStream),
		Parallel: s.spec.Parallel || t.Local(outputKey) != nil,
	}
	return library, call, nil
}

func (h *castHost) Check(t *starlark.Thread, plan *castplan.Plan) error {
	defer perf.Track(nil, "starlark.castHost.Check")()

	library, call, err := h.services(t)
	if err != nil {
		return err
	}
	return library.ValidateCast(plan, call)
}

func (h *castHost) Generate(t *starlark.Thread, plan *castplan.Plan, policy starlark.Callable) (string, error) {
	defer perf.Track(nil, "starlark.castHost.Generate")()

	library, call, err := h.services(t)
	if err != nil {
		return "", err
	}
	return library.GenerateCast(threadContext(t), plan, call, func(path string) error {
		if err := asciicast.NormalizeRecording(path, plan.Replacements); err != nil {
			return err
		}
		return h.checkRecording(t, path, plan, policy)
	})
}

func (h *castHost) Validate(t *starlark.Thread, plan *castplan.Plan, policy starlark.Callable) (string, error) {
	defer perf.Track(nil, "starlark.castHost.Validate")()

	output, _ := plan.Configuration["output"].(map[string]any)
	path, _ := output["cast"].(string)
	if path == "" {
		return "", invalidArg("cast.recorder requires output.cast for saved validation")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(h.session.spec.WorkingDirectory, path)
	}
	return path, h.checkRecording(t, path, plan, policy)
}

func (h *castHost) checkRecording(t *starlark.Thread, path string, plan *castplan.Plan, policy starlark.Callable) error {
	text, err := asciicast.ValidateDeclaration(path, plan)
	if err != nil {
		return err
	}
	if policy != nil {
		_, err = starlark.Call(t, policy, starlark.Tuple{starlark.String(text)}, nil)
	}
	return err
}
