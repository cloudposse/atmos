package auth

import "os"

func IsInDocker() bool {
	if _, err := os.Stat("/.dockerenv"); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}
