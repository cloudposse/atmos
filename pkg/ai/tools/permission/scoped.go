package permission

// ScopedTool is an optional interface a Tool can implement to scope remembered
// ("always allow" / "always deny") decisions more narrowly than the tool name.
//
// CacheKey returns the key under which decisions are stored, for example
// "Bash(atmos list stacks)". Matching a scoped key is exact string equality on the
// whole key: a stored "Bash(atmos list stacks)" never matches the bare tool name
// "Bash" or a different command, and the legacy "Name(anything)" rule is not applied.
// A tool with no stable specifier may return just its name; that key then matches
// only the bare name, never a parenthesized entry.
type ScopedTool interface {
	Tool
	CacheKey() string
}
