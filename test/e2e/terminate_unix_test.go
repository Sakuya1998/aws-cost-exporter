//go:build !windows

package e2e_test

import (
	"os"
	"syscall"
)

func terminateProcess(process *os.Process) (bool, error) {
	return true, process.Signal(syscall.SIGTERM)
}
