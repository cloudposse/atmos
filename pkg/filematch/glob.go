package filematch

import (
	"github.com/gobwas/glob"
)

// defaultGlobCompiler implements GlobCompiler using gobwas/glob.
type defaultGlobCompiler struct{}

func NewDefaultGlobCompiler() globCompiler {
	return &defaultGlobCompiler{}
}

func (c *defaultGlobCompiler) Compile(pattern string) (compiledGlob, error) {
	g, err := glob.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return &defaultGlob{g}, nil
}

type defaultGlob struct {
	g glob.Glob
}

func (g *defaultGlob) Match(s string) bool {
	return g.g.Match(s)
}
