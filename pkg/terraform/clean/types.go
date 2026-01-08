package clean

// EnvTFDataDir is the environment variable name for TF_DATA_DIR.
const EnvTFDataDir = "TF_DATA_DIR"

// ObjectInfo contains information about a file or directory object discovered during clean operations.
type ObjectInfo struct {
	// FullPath is the absolute path to the object.
	FullPath string
	// RelativePath is the path relative to the component directory.
	RelativePath string
	// Name is the base name of the object.
	Name string
	// IsDir indicates whether the object is a directory.
	IsDir bool
}

// Directory represents a directory discovered during clean operations along with its contained files.
type Directory struct {
	// Name is the base name of the directory.
	Name string
	// FullPath is the absolute path to the directory.
	FullPath string
	// RelativePath is the path relative to the component directory.
	RelativePath string
	// Files contains the list of files and subdirectories within this directory.
	Files []ObjectInfo
}
