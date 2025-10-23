package filematch

// REMOVED: fileSystem interface - now using shared filesystem.FileSystem from pkg/filesystem.
// This eliminates duplication and allows consistent mocking across the codebase.

// globCompiler defines the glob pattern compilation behavior.
type globCompiler interface {
	Compile(pattern string) (compiledGlob, error)
}

// compiledGlob defines the behavior of a compiled glob pattern.
type compiledGlob interface {
	Match(string) bool
}

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

// FileMatcher matches file paths against configured patterns and returns the subset of paths that match.
type FileMatcher interface {
	MatchFiles([]string) ([]string, error)
}
