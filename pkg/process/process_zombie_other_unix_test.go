//go:build !windows && !darwin

package process

import (
	"os"
	"strconv"
)

func processIsZombie(pid int) bool {
	data, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/stat")
	if err != nil {
		return false
	}
	// Format: "pid (comm) S ..." -- the state follows the last ')'.
	s := string(data)
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == ')' {
			return i+2 < len(s) && s[i+2] == 'Z'
		}
	}
	return false
}
