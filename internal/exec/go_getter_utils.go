// https://github.com/hashicorp/go-getter

package exec

import (
	"fmt"
	"strings"
)

// ValidateURI validates URIs.
func ValidateURI(uri string) error {
	if uri == "" {
		return fmt.Errorf("URI cannot be empty")
	}
	// Maximum length check
	if len(uri) > 2048 {
		return fmt.Errorf("URI exceeds maximum length of 2048 characters")
	}
	// Validate URI format
	if strings.Contains(uri, "..") {
		return fmt.Errorf("URI cannot contain path traversal sequences")
	}
	if strings.Contains(uri, " ") {
		return fmt.Errorf("URI cannot contain spaces")
	}
	// Validate scheme-specific format
	if strings.HasPrefix(uri, "oci://") {
		if !strings.Contains(uri[6:], "/") {
			return fmt.Errorf("invalid OCI URI format")
		}
	} else if strings.Contains(uri, "://") {
		scheme := strings.Split(uri, "://")[0]
		if !IsValidScheme(scheme) {
			return fmt.Errorf("unsupported URI scheme: %s", scheme)
		}
	}
	return nil
}

// IsValidScheme checks if the URL scheme is valid
func IsValidScheme(scheme string) bool {
	validSchemes := map[string]bool{
		"http":       true,
		"https":      true,
		"git":        true,
		"ssh":        true,
		"git::https": true,
		"git::ssh":   true,
	}
	return validSchemes[scheme]
}

/*
Supported schemes:

file, dir, tar, zip
http, https
git, hg
s3, gcs
oci
scp, sftp
Shortcuts like github.com, bitbucket.org

- File-related Schemes:
file - Local filesystem paths
dir - Local directories
tar - Tar files, potentially compressed (tar.gz, tar.bz2, etc.)
zip - Zip files

- HTTP/HTTPS:
http - HTTP URLs
https - HTTPS URLs

- Git:
git - Git repositories, which can be accessed via HTTPS or SSH

- Mercurial:
hg - Mercurial repositories, accessed via HTTP/S or SSH

- Amazon S3:
s3 - Amazon S3 bucket URLs

- Google Cloud Storage:
gcs - Google Cloud Storage URLs

- OCI:
oci - Open Container Initiative (OCI) images

- Other Protocols:
scp - Secure Copy Protocol for SSH-based transfers
sftp - SSH File Transfer Protocol

- GitHub/Bitbucket/Other Shortcuts:
github.com - Direct GitHub repository shortcuts
bitbucket.org - Direct Bitbucket repository shortcuts

- Composite Schemes:
go-getter allows for composite schemes, where multiple operations can be combined. For example:
git::https://github.com/user/repo - Forces the use of git over an HTTPS URL.
tar::http://example.com/archive.tar.gz - Treats the HTTP resource as a tarball.

*/
