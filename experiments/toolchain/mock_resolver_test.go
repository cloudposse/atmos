package main

import "fmt"

type mockToolResolver struct {
	mapping map[string][2]string // toolName -> [owner, repo]
}

func (m *mockToolResolver) Resolve(toolName string) (string, string, error) {
	if val, ok := m.mapping[toolName]; ok {
		return val[0], val[1], nil
	}
	return "", "", fmt.Errorf("not found")
}
