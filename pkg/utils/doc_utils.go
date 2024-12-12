package utils

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// DisplayDocs displays component documentation directly through the terminal or
// through a pager (like less). The use of a pager is determined by the pagination value
// set in the CLI Settings for Atmos
func DisplayDocs(componentDocs string, usePager bool) error {
	if !usePager {
		fmt.Println(componentDocs)
		return nil
	}

	pagerCmd := os.Getenv("PAGER")
	if pagerCmd == "" {
		pagerCmd = "less -r"
	}

	args := strings.Fields(pagerCmd)
	if len(args) == 0 {
		return fmt.Errorf("invalid pager command")
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin = strings.NewReader(componentDocs)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to execute pager: %w", err)
	}

	return nil
}
