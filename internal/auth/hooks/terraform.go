package auth

// This file has been consolidated into internal/auth/interfaces.go
// The TerraformPreHook function is now available in the main auth package
// with complete component auth merging and logging functionality.
//
// To use the TerraformPreHook, import:
// import "github.com/cloudposse/atmos/internal/auth"
// and call: auth.TerraformPreHook(atmosConfig, stackInfo)
