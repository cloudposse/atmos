package store

import "strings"

// Store defines the common interface for all store implementations.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_store.go -package=store
type Store interface {
	Set(stack string, component string, key string, value any) error
	Get(stack string, component string, key string) (any, error)
	GetKey(key string) (any, error)
}

// StoreFactory is a function type to initialize a new store.
type StoreFactory func(options map[string]any) (Store, error)

// nolint
// getKey generates a key for the store. First it splits the stack by the stack delimiter (from atmos.yaml),
// then it splits the component if it contains a "/",
// then it appends the key to the parts,
// then it joins the parts with the final delimiter.
func getKey(prefix string, stackDelimiter string, stack string, component string, key string, finalDelimiter string) (string, error) { //nolint
	stackParts := strings.Split(stack, stackDelimiter)
	componentParts := strings.Split(component, "/")

	parts := append([]string{prefix}, stackParts...)
	parts = append(parts, componentParts...)
	parts = append(parts, key)

	joinedKey := strings.Join(parts, finalDelimiter)
	finalKey := strings.ReplaceAll(joinedKey, "//", "/")

	return finalKey, nil
}
