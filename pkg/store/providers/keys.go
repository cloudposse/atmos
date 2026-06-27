package providers

import "strings"

// getKey generates a key for the store. First it splits the stack by the stack delimiter (from atmos.yaml),
// then it splits the component if it contains a "/",
// then it appends the key to the parts,
// then it joins the parts with the final delimiter.
//
// Empty segments are omitted entirely — independent of the final delimiter — so scoped secret
// coordinates collapse cleanly: an empty component (a stack-scoped secret) yields
// `prefix<delim>stack<delim>key`, and an empty stack and component (a global secret) yields
// `prefix<delim>key`.
func getKey(prefix string, stackDelimiter string, stack string, component string, key string, finalDelimiter string) (string, error) { //nolint
	parts := []string{prefix}
	if stack != "" {
		parts = append(parts, strings.Split(stack, stackDelimiter)...)
	}
	if component != "" {
		parts = append(parts, strings.Split(component, "/")...)
	}
	parts = append(parts, key)

	joinedKey := strings.Join(parts, finalDelimiter)
	finalKey := strings.ReplaceAll(joinedKey, "//", "/")

	return finalKey, nil
}
