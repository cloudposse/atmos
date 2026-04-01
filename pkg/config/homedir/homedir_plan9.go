//go:build plan9

package homedir

// init sets homeEnvName to "home" on Plan 9, where environment variables follow
// a lowercase convention. This runs before any goroutines start, so there is no
// data race with concurrent readers; subsequent test overrides of homeEnvName
// must save/restore the value to avoid cross-test interference.
func init() {
	// On Plan 9, environment variables follow a lowercase convention.
	// Override the default "HOME" env key to the Plan 9 equivalent "home".
	homeEnvName = "home"
}
