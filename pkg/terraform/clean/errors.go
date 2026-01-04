package clean

import "errors"

// Error variables for the clean package.
var (
	ErrParseTerraformComponents  = errors.New("could not parse Terraform components")
	ErrParseComponentsAttributes = errors.New("could not parse component attributes")
	ErrDescribeStack             = errors.New("error describing stacks")
	ErrEmptyPath                 = errors.New("path cannot be empty")
	ErrPathNotExist              = errors.New("path does not exist")
	ErrFileStat                  = errors.New("error getting file stat")
	ErrMatchPattern              = errors.New("error matching pattern")
	ErrReadDir                   = errors.New("error reading directory")
	ErrFailedFoundStack          = errors.New("failed to find stack folders")
	ErrCollectFiles              = errors.New("failed to collect files")
	ErrEmptyEnvDir               = errors.New("ENV TF_DATA_DIR is empty")
	ErrResolveEnvDir             = errors.New("error resolving TF_DATA_DIR path")
	ErrRefusingToDeleteDir       = errors.New("refusing to delete root directory")
	ErrRefusingToDelete          = errors.New("refusing to delete directory containing")
	ErrRootPath                  = errors.New("root path cannot be empty")
	ErrUserAborted               = errors.New("mission aborted")
	ErrComponentNotFound         = errors.New("could not find component")
	ErrRelPath                   = errors.New("error getting relative path")
)
