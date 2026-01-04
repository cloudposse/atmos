package clean

// EnvTFDataDir is the environment variable name for TF_DATA_DIR.
const EnvTFDataDir = "TF_DATA_DIR"

// ObjectInfo contains information about a file or directory object.
type ObjectInfo struct {
	FullPath     string
	RelativePath string
	Name         string
	IsDir        bool
}

// Directory represents a directory with its files.
type Directory struct {
	Name         string
	FullPath     string
	RelativePath string
	Files        []ObjectInfo
}
