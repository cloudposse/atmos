package convert

import (
	"crypto/sha1"
	"fmt"
)

// MakeId takes a byte representation of a resource and returns a stable string ID for it.
func MakeId(s []byte) string {
	return fmt.Sprintf("%x", sha1.Sum(s))
}
