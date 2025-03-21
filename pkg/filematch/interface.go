package filematch

import (
	"os"
	"path/filepath"
)

// fileSystem defines the filesystem operations needed by MatchFiles.
type fileSystem interface {
	Getwd() (string, error)
	Walk(root string, walkFn filepath.WalkFunc) error
	Stat(path string) (os.FileInfo, error)
}

// globCompiler defines the glob pattern compilation behavior.
type globCompiler interface {
	Compile(pattern string) (compiledGlob, error)
}

// compiledGlob defines the behavior of a compiled glob pattern.
type compiledGlob interface {
	Match(string) bool
}

//go:generate mockgen -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type FileMatcherInterface interface {
	MatchFiles([]string) ([]string, error)
}
