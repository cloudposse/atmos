package starlark

import (
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/cloudposse/atmos/pkg/ci"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// frozenStruct builds an immutable named struct from plain values.
func frozenStruct(name string, members starlark.StringDict) starlark.Value {
	value := starlarkstruct.FromStringDict(starlark.String(name), members)
	value.Freeze()
	return value
}

// ciContextValue returns the frozen `ci.context` struct, or None when the reporter has no
// context at all. Starlark attribute access does not expose the calling thread, so the context is
// resolved once through the unbound reporter and cached for the session. That is safe because the
// context describes the run, not the calling task, and local renderings never use it.
func (s *session) ciContextValue() starlark.Value {
	s.ciCtxOnce.Do(func() {
		s.ciCtxValue = starlark.None
		c, err := s.ciReporter.Context()
		if err != nil || c == nil {
			log.Debug("CI context is unavailable to the script", "error", err)
			return
		}
		s.ciCtxValue = contextValue(c)
	})
	return s.ciCtxValue
}

// contextValue converts the run context. `pr.fork` is true when the pull request head lives in a
// fork of the base repository, the condition under which elevated events hold comments and checks. `local` is true when the generic fallback provider
// supplied the context, meaning no CI provider was detected and writes render locally. It does not
// reflect configuration switches: a detected provider whose switch is off still reports false, and
// the write itself warns when it is rendered locally.
func contextValue(c *ci.Context) starlark.Value {
	var pr starlark.Value = starlark.None
	if c.PullRequest != nil {
		pr = frozenStruct("pr", starlark.StringDict{
			"number": starlark.MakeInt(c.PullRequest.Number),
			"head":   starlark.String(c.PullRequest.HeadRef),
			"base":   starlark.String(c.PullRequest.BaseRef),
			"url":    starlark.String(c.PullRequest.URL),
			"fork":   starlark.Bool(c.PullRequest.Fork),
		})
	}
	return frozenStruct("context", starlark.StringDict{
		"provider": starlark.String(c.Provider),
		"local":    starlark.Bool(c.Provider == ci.GenericProviderName),
		"event":    starlark.String(c.EventName),
		"sha":      starlark.String(c.SHA),
		"branch":   starlark.String(c.Branch),
		"repo":     starlark.String(c.Repository),
		"actor":    starlark.String(c.Actor),
		"run_id":   starlark.String(c.RunID),
		"run_url":  starlark.String(c.RunURL),
		"elevated": starlark.Bool(c.ElevatedEvent),
		"pr":       pr,
	})
}

// ciBase resolves the base commit for affected detection. An unresolved base is None rather than
// an error, because it is an expected state outside CI and in shallow clones.
func (s *session) ciBase(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if err := unpackCI(b, args, kwargs); err != nil {
		return nil, err
	}
	base, err := s.reporter(t).Base()
	if err != nil || base == nil {
		log.Debug("CI base is unavailable to the script", "error", err)
		return starlark.None, nil
	}
	return frozenStruct("base", starlark.StringDict{
		"ref":           starlark.String(base.Ref),
		"sha":           starlark.String(base.SHA),
		"head_sha":      starlark.String(base.HeadSHA),
		"target_branch": starlark.String(base.TargetBranch),
		"source":        starlark.String(base.Source),
	}), nil
}

// commentValue converts a posted comment. A comment rendered locally by the generic provider
// carries a synthetic id that starts at 1 and an empty URL. Only a reporter that returned no
// comment at all (a mock, or a provider that cannot render one) reads as id 0.
func commentValue(comment *ci.Comment) starlark.Value {
	if comment == nil {
		comment = &ci.Comment{}
	}
	return frozenStruct("comment", starlark.StringDict{
		"id":      starlark.MakeInt64(comment.ID),
		"url":     starlark.String(comment.URL),
		"created": starlark.Bool(comment.Created),
		"target":  starlark.String(comment.Target),
	})
}
