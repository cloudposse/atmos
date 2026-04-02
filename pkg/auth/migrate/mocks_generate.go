package migrate

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mocks/mock_filesystem.go -package=mocks github.com/cloudposse/atmos/pkg/auth/migrate FileSystem
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mocks/mock_prompter.go -package=mocks github.com/cloudposse/atmos/pkg/auth/migrate Prompter
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mocks/mock_step.go -package=mocks github.com/cloudposse/atmos/pkg/auth/migrate MigrationStep
