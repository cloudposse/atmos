package materialize

import "strings"

// maxPathChars is the Windows MAX_PATH limit, which counts the terminating NUL.
const maxPathChars = 260

// extendedLengthPath converts a long absolute Windows path to the extended-length form that APIs such
// as MoveFile accept beyond MAX_PATH. A path shorter than the limit, and one that already has an
// extended-length prefix, is returned unchanged. A UNC path (\\server\share) becomes \\?\UNC\server\share.
// The path must already be absolute and use backslashes: the prefix switches off the normalization
// that would otherwise convert slashes and resolve dot segments.
func extendedLengthPath(absolute string) string {
	if len(absolute) < maxPathChars || strings.HasPrefix(absolute, `\\?\`) {
		return absolute
	}
	if strings.HasPrefix(absolute, `\\`) {
		return `\\?\UNC\` + absolute[2:]
	}
	return `\\?\` + absolute
}
