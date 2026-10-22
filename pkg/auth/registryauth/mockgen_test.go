package registryauth

//go:generate go run go.uber.org/mock/mockgen -destination mock_manager_test.go -package registryauth github.com/cloudposse/atmos/pkg/auth/registryauth Manager
