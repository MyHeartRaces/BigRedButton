package linux

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type lookPathFunc func(string) (string, error)

func defaultLookPath(lookPath lookPathFunc) lookPathFunc {
	if lookPath == nil {
		return exec.LookPath
	}
	return lookPath
}

func validateExecutable(lookPath lookPathFunc, binary string) error {
	binary = strings.TrimSpace(binary)
	if binary == "" {
		return fmt.Errorf("prerequisite binary is empty")
	}
	if strings.Contains(binary, "/") {
		info, err := os.Stat(binary)
		if err != nil {
			return fmt.Errorf("required binary %s is not accessible: %w", binary, err)
		}
		if info.IsDir() {
			return fmt.Errorf("required binary %s is a directory", binary)
		}
		if info.Mode()&0o111 == 0 {
			return fmt.Errorf("required binary %s is not executable", binary)
		}
		return nil
	}
	if _, err := defaultLookPath(lookPath)(binary); err != nil {
		return fmt.Errorf("required binary %s was not found in PATH: %w", binary, err)
	}
	return nil
}
